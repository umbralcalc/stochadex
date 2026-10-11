package keyboard

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eiannone/keyboard"
	"github.com/umbralcalc/stochadex/pkg/api"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gopkg.in/yaml.v2"
)

// fakeKeys replaces the terminal with events, until the test ends, and
// reports whether the keyboard was released.
func fakeKeys(t *testing.T, events ...keyboard.KeyEvent) *bool {
	t.Helper()
	channel := make(chan keyboard.KeyEvent, len(events))
	for _, event := range events {
		channel <- event
	}
	released := false
	original := openKeys
	openKeys = func() (<-chan keyboard.KeyEvent, func() error, error) {
		return channel, func() error { released = true; return nil }, nil
	}
	t.Cleanup(func() { openKeys = original })
	return &released
}

func press(r rune) keyboard.KeyEvent { return keyboard.KeyEvent{Rune: r} }

func fields(t *testing.T, text string) map[string]interface{} {
	t.Helper()
	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestKeyMap(t *testing.T) {
	t.Run("keys map to values of one width, single digits included", func(t *testing.T) {
		parsed, err := parseKeyMap(fields(t, "{partition: steer, keys: {a: [-1.0, 0], d: [1.0, 2], 1: [0.5, 0.5]}}"))
		if err != nil {
			t.Fatal(err)
		}
		want := map[rune][]float64{'a': {-1, 0}, 'd': {1, 2}, '1': {0.5, 0.5}}
		if parsed.partition != "steer" || !reflect.DeepEqual(parsed.keys, want) {
			t.Errorf("parsed %+v", parsed)
		}
	})
	cases := map[string]string{
		"{keys: {a: [1.0]}}":                                  "needs partition: and keys:",
		"{partition: steer}":                                  "needs partition: and keys:",
		"{partition: 3, keys: {a: [1.0]}}":                    "partition must name",
		"{partition: steer, keys: []}":                        "keys must map each key",
		"{partition: steer, keys: {ab: [1.0]}}":               `key "ab" must be a single character`,
		"{partition: steer, keys: {a: 1.0}}":                  `key "a": its value must be a list of numbers`,
		"{partition: steer, keys: {a: []}}":                   `key "a": its value must be a list of numbers`,
		"{partition: steer, keys: {a: [x]}}":                  `key "a": its value must be a list of numbers, got x`,
		"{partition: steer, keys: {a: [1.0], d: [1.0, 2.0]}}": "every key sets the whole partition",
		"{partition: steer, keys: {a: [1.0]}, mode: x}":       `unknown field "mode"`,
	}
	for text, want := range cases {
		t.Run(text, func(t *testing.T) {
			if _, err := parseKeyMap(fields(t, text)); err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("expected an error containing %q, got %v", want, err)
			}
		})
	}
}

func readAll(t *testing.T, source api.StreamSource) []simulator.JsonLogEntry {
	t.Helper()
	var delivered []simulator.JsonLogEntry
	source.Read(func(message []byte) error {
		var entries []simulator.JsonLogEntry
		if err := json.Unmarshal(message, &entries); err != nil {
			t.Fatal(err)
		}
		delivered = append(delivered, entries...)
		return nil
	})
	return delivered
}

func TestKeyboardSource(t *testing.T) {
	keyMap := fields(t, "{partition: throttle, keys: {a: [-1.0], d: [1.0]}}")

	t.Run("each mapped key is a message; others are ignored, and Esc stops listening", func(t *testing.T) {
		released := fakeKeys(t, press('d'), press('x'), press('a'), keyboard.KeyEvent{Key: keyboard.KeyEsc}, press('d'))
		source, err := openKeyboard(keyMap)
		if err != nil {
			t.Fatal(err)
		}
		delivered := readAll(t, source)
		want := []simulator.JsonLogEntry{
			{PartitionName: "throttle", State: []float64{1}},
			{PartitionName: "throttle", State: []float64{-1}},
		}
		if !reflect.DeepEqual(delivered, want) {
			t.Errorf("delivered %+v, want %+v (nothing after Esc)", delivered, want)
		}
		source.Close()
		if !*released {
			t.Error("Close did not give the terminal back")
		}
	})

	t.Run("Close ends a read waiting for keys, and may be called twice", func(t *testing.T) {
		fakeKeys(t)
		source, err := openKeyboard(keyMap)
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() { readAll(t, source); close(done) }()
		source.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Read did not return after Close")
		}
		source.Close()
	})

	t.Run("a message the run cannot take ends the read", func(t *testing.T) {
		fakeKeys(t, press('d'), press('a'))
		source, _ := openKeyboard(keyMap)
		calls := 0
		source.Read(func([]byte) error { calls++; return fmt.Errorf("bad width") })
		if calls != 1 {
			t.Errorf("Read delivered %d messages after a refusal, want 1", calls)
		}
	})

	t.Run("a keyboard that will not open fails the opening", func(t *testing.T) {
		original := openKeys
		openKeys = func() (<-chan keyboard.KeyEvent, func() error, error) {
			return nil, nil, fmt.Errorf("not a terminal")
		}
		defer func() { openKeys = original }()
		if _, err := openKeyboard(keyMap); err == nil || !strings.Contains(err.Error(), "not a terminal") {
			t.Errorf("got %v", err)
		}
	})
}

