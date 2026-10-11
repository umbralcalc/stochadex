package api

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// panickingYAML is a 30-step run in which partition "counter" panics at step
// failAt (never when 0). It sits mid-graph: it reads "a" within the step, and
// "c" reads it alongside a second upstream "b", so a panic there has to stop
// a consumer that is waiting on it while "b" has already sent. "walk" is
// independent of all of them. The order suits inline execution (every upstream
// precedes its consumers).
func panickingYAML(failAt int, strategy string) string {
	strategyLine := ""
	if strategy != "" {
		strategyLine = fmt.Sprintf("    execution_strategy: {type: %s}\n", strategy)
	}
	return fmt.Sprintf(`main:
  partitions:
  - {name: a, iteration: {type: fails_at_step}, params: {fail_at: [0], sleep_ms: [0]}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
  - {name: b, iteration: {type: fails_at_step}, params: {fail_at: [0], sleep_ms: [0]}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
  - {name: counter, iteration: {type: fails_at_step}, params: {fail_at: [%d], sleep_ms: [0]}, params_from_upstream: {from_a: {upstream: a}}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
  - {name: c, iteration: {type: fails_at_step}, params: {fail_at: [0], sleep_ms: [0]}, params_from_upstream: {from_counter: {upstream: counter}, from_b: {upstream: b}}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
  - {name: walk, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [0.0], state_history_depth: 1, seed: 5}
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 30}
    timestep_function: {type: constant, stepsize: 1.0}
%s    init_time_value: 0.0
outputs:
- {name: none, function: {type: nil}}
`, failAt, strategyLine)
}

// assertNoLeakedGoroutines fails unless the goroutine count returns to before
// within a second: every worker of a failed run must have finished or stopped.
func assertNoLeakedGoroutines(t *testing.T, before int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			buf := make([]byte, 1<<16)
			t.Fatalf("goroutines leaked: %d before, %d after\n%s",
				before, runtime.NumGoroutine(), buf[:runtime.Stack(buf, true)])
		}
		time.Sleep(time.Millisecond)
	}
}

// assertPartitionPanicError checks err is the ErrRuntime the failing counter
// partition's panic should come back as, carrying the partition's name and its
// panic message, with the *simulator.PartitionPanic reachable through it.
func assertPartitionPanicError(t *testing.T, err error) {
	t.Helper()
	if KindOf(err) != ErrRuntime {
		t.Fatalf("want an ErrRuntime, got kind %d: %v", KindOf(err), err)
	}
	const want = `simulation failed: partition "counter" panicked: failing at step 7`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("want the error to contain %q, got %q", want, err.Error())
	}
	var partitionPanic *simulator.PartitionPanic
	if !errors.As(err, &partitionPanic) {
		t.Fatalf("want a *simulator.PartitionPanic in the chain, got %v", err)
	}
	if partitionPanic.Partition != "counter" || partitionPanic.Value != "failing at step 7" {
		t.Errorf("want counter's panic, got partition %q value %v",
			partitionPanic.Partition, partitionPanic.Value)
	}
	if !strings.Contains(string(partitionPanic.Stack), "failsAtStepIteration") {
		t.Errorf("want the panicking goroutine's stack, got:\n%s", partitionPanic.Stack)
	}
}

func TestPartitionPanicComesBackAsAnError(t *testing.T) {
	strategies := []string{"", "spawn_per_step", "persistent_worker", "inline"}
	for _, strategy := range strategies {
		name := strategy
		if name == "" {
			name = "default"
		}
		t.Run(name+": the run completes when nothing panics", func(t *testing.T) {
			result, err := RunToStorage(writeConfig(t, panickingYAML(0, strategy)))
			if err != nil {
				t.Fatal(err)
			}
			if got := len(result.Storage.GetValues("c")); got != 31 {
				t.Errorf("want 31 rows of c, got %d", got)
			}
		})

		t.Run(name+": RunToStorage returns ErrRuntime naming the partition", func(t *testing.T) {
			before := runtime.NumGoroutine()
			result, err := RunToStorage(writeConfig(t, panickingYAML(7, strategy)))
			if result != nil {
				t.Errorf("want no result from a failed run, got %+v", result)
			}
			assertPartitionPanicError(t, err)
			assertNoLeakedGoroutines(t, before)
		})

		t.Run(name+": RunWith returns ErrRuntime naming the partition", func(t *testing.T) {
			before := runtime.NumGoroutine()
			_, err := RunWith(writeConfig(t, panickingYAML(7, strategy)),
				CaptureView("all", nil))
			assertPartitionPanicError(t, err)
			assertNoLeakedGoroutines(t, before)
		})

		t.Run(name+": Execute returns ErrRuntime naming the partition", func(t *testing.T) {
			before := runtime.NumGoroutine()
			path := writeConfigPath(t, panickingYAML(7, strategy))
			var err error
			captureStdout(t, func() { err = Execute([]string{"stochadex", "--config", path}) })
			assertPartitionPanicError(t, err)
			if ExitCode(err) != 70 {
				t.Errorf("want exit code 70, got %d", ExitCode(err))
			}
			assertNoLeakedGoroutines(t, before)
		})
	}

	for _, strategy := range strategies {
		t.Run("a paced, stepped run returns ErrRuntime under "+strategy, func(t *testing.T) {
			// Pacing routes the run through runSteps' stepper loop rather
			// than Run.
			config := writeConfig(t, panickingYAML(7, strategy))
			before := runtime.NumGoroutine()
			prepared, err := preparedMainGenerator(config)
			if err != nil {
				t.Fatal(err)
			}
			err = runRecovered(func() error {
				return runCoordinator(
					simulator.NewPartitionCoordinator(prepared.settings, prepared.implementations),
					prepared.feeds, time.Microsecond)
			})
			assertPartitionPanicError(t, err)
			assertNoLeakedGoroutines(t, before)
		})
	}
}

func TestUndeclaredGroupErrorComesBackAsAnError(t *testing.T) {
	// The motivating case: values_grouped_aggregation with undeclared_groups:
	// error panics mid-run on group 3, which only appears at step 1, from a
	// partition fed within the step by two upstreams.
	for _, strategy := range []string{"", "spawn_per_step", "persistent_worker", "inline"} {
		t.Run("strategy "+strategy, func(t *testing.T) {
			yaml := groupedYAML(", undeclared_groups: error", "[0, 0]")
			if strategy != "" {
				yaml = strings.Replace(yaml, "    init_time_value: 0.0\n",
					"    init_time_value: 0.0\n    execution_strategy: {type: "+strategy+"}\n", 1)
			}
			before := runtime.NumGoroutine()
			_, err := RunToStorage(writeConfig(t, yaml))
			if KindOf(err) != ErrRuntime {
				t.Fatalf("want an ErrRuntime, got kind %d: %v", KindOf(err), err)
			}
			const want = `simulation failed: partition "agg" panicked: ` +
				`values_grouped_aggregation: group (3.0) is not declared`
			if !strings.Contains(err.Error(), want) {
				t.Errorf("want the error to contain %q, got %q", want, err.Error())
			}
			assertNoLeakedGoroutines(t, before)
		})
	}
}
