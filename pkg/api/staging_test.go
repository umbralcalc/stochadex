package api

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// failsAtStepIteration counts steps, sleeping sleep_ms each, and panics at
// step fail_at (never when it is 0).
type failsAtStepIteration struct {
	failAt, step int
	sleep        time.Duration
}

func (f *failsAtStepIteration) Configure(partitionIndex int, settings *simulator.Settings) {
	params := settings.Iterations[partitionIndex].Params
	f.failAt = int(params.GetIndex("fail_at", 0))
	f.sleep = time.Duration(params.GetIndex("sleep_ms", 0)) * time.Millisecond
	f.step = 0
}

func (f *failsAtStepIteration) Iterate(
	*simulator.Params, int, []*simulator.StateHistory, *simulator.CumulativeTimestepsHistory,
) []float64 {
	f.step++
	time.Sleep(f.sleep)
	if f.step == f.failAt {
		panic(fmt.Sprintf("failing at step %d", f.step))
	}
	return []float64{float64(f.step)}
}

func init() {
	RegisterIteration("fails_at_step", func(simulator.ComponentSpec) (simulator.Iteration, error) {
		return &failsAtStepIteration{}, nil
	})
}

// stagedYAML is a 30-step run, stepped inline so its logs are written in a
// fixed order: a counter that fails at step failAt (0 for never) beside a
// Wiener process, a log of both, and an embedded run writing its own log.
func stagedYAML(failAt, sleepMs int, log, nested string) string {
	return fmt.Sprintf(`main:
  partitions:
  - {name: counter, iteration: {type: fails_at_step}, params: {fail_at: [%d], sleep_ms: [%d]}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
  - {name: walk, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [0.0], state_history_depth: 1, seed: 5}
  - {name: nested, params: {burn_in_steps: [0]}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 30}
    timestep_function: {type: constant, stepsize: 1.0}
    execution_strategy: {type: inline}
    init_time_value: 0.0
outputs:
- {name: log, function: {type: json_log, path: %q}}
embedded:
- name: nested
  partitions:
  - {name: inner, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [5.0], state_history_depth: 1, seed: 11}
  simulation:
    output_condition: {type: every_step}
    output_function: {type: json_log, path: %q}
    termination_condition: {type: number_of_steps, max_steps: 3}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`, failAt, sleepMs, log, nested)
}

const earlierLog = "an earlier run's log\n"

// assertOnlyEarlier fails unless each path still holds the earlier run's log,
// with no partial file beside it.
func assertOnlyEarlier(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if data, err := os.ReadFile(path); err != nil || string(data) != earlierLog {
			t.Errorf("%s: want the earlier log intact, got %q (%v)", path, data, err)
		}
		if _, err := os.Stat(path + ".partial"); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s.partial was left behind", path)
		}
	}
}

// unstagedLogs writes yaml's logs as the simulator always has, driving the
// config's own generator straight through a coordinator: the reference for
// what a staged run must publish.
func unstagedLogs(t *testing.T, yaml string) {
	t.Helper()
	config, err := LoadConfig(writeConfigPath(t, yaml))
	if err != nil {
		t.Fatal(err)
	}
	simulator.NewPartitionCoordinator(config.GetConfigGenerator().GenerateConfigs()).Run()
}