// steeredYAML is a run steered from the keyboard: the key sets a param_values
// partition, which drives a Wiener process's variance, and the keyboard's
// values are recorded, paced so the keys reach it while it runs.
func steeredYAML(keys, record, log string) string {
	return fmt.Sprintf(`inputs:
  keys:
%s
main:
  partitions:
  - name: steering
    iteration: {type: param_values}
    params: {param_values: [0.5]}
    params_from_input: {param_values: {input: keys, partition: steer}}
    init_state_values: [0.5]
    state_history_depth: 1
    seed: 0
  - name: walk
    iteration: {type: wiener_process}
    params: {variances: [1.0]}
    params_from_upstream: {variances: {upstream: steering}}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 7
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 40}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
outputs:
- {name: log, function: {type: json_log, path: %q}}
run: {pace_ms: 5}
`, keys, log)
}

func logRows(t *testing.T, path, partition string) map[float64][]float64 {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	rows := map[float64][]float64{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry simulator.JsonLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatal(err)
		}
		if entry.PartitionName == partition {
			rows[entry.CumulativeTimesteps] = entry.State
		}
	}
	return rows
}

func TestKeyboardSteersARun(t *testing.T) {
	dir := t.TempDir()
	record, live, replayed := filepath.Join(dir, "keys.log"), filepath.Join(dir, "live.log"), filepath.Join(dir, "replay.log")
	fakeKeys(t, press('d'))
	keys := fmt.Sprintf("    stream: {keyboard: {partition: steer, keys: {a: [0.1], d: [4.0]}}}\n    record: %q", record)
	write := func(name, text string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	if err := api.Execute([]string{"stochadex", "--config", write("live.yaml", steeredYAML(keys, record, live))}); err != nil {
		t.Fatal(err)
	}
	steering := logRows(t, live, "steering")
	if steering[0][0] != 0.5 || steering[40][0] != 4 {
		t.Errorf("steering went from %v to %v; want the configured 0.5, then the key's 4", steering[0], steering[40])
	}
	for step := 1; step <= 40; step++ {
		if previous, now := steering[float64(step-1)][0], steering[float64(step)][0]; now != previous && now != 4 {
			t.Fatalf("step %d: steering %v; it only changes to the key's value", step, now)
		}
	}
	// The recording replays the run exactly, as a json_log source.
	replay := fmt.Sprintf("    source: {json_log: {path: %q}}", record)
	if err := api.Execute([]string{"stochadex", "--config", write("replay.yaml",
		strings.Replace(steeredYAML(replay, record, replayed), "run: {pace_ms: 5}\n", "", 1))}); err != nil {
		t.Fatal(err)
	}
	for _, partition := range []string{"steering", "walk"} {
		if got, want := logRows(t, replayed, partition), logRows(t, live, partition); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: the replay differs from the keyboard run", partition)
		}
	}
}

func TestKeyboardStreamInTheManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(path, []byte(steeredYAML("    stream: {keyboard: {partition: steer, keys: {d: [1.0]}}}", "", "x.log")), 0o644)
	config, err := api.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if input := api.Manifest(config).Inputs[0]; input.Kind != "stream" || input.Location != "keyboard" {
		t.Errorf("manifest input %+v", input)
	}
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	os.WriteFile(bad, []byte(steeredYAML("    stream: {keyboard: {partition: steer, keys: {dd: [1.0]}}}", "", "x.log")), 0o644)
	if _, err := api.LoadConfig(bad); api.KindOf(err) != api.ErrConfig ||
		!strings.Contains(err.Error(), `input "keys": stream keyboard: key "dd" must be a single character`) {
		t.Errorf("a bad key map should fail the load, got %v", err)
	}
}

func TestOpeningTheKeyboard(t *testing.T) {
	t.Run("without a terminal, opening the real keyboard is an error, not a hang", func(t *testing.T) {
		events, release, err := openKeys()
		if err == nil {
			release()
			_ = events
			t.Skip("a terminal is attached, so the keyboard opened")
		}
	})
	t.Run("Open refuses fields Check would have, for callers that skip loading", func(t *testing.T) {
		fakeKeys(t)
		if _, err := openKeyboard(fields(t, "{partition: steer}")); err == nil ||
			!strings.Contains(err.Error(), "needs partition: and keys:") {
			t.Errorf("got %v", err)
		}
	})
}
