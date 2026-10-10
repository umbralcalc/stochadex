package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// sleepyParamEchoIteration's next state is its "level" param, after a short
// sleep, so a run of it lasts long enough for a stream's messages to arrive
// while it runs.
type sleepyParamEchoIteration struct{}

func (s *sleepyParamEchoIteration) Configure(int, *simulator.Settings) {}

func (s *sleepyParamEchoIteration) Iterate(
	params *simulator.Params,
	_ int,
	_ []*simulator.StateHistory,
	_ *simulator.CumulativeTimestepsHistory,
) []float64 {
	time.Sleep(300 * time.Microsecond)
	return append([]float64(nil), params.Get("level")...)
}

func init() {
	RegisterIteration("sleepy_param_echo", func(simulator.ComponentSpec) (simulator.Iteration, error) {
		return &sleepyParamEchoIteration{}, nil
	})
}

// feedServer serves one websocket connection at a time, handing it to send,
// and returns the server's ws:// URL.
func feedServer(t *testing.T, send func(connection *websocket.Conn)) string {
	t.Helper()
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer connection.Close()
		send(connection)
		// Hold the connection open until the client goes, as a live feed would.
		for {
			if _, _, err := connection.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)
	return "ws" + strings.TrimPrefix(server.URL, "http")
}

// rising sends level = 1, 2, 3, ... every few milliseconds after a delay,
// interleaved with a partition no config binds, until the client goes.
func rising(connection *websocket.Conn) {
	time.Sleep(20 * time.Millisecond)
	for i := 1; ; i++ {
		message := fmt.Sprintf(`[{"partition_name": "level", "state": [%d]}, `+
			`{"partition_name": "unbound", "state": [0, 0, 0]}]`, i)
		if err := connection.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
			return
		}
		time.Sleep(3 * time.Millisecond)
	}
}

// streamConfigYAML is a run whose dial echoes its level param, set from the
// feed input, and whose walk's variances follow the dial within each step.
func streamConfigYAML(feed string, steps int) string {
	return fmt.Sprintf(`inputs:
  feed:
%s
main:
  partitions:
  - {name: dial, iteration: {type: sleepy_param_echo}, params: {level: [0.0]}, params_from_input: {level: {input: feed}}, init_state_values: [0.0], state_history_depth: 1, seed: 1}
  - {name: walk, iteration: {type: wiener_process}, params: {variances: [1.0]}, params_from_upstream: {variances: {upstream: dial}}, init_state_values: [0.0], state_history_depth: 1, seed: 2}
  simulation:
    output_condition: {type: every_step}
    output_function: {type: nil}
    termination_condition: {type: number_of_steps, max_steps: %d}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`, feed, steps)
}