func readAll(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestAllOrNothingOutputs(t *testing.T) {
	t.Run("a clean run publishes exactly what an unstaged run writes", func(t *testing.T) {
		dir := t.TempDir()
		log, nested := filepath.Join(dir, "run.log"), filepath.Join(dir, "nested.log")
		reference, referenceNested := filepath.Join(dir, "ref.log"), filepath.Join(dir, "ref-nested.log")
		unstagedLogs(t, stagedYAML(0, 0, reference, referenceNested))
		writeFileAt(t, log, earlierLog)
		if err := Execute([]string{"stochadex", "--config", writeConfigPath(t, stagedYAML(0, 0, log, nested))}); err != nil {
			t.Fatal(err)
		}
		if readAll(t, log) != readAll(t, reference) || readAll(t, nested) != readAll(t, referenceNested) {
			t.Error("the published logs differ from the unstaged run's")
		}
		if lines := strings.Count(readAll(t, nested), "\n"); lines != 30*4 {
			t.Errorf("the nested log has %d entries, want 120 (30 outer steps x 4 rows)", lines)
		}
		for _, path := range []string{log, nested} {
			if _, err := os.Stat(path + ".partial"); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("%s.partial was left behind", path)
			}
		}
	})

	t.Run("a run that panics partway leaves the earlier logs, nested too", func(t *testing.T) {
		dir := t.TempDir()
		log, nested := filepath.Join(dir, "run.log"), filepath.Join(dir, "nested.log")
		writeFileAt(t, log, earlierLog)
		writeFileAt(t, nested, earlierLog)
		err := Execute([]string{"stochadex", "--config", writeConfigPath(t, stagedYAML(20, 0, log, nested))})
		if KindOf(err) != ErrRuntime || !strings.Contains(err.Error(), "failing at step 20") {
			t.Fatalf("expected the run to fail at step 20, got %v", err)
		}
		assertOnlyEarlier(t, log, nested)
	})

	t.Run("RunWith with the config's outputs leaves the earlier logs when its run panics", func(t *testing.T) {
		dir := t.TempDir()
		log, nested := filepath.Join(dir, "run.log"), filepath.Join(dir, "nested.log")
		writeFileAt(t, log, earlierLog)
		writeFileAt(t, nested, earlierLog)
		config := writeConfig(t, stagedYAML(20, 0, log, nested))
		func() {
			defer func() {
				if recover() == nil {
					t.Error("expected the run to panic at step 20")
				}
			}()
			RunWith(config, WithConfigOutputs())
		}()
		assertOnlyEarlier(t, log, nested)
	})

	t.Run("a run that fails partway leaves the earlier log", func(t *testing.T) {
		// A stored params_from_input input three rows long runs out at step 3.
		dir := t.TempDir()
		log := filepath.Join(dir, "run.log")
		writeFileAt(t, log, earlierLog)
		csv := filepath.Join(dir, "vol.csv")
		writeFileAt(t, csv, "0,1.0\n1,2.0\n2,3.0\n")
		yaml := hostedYAML(csv, "params_from_input: {inner/variances: {input: vol, partition: variances}}", "")
		yaml = strings.Replace(yaml, "    output_condition: {type: every_step}\n    output_function: {type: nil}\n", "", 1) +
			fmt.Sprintf("outputs:\n- {name: log, function: {type: json_log, path: %q}}\n", log)
		err := Execute([]string{"stochadex", "--config", writeConfigPath(t, yaml)})
		if KindOf(err) != ErrData {
			t.Fatalf("expected the input to run out, got %v", err)
		}
		assertOnlyEarlier(t, log)
	})

	t.Run("a stream's recording is not published when its run fails", func(t *testing.T) {
		// A stored input three rows long ends the run at step 3, after the
		// live feed has been recorded for a few steps.
		dir := t.TempDir()
		record, csv := filepath.Join(dir, "feed.log"), filepath.Join(dir, "vol.csv")
		writeFileAt(t, record, earlierLog)
		writeFileAt(t, csv, "0,1.0\n1,2.0\n2,3.0\n")
		feed := fmt.Sprintf("    stream: {websocket: {url: %q}}\n    record: %q\n"+
			"  vol: {source: {csv: {path: %q, time_column: 0, state_columns: {variances: [1]}}}}",
			feedServer(t, rising), record, csv)
		yaml := replaceOnce(t, streamConfigYAML(feed, 10), "  simulation:",
			"  - {name: ext, iteration: {type: wiener_process}, params: {variances: [1.0]}, "+
				"params_from_input: {variances: {input: vol}}, init_state_values: [0.0], "+
				"state_history_depth: 1, seed: 3}\n  simulation:")
		err := Execute([]string{"stochadex", "--config", writeConfigPath(t, yaml)})
		if KindOf(err) != ErrData {
			t.Fatalf("expected the stored input to run out, got %v", err)
		}
		assertOnlyEarlier(t, record)
	})

	t.Run("an output that cannot be published fails the run as unavailable", func(t *testing.T) {
		// A non-empty directory at a path cannot be replaced by the log; every
		// output that fails is named.
		dir := t.TempDir()
		taken, nestedTaken := filepath.Join(dir, "taken"), filepath.Join(dir, "nested-taken")
		writeFileAt(t, filepath.Join(taken, "inside"), "x")
		writeFileAt(t, filepath.Join(nestedTaken, "inside"), "x")
		err := Execute([]string{"stochadex", "--config",
			writeConfigPath(t, stagedYAML(0, 0, taken, nestedTaken))})
		if KindOf(err) != ErrUnavailable || !strings.Contains(err.Error(), "publishing "+taken) ||
			!strings.Contains(err.Error(), "publishing "+nestedTaken) {
			t.Errorf("expected ErrUnavailable naming both logs, got %v", err)
		}
	})

	t.Run("an ensemble publishes every member's log", func(t *testing.T) {
		dir := t.TempDir()
		// Four members: more sinks than a run holds in place.
		yaml := strings.NewReplacer("out/run-{member}-{seed}.log", filepath.Join(dir, "run-{member}.log"),
			"seeds: [4, 9]", "seeds: [4, 9, 11, 12]").Replace(ensembleIOYAML)
		result, err := RunWith(writeConfig(t, yaml), WithConfigOutputs())
		if err != nil {
			t.Fatal(err)
		}
		for member, run := range result.Members {
			path := filepath.Join(dir, fmt.Sprintf("run-%d.log", member))
			assertSameEntries(t, path, keyedEntries(t, path), storageEntries(run.Storage))
		}
		if files := filesUnder(t, dir); len(files) != 4 {
			t.Errorf("want exactly the four members' logs, found %v", files)
		}
	})

	t.Run("a macros run publishes its replayed log", func(t *testing.T) {
		dir := t.TempDir()
		log := filepath.Join(dir, "macro.log")
		macro, err := os.ReadFile("../../cfg/example_macro_config.yaml")
		if err != nil {
			t.Fatal(err)
		}
		config := writeConfig(t, string(macro)+
			fmt.Sprintf("outputs:\n- {name: log, function: {type: json_log, path: %q}}\n", log))
		result, err := RunWith(config, WithConfigOutputs())
		if err != nil {
			t.Fatal(err)
		}
		assertSameEntries(t, "macro log", keyedEntries(t, log), storageEntries(result.Storage))
		if files := filesUnder(t, dir); len(files) != 1 {
			t.Errorf("want only the log, found %v", files)
		}
	})
}

func TestServedOutputsArePublishedWhenTheSessionEnds(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "session-{connection}.log")
	published := filepath.Join(dir, "session-0.log")
	yaml := replaceOnce(t, servedYAML(serveConfigYAML, "127.0.0.1:0", streamView,
		fmt.Sprintf("{name: log, function: {type: json_log, path: %q}}", log)),
		"run: {mode: serve, websocket: {address: \"127.0.0.1:0\", handle: /handle}}",
		"run: {mode: serve, websocket: {address: \"127.0.0.1:0\", handle: /handle}, pace_ms: 5}")
	server := servedHandler(t, yaml)

	connection, _, err := websocket.DefaultDialer.Dial(wsURLOf(server), nil)
	if err != nil {
		t.Fatal(err)
	}
	for range 4 {
		if _, _, err := connection.ReadMessage(); err != nil {
			t.Fatal(err)
		}
	}
	// Mid-session the log is being written, but only under its partial name.
	if _, err := os.Stat(published + ".partial"); err != nil {
		t.Errorf("mid-session the partial log should exist: %v", err)
	}
	if _, err := os.Stat(published); !errors.Is(err, os.ErrNotExist) {
		t.Error("the session's log was published before the session ended")
	}
	// The client leaving ends the session cleanly, so the log is published.
	connection.Close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(published); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the log was not published after the client left")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if entries := keyedEntries(t, published); len(entries) < 4 {
		t.Errorf("the published log has %d entries, want at least the 4 streamed", len(entries))
	}
	if _, err := os.Stat(published + ".partial"); !errors.Is(err, os.ErrNotExist) {
		t.Error("the partial log was left behind")
	}
}