func TestStreamInputs(t *testing.T) {
	t.Run("a live run replays exactly from its recording", func(t *testing.T) {
		record := filepath.Join(t.TempDir(), "feed.log")
		url := feedServer(t, rising)
		live, err := RunToStorage(writeConfig(t, streamConfigYAML(fmt.Sprintf(
			"    stream: {websocket: {url: %q}}\n    record: %q", url, record), 300)))
		if err != nil {
			t.Fatal(err)
		}
		dial := live.Storage.GetValues("dial")
		if len(dial) != 301 {
			t.Fatalf("dial has %d rows, want 301", len(dial))
		}
		// Until the first message the level keeps its configured value; after,
		// each step holds the newest value, which only ever rises.
		if dial[1][0] != 0 {
			t.Errorf("step 1 should hold the configured level 0 (the first message is delayed), got %v", dial[1])
		}
		distinct := map[float64]bool{}
		for k := 2; k < len(dial); k++ {
			if dial[k][0] < dial[k-1][0] {
				t.Fatalf("level fell from %v to %v at step %d", dial[k-1][0], dial[k][0], k)
			}
			distinct[dial[k][0]] = true
		}
		if len(distinct) < 3 {
			t.Fatalf("the run saw only %d distinct levels; the stream did not reach it as it ran", len(distinct))
		}
		// The recording is exactly what each step was given: row k is step k's level.
		recorded := readLogEntries(t, record)
		if len(recorded) != 301 {
			t.Fatalf("the recording has %d rows, want 301 (the initial row and 300 steps)", len(recorded))
		}
		for k, entry := range recorded {
			if entry.PartitionName != "level" || entry.CumulativeTimesteps != float64(k) ||
				entry.State[0] != dial[k][0] && k > 0 {
				t.Fatalf("recording row %d = %+v, but step %d's level was %v", k, entry, k, dial[k])
			}
		}
		// Replay: the same config with the stream swapped for its recording.
		replay, err := RunToStorage(writeConfig(t, streamConfigYAML(fmt.Sprintf(
			"    source: {json_log: {path: %q}}", record), 300)))
		if err != nil {
			t.Fatal(err)
		}
		assertSameStorage(t, "replay vs live", replay.Storage, live.Storage)
		assertSameStorage(t, "live vs replay", live.Storage, replay.Storage)
	})

	t.Run("a stream that sends nothing leaves the configured value", func(t *testing.T) {
		url := feedServer(t, func(*websocket.Conn) {})
		result, err := RunToStorage(writeConfig(t, streamConfigYAML(
			fmt.Sprintf("    stream: {websocket: {url: %q}}", url), 20)))
		if err != nil {
			t.Fatal(err)
		}
		for k, row := range result.Storage.GetValues("dial") {
			if row[0] != 0 {
				t.Fatalf("step %d has level %v, want the configured 0", k, row)
			}
		}
	})

	failures := []struct {
		name string
		send func(*websocket.Conn)
		kind ErrorKind
		want string
	}{
		{"a message that is not JSON", func(c *websocket.Conn) {
			c.WriteMessage(websocket.TextMessage, []byte("level=3"))
		}, ErrData, `stream input "feed": decoding message "level=3"`},
		{"an entry with no partition_name", func(c *websocket.Conn) {
			c.WriteMessage(websocket.TextMessage, []byte(`{"state": [1]}`))
		}, ErrData, "no partition_name"},
		{"a value of the wrong width", func(c *websocket.Conn) {
			c.WriteMessage(websocket.TextMessage, []byte(`{"partition_name": "level", "state": [1, 2]}`))
		}, ErrData, `params key "level" has width 1, got 2 values`},
	}
	for _, c := range failures {
		t.Run(c.name+" fails the run", func(t *testing.T) {
			url := feedServer(t, c.send)
			_, err := RunToStorage(writeConfig(t, streamConfigYAML(
				fmt.Sprintf("    stream: {websocket: {url: %q}}", url), 1000)))
			if KindOf(err) != c.kind || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected %v containing %q, got %v", c.kind, c.want, err)
			}
		})
	}

	t.Run("each bound partition of a stream gets its own values", func(t *testing.T) {
		url := feedServer(t, func(c *websocket.Conn) {
			c.WriteMessage(websocket.TextMessage, []byte(
				`[{"partition_name": "level", "state": [3]}, {"partition_name": "other", "state": [7]}]`))
		})
		yaml := strings.Replace(streamConfigYAML(fmt.Sprintf("    stream: {websocket: {url: %q}}", url), 200),
			"  simulation:", "  - {name: dial2, iteration: {type: sleepy_param_echo}, params: {level: [0.0]}, "+
				"params_from_input: {level: {input: feed, partition: other}}, init_state_values: [0.0], "+
				"state_history_depth: 1, seed: 4}\n  simulation:", 1)
		result, err := RunToStorage(writeConfig(t, yaml))
		if err != nil {
			t.Fatal(err)
		}
		dial, dial2 := result.Storage.GetValues("dial"), result.Storage.GetValues("dial2")
		if last := len(dial) - 1; dial[last][0] != 3 || dial2[last][0] != 7 {
			t.Errorf("finished with dial %v and dial2 %v, want [3] and [7]", dial[last], dial2[last])
		}
	})

	t.Run("a second stream that cannot be reached closes the first", func(t *testing.T) {
		// A server that reports when its one client goes.
		closed := make(chan struct{})
		upgrader := websocket.Upgrader{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			connection, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer connection.Close()
			for {
				if _, _, err := connection.ReadMessage(); err != nil {
					close(closed)
					return
				}
			}
		}))
		defer server.Close()
		url := "ws" + strings.TrimPrefix(server.URL, "http")
		yaml := strings.Replace(streamConfigYAML(fmt.Sprintf("    stream: {websocket: {url: %q}}", url), 5),
			"main:", "  gone:\n    stream: {websocket: {url: \"ws://127.0.0.1:1/feed\"}}\nmain:", 1)
		yaml = strings.Replace(yaml, "params: {variances: [1.0]}, params_from_upstream: {variances: {upstream: dial}}",
			"params: {variances: [1.0]}, params_from_input: {variances: {input: gone}}", 1)
		_, err := RunToStorage(writeConfig(t, yaml))
		if KindOf(err) != ErrUnavailable || !strings.Contains(err.Error(), `stream input "gone"`) {
			t.Fatalf("expected ErrUnavailable naming the unreachable stream, got %v", err)
		}
		select {
		case <-closed:
		case <-time.After(5 * time.Second):
			t.Error("the first stream's connection was left open")
		}
	})

	t.Run("a stream that cannot be reached is unavailable", func(t *testing.T) {
		_, err := RunToStorage(writeConfig(t, streamConfigYAML(
			`    stream: {websocket: {url: "ws://127.0.0.1:1/feed"}}`, 5)))
		if KindOf(err) != ErrUnavailable || !strings.Contains(err.Error(), `stream input "feed"`) {
			t.Errorf("expected ErrUnavailable naming the input, got %v", err)
		}
	})
}

// storedParamsYAML sets a walk's variances from a CSV input's rows.
func storedParamsYAML(csv string, steps int, binding string) string {
	return fmt.Sprintf(`inputs:
  vol: {source: {csv: {path: %q, time_column: 0, state_columns: {variances: [1]}}}}
main:
  partitions:
  - {name: walk, iteration: {type: wiener_process}, params: {variances: [1.0]}, %s, init_state_values: [0.0], state_history_depth: 1, seed: 3}
%s  simulation:
    output_condition: {type: every_step}
    output_function: {type: nil}
    termination_condition: {type: number_of_steps, max_steps: %d}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`, csv, binding, "", steps)
}

func TestParamsFromStoredInput(t *testing.T) {
	dir := t.TempDir()
	csv := filepath.Join(dir, "vol.csv")
	writeFile(t, csv, "0,0.5\n1,1\n2,4\n3,0.25\n4,9\n5,16\n")

	t.Run("step k gets row k, as a from_input partition wired through params_from_upstream does", func(t *testing.T) {
		got, err := RunToStorage(writeConfig(t, storedParamsYAML(csv, 5,
			"params_from_input: {variances: {input: vol}}")))
		if err != nil {
			t.Fatal(err)
		}
		// Reference: the input replayed as a partition, read within the step.
		reference, err := RunToStorage(writeConfig(t, strings.Replace(storedParamsYAML(csv, 5,
			"params_from_upstream: {variances: {upstream: variances}}"),
			"  simulation:", "  - {name: variances, iteration: {type: from_input, input: vol}, "+
				"state_history_depth: 1, seed: 0}\n  simulation:", 1)))
		if err != nil {
			t.Fatal(err)
		}
		assertSameStorage(t, "params_from_input vs the reference",
			storageWith(got.Storage, "walk"), storageWith(reference.Storage, "walk"))
		// Positive control: the input changes the run.
		withoutInput := storedParamsYAML(csv, 5, "seed: 3")
		withoutInput = withoutInput[strings.Index(withoutInput, "main:"):]
		constant, err := RunToStorage(writeConfig(t, withoutInput))
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(constant.Storage.GetValues("walk")) == fmt.Sprint(got.Storage.GetValues("walk")) {
			t.Error("the run with variances from the input matches one with constant variances")
		}
	})

	t.Run("a config reads its stored rows once, as from_input does", func(t *testing.T) {
		copied := filepath.Join(t.TempDir(), "vol.csv")
		writeFile(t, copied, readFile(t, csv))
		config := writeConfig(t, storedParamsYAML(copied, 5, "params_from_input: {variances: {input: vol}}"))
		first, err := RunToStorage(config)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(copied); err != nil {
			t.Fatal(err)
		}
		second, err := RunToStorage(config)
		if err != nil {
			t.Fatalf("a second run of the same config should reuse the rows it read: %v", err)
		}
		assertSameStorage(t, "second run", second.Storage, first.Storage)
	})

	t.Run("a stored input that cannot be read is unavailable", func(t *testing.T) {
		_, err := RunToStorage(writeConfig(t, storedParamsYAML(filepath.Join(dir, "absent.csv"), 5,
			"params_from_input: {variances: {input: vol}}")))
		if KindOf(err) != ErrUnavailable || !strings.Contains(err.Error(), `input "vol"`) {
			t.Errorf("expected ErrUnavailable naming the input, got %v", err)
		}
	})

	t.Run("a row of another width is a data error at its step", func(t *testing.T) {
		log := filepath.Join(dir, "ragged.log")
		writeFile(t, log, `{"partition_name": "variances", "state": [1], "time": 0}
{"partition_name": "variances", "state": [2], "time": 1}
{"partition_name": "variances", "state": [3, 4], "time": 2}
`)
		yaml := strings.Replace(storedParamsYAML(csv, 2, "params_from_input: {variances: {input: vol}}"),
			fmt.Sprintf("{csv: {path: %q, time_column: 0, state_columns: {variances: [1]}}}", csv),
			fmt.Sprintf("{json_log: {path: %q}}", log), 1)
		_, err := RunToStorage(writeConfig(t, yaml))
		if KindOf(err) != ErrData || !strings.Contains(err.Error(), "has width 1, got 2 values") {
			t.Errorf("expected ErrData about the row's width, got %v", err)
		}
	})

	errorCases := []struct{ name, yaml, want string }{
		{"running past the input's rows", storedParamsYAML(csv, 6,
			"params_from_input: {variances: {input: vol}}"), `input "vol"'s "variances" has 6 rows`},
		{"an input partition that does not exist", storedParamsYAML(csv, 5,
			"params_from_input: {variances: {input: vol, partition: vol}}"), `input "vol" has no partition "vol"`},
		{"a width that does not match", strings.Replace(storedParamsYAML(csv, 5,
			"params_from_input: {variances: {input: vol}}"), "params: {variances: [1.0]}",
			"params: {variances: [1.0, 1.0]}", 1), `params "variances" has width 2 but input "vol"'s "variances" rows have 1`},
	}
	for _, c := range errorCases {
		t.Run(c.name+" is a data error", func(t *testing.T) {
			_, err := RunToStorage(writeConfig(t, c.yaml))
			if KindOf(err) != ErrData || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrData containing %q, got %v", c.want, err)
			}
		})
	}
}