// TestKilledRunHelper runs a config through the CLI when the kill test starts
// it as a child process.
func TestKilledRunHelper(t *testing.T) {
	config := os.Getenv("STOCHADEX_KILLED_RUN_CONFIG")
	if config == "" {
		t.Skip("runs only as a child of TestAKilledRunLeavesTheEarlierOutput")
	}
	if err := Execute([]string{"stochadex", "--config", config}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestAKilledRunLeavesTheEarlierOutput(t *testing.T) {
	dir := t.TempDir()
	log, nested := filepath.Join(dir, "run.log"), filepath.Join(dir, "nested.log")
	writeFileAt(t, log, earlierLog)
	writeFileAt(t, nested, earlierLog)
	start := func(sleepMs int) *exec.Cmd {
		child := exec.Command(os.Args[0], "-test.run=^TestKilledRunHelper$", "-test.count=1")
		child.Env = append(os.Environ(), "STOCHADEX_KILLED_RUN_CONFIG="+
			writeConfigPath(t, stagedYAML(0, sleepMs, log, nested)))
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		return child
	}

	// 30 steps at 200ms: killed once it is writing, well before it ends.
	child := start(200)
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(nested + ".partial"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			child.Process.Kill()
			t.Fatal("the child never started writing")
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	if err := child.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	child.Wait()
	for _, path := range []string{log, nested} {
		if data, err := os.ReadFile(path); err != nil || string(data) != earlierLog {
			t.Errorf("%s: want the earlier log intact after the kill, got %q (%v)", path, data, err)
		}
	}

	// A retry runs to completion and publishes, over the killed run's partials.
	retry := start(0)
	if err := retry.Wait(); err != nil {
		t.Fatalf("the retry failed: %v", err)
	}
	reference, referenceNested := filepath.Join(dir, "ref.log"), filepath.Join(dir, "ref-nested.log")
	unstagedLogs(t, stagedYAML(0, 0, reference, referenceNested))
	if readAll(t, log) != readAll(t, reference) || readAll(t, nested) != readAll(t, referenceNested) {
		t.Error("the retry's published logs differ from an unstaged run's")
	}
	if files := filesUnder(t, dir); strings.Join(files, ",") !=
		strings.Join([]string{"nested.log", "ref-nested.log", "ref.log", "run.log"}, ",") {
		t.Errorf("want only the logs after the retry, found %v", files)
	}
}

func TestAKilledEnsembleLeavesEveryMembersEarlierOutput(t *testing.T) {
	dir := t.TempDir()
	members := []string{filepath.Join(dir, "run-0.log"), filepath.Join(dir, "run-1.log")}
	for _, path := range members {
		writeFileAt(t, path, earlierLog)
	}
	yaml := fmt.Sprintf(`main:
  partitions:
  - {name: counter, iteration: {type: fails_at_step}, params: {fail_at: [0], sleep_ms: [200]}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 30}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
run: {mode: ensemble, seeds: [4, 9]}
outputs:
- {name: log, function: {type: json_log, path: %q}}
`, filepath.Join(dir, "run-{member}.log"))
	child := exec.Command(os.Args[0], "-test.run=^TestKilledRunHelper$", "-test.count=1")
	child.Env = append(os.Environ(), "STOCHADEX_KILLED_RUN_CONFIG="+writeConfigPath(t, yaml))
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, first := os.Stat(members[0] + ".partial")
		_, second := os.Stat(members[1] + ".partial")
		if first == nil && second == nil {
			break
		}
		if time.Now().After(deadline) {
			child.Process.Kill()
			t.Fatal("the members never started writing")
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	if err := child.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	child.Wait()
	for _, path := range members {
		if data, err := os.ReadFile(path); err != nil || string(data) != earlierLog {
			t.Errorf("%s: want the earlier log intact after the kill, got %q (%v)", path, data, err)
		}
	}
}

// noopStaged is a staged sink that does nothing.
type noopStaged struct{ simulator.NilOutputFunction }

func (noopStaged) Stage()        {}
func (noopStaged) Commit() error { return nil }
func (noopStaged) Abort()        {}

func TestStagingARunAllocatesNothing(t *testing.T) {
	// A run's main output and one nested sink are held in place.
	first, second := &noopStaged{}, &noopStaged{}
	if allocs := testing.AllocsPerRun(100, func() {
		var outputs stagedOutputs
		outputs.stage(first)
		outputs.stage(second)
		defer outputs.discard()
		outputs.finish(nil)
	}); allocs != 0 {
		t.Errorf("staging two sinks allocated %v times, want 0", allocs)
	}
}