func TestStreamInputValidation(t *testing.T) {
	stream := `    stream: {websocket: {url: "ws://localhost:9000/feed"}}`
	cases := []struct{ name, yaml, want string }{
		{"a stream with no transport", streamConfigYAML("    stream: {}", 5), "stream: needs one transport"},
		{"an unknown decode", streamConfigYAML(stream+"\n    decode: csv", 5), `unknown decode "csv"`},
		{"an on_empty other than hold_last", streamConfigYAML(stream+"\n    on_empty: block", 5),
			`on_empty "block" is not supported yet`},
		{"stream options on a stored input", streamConfigYAML(
			"    source: {csv: {path: x.csv, time_column: 0, state_columns: {level: [1]}}}\n    record: r.log", 5),
			"decode:, on_empty: and record: only apply to stream: inputs"},
		{"both stream: and source:", streamConfigYAML(stream+
			"\n    source: {csv: {path: x.csv, time_column: 0, state_columns: {level: [1]}}}", 5),
			"needs exactly one of source:, simulation:, stream: or file:"},
		{"from_input on a stream", strings.Replace(streamConfigYAML(stream, 5),
			"iteration: {type: sleepy_param_echo}, params: {level: [0.0]}, params_from_input: {level: {input: feed}}",
			"iteration: {type: from_input, input: feed, partition: level}", 1),
			`reads input "feed"'s stored rows, but it is a stream input`},
		{"an undeclared params key", strings.Replace(streamConfigYAML(stream, 5),
			"params: {level: [0.0]}", "params: {other: [0.0]}", 1),
			`declare "level" in the partition's params`},
		{"a key also set from upstream", strings.Replace(streamConfigYAML(stream, 5),
			"params_from_upstream: {variances: {upstream: dial}}",
			"params_from_upstream: {variances: {upstream: dial}}, params_from_input: {variances: {input: feed, partition: level}}", 1),
			`"variances" is also set by params_from_upstream`},
		{"two initial values for one stream partition", strings.Replace(streamConfigYAML(stream, 5),
			"params_from_upstream: {variances: {upstream: dial}}",
			"params_from_input: {variances: {input: feed, partition: level}}", 1),
			"declare different initial values"},
		{"an undeclared input", strings.Replace(streamConfigYAML(stream, 5),
			"{level: {input: feed}}", "{level: {input: fed}}", 1), `names input "fed"`},
		{"a stream under an ensemble", streamConfigYAML(stream, 5) + "run: {mode: ensemble, seeds: [1]}\n",
			"do not yet apply to run: {mode: ensemble}"},
		{"two widths for one stream partition", strings.Replace(streamConfigYAML(stream, 5),
			"params: {variances: [1.0]}, params_from_upstream: {variances: {upstream: dial}}",
			"params: {variances: [0.0, 0.0]}, params_from_input: {variances: {input: feed, partition: level}}", 1),
			"declare different initial values"},
		{"params_from_input in an embedded run", strings.Replace(nestedSinksYAML(t.TempDir()),
			"    params: {variances: [1.0]}\n    init_state_values: [5.0]",
			"    params: {variances: [1.0]}\n    params_from_input: {variances: {input: feed, partition: level}}\n"+
				"    init_state_values: [5.0]", 1) + "inputs:\n  feed:\n" + stream + "\n",
			"params_from_input is not yet supported inside embedded runs"},
	}
	for _, c := range cases {
		t.Run(c.name+" is a config error", func(t *testing.T) {
			_, err := LoadConfig(writeConfigPath(t, c.yaml))
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}
	t.Run("two partitions may share a stream partition with the same initial value", func(t *testing.T) {
		_, err := LoadConfig(writeConfigPath(t, strings.Replace(streamConfigYAML(stream, 5),
			"params: {variances: [1.0]}, params_from_upstream: {variances: {upstream: dial}}",
			"params: {variances: [0.0]}, params_from_input: {variances: {input: feed, partition: level}}", 1)))
		if err != nil {
			t.Errorf("the same initial value should be accepted, got %v", err)
		}
	})
	t.Run("a stream input for macros is a config error", func(t *testing.T) {
		_, err := LoadConfig(writeConfigPath(t, macroConfigYAML+"inputs:\n  feed:\n"+stream+"\n"))
		if err == nil || !strings.Contains(err.Error(), "stream") {
			t.Errorf("expected a config error about the stream input, got %v", err)
		}
	})
}

func TestRunsWithoutFeedsCostNothing(t *testing.T) {
	// A config without params_from_input runs exactly as before: no feeds are
	// built, and building none allocates nothing.
	config := writeConfig(t, serveConfigYAML)
	loader := inputLoader{config: config}
	if allocs := testing.AllocsPerRun(100, func() {
		if feeds, err := newParamFeeds(config, &loader); feeds != nil || err != nil {
			t.Fatalf("expected no feeds, got %v, %v", feeds, err)
		}
	}); allocs != 0 {
		t.Errorf("building no feeds allocated %v times, want 0", allocs)
	}
}

func TestOpeningFeedsChecksTheirKeys(t *testing.T) {
	// Load-time validation keeps these from happening through a config; open
	// still refuses a key it cannot set, before connecting anything.
	prepared, err := preparedMainGenerator(writeConfig(t, serveConfigYAML))
	if err != nil {
		t.Fatal(err)
	}
	coordinator := simulator.NewPartitionCoordinator(prepared.settings, prepared.implementations)
	for name, feeds := range map[string]*paramFeeds{
		"stored": {stored: []storedFeed{{partition: "ghost", key: "k"}}},
		"stream": {streams: []*streamFeed{{name: "feed", url: "ws://127.0.0.1:1/x",
			bindings: []streamBinding{{partition: "ghost", key: "k"}}}}},
	} {
		if err := feeds.open(coordinator); KindOf(err) != ErrData ||
			!strings.Contains(err.Error(), `no partition named "ghost"`) {
			t.Errorf("%s: expected ErrData about the partition, got %v", name, err)
		}
	}
}

func TestDecodeEntries(t *testing.T) {
	one, err := decodeEntries([]byte(` {"partition_name": "a", "state": [1, 2], "time": 9} `))
	if err != nil || len(one) != 1 || one[0].PartitionName != "a" || len(one[0].State) != 2 {
		t.Errorf("one entry: got %+v, %v", one, err)
	}
	many, err := decodeEntries([]byte(`[{"partition_name": "a", "state": [1]}, {"partition_name": "b", "state": []}]`))
	if err != nil || len(many) != 2 || many[1].PartitionName != "b" {
		t.Errorf("an array: got %+v, %v", many, err)
	}
	if _, err := decodeEntries([]byte(`[{"partition_name": "a"`)); err == nil ||
		!strings.Contains(err.Error(), "decoding message") {
		t.Errorf("a broken array should be an error, got %v", err)
	}
	if _, err := decodeEntries([]byte(strings.Repeat("x", 200))); err == nil ||
		!strings.Contains(err.Error(), "...") {
		t.Errorf("a long bad message should be an error quoting its start, got %v", err)
	}
}
