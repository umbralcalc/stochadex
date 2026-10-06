package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// macroTwinYAML is the core-language twin of macroConfigYAML (the rolling mean
// and variance macros over a Normal stream), from the Phase 2 expansion spike
// (PLAN.md §4.2), where it reproduces the macro path exactly. It runs as one live
// simulation, so a view fed by it is what a live run of the same model outputs.
const macroTwinYAML = `main:
  partitions:
  - name: data_stream
    iteration: {type: data_generation, likelihood: {type: normal, allow_default_covariance_fallback: true}}
    params:
      mean: [1.8, 5.0]
      covariance_matrix: [2.5, 0.0, 0.0, 9.0]
    init_state_values: [1.3, 8.3]
    state_history_depth: 150
    seed: 291
  - name: rolling_mean
    iteration: {type: values_function_vector_mean, function: data_values, kernel: {type: exponential}}
    params:
      data_values_indices: [0, 1]
      exponential_weighting_timescale: [100.0]
    params_from_upstream:
      latest_data_values: {upstream: data_stream, indices: [0, 1]}
    params_as_partitions:
      data_values_partition: [data_stream]
    init_state_values: [0.0, 0.0]
    state_history_depth: 150
    seed: 0
  - name: rolling_var
    iteration: {type: values_function_vector_mean, function: data_values_variance, kernel: {type: exponential}}
    params:
      data_values_indices: [0, 1]
      exponential_weighting_timescale: [100.0]
      subtract_from_normalisation: [1]
    params_from_upstream:
      latest_data_values: {upstream: data_stream, indices: [0, 1]}
      mean: {upstream: rolling_mean, indices: [0, 1]}
    params_as_partitions:
      data_values_partition: [data_stream]
    init_state_values: [0.0, 0.0]
    state_history_depth: 1
    seed: 0
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 500}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`

// viewsBlock builds an outputs: block of json_log views under dir, one per
// named condition, with file names prefixed by prefix.
func viewsBlock(dir, prefix string, conditions map[string]string) string {
	block := "outputs:\n"
	for name, condition := range conditions {
		block += fmt.Sprintf("- {name: %s, condition: %s, function: {type: json_log, path: %q}}\n",
			name, condition, filepath.Join(dir, prefix+"-"+name+".log"))
	}
	return block
}

func TestMacroResultsThroughOutputViews(t *testing.T) {
	conditions := map[string]string{
		"all":    "{type: every_step}",
		"var":    "{type: only_given_partitions, partitions: [rolling_var]}",
		"sparse": "{type: every_n_steps, n: 7}",
	}

	t.Run("each view gets exactly what a live run of the same model gives it", func(t *testing.T) {
		// The macro path computes its result and replays it through the views; the
		// twin runs live. Matching files mean the replay applies every condition,
		// step counting included, exactly as a live run does.
		dir := t.TempDir()
		var macroErr, twinErr error
		printed := captureStdout(t, func() {
			macroErr = runChecked(writeConfig(t, macroConfigYAML+viewsBlock(dir, "macro", conditions)), &SocketConfig{})
		})
		captureStdout(t, func() {
			twinErr = runChecked(writeConfig(t, macroTwinYAML+viewsBlock(dir, "twin", conditions)), &SocketConfig{})
		})
		if macroErr != nil || twinErr != nil {
			t.Fatalf("runs failed: macro %v, twin %v", macroErr, twinErr)
		}
		for name := range conditions {
			assertSameEntries(t, "view "+name,
				keyedEntries(t, filepath.Join(dir, "macro-"+name+".log")),
				keyedEntries(t, filepath.Join(dir, "twin-"+name+".log")))
		}
		// The views genuinely differ: 3 partitions x 501 rows, rolling_var's 501,
		// and every 7th step of all three.
		counts := map[string]int{"all": 3 * 501, "var": 501, "sparse": 3 * 72}
		for name, want := range counts {
			if got := len(keyedEntries(t, filepath.Join(dir, "macro-"+name+".log"))); got != want {
				t.Errorf("view %s has %d entries, want %d", name, got, want)
			}
		}
		if printed != "" {
			t.Errorf("with outputs: declared, results go to the views, not stdout; printed %d bytes",
				len(printed))
		}
	})

	t.Run("a live macro's results reach the views", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "mean.log")
		config := writeConfig(t, readFile(t, "../../cfg/example_evolution_strategy_config.yaml")+
			fmt.Sprintf("outputs:\n- {name: mean, condition: {type: only_given_partitions, partitions: [es_mean]}, function: {type: json_log, path: %q}}\n", path))
		if err := runChecked(config, &SocketConfig{}); err != nil {
			t.Fatal(err)
		}
		reference, err := RunToStorage(LoadApiRunConfigFromYaml(config.sourcePath))
		if err != nil {
			t.Fatal(err)
		}
		entries := readLogEntries(t, path)
		want := reference.Storage.GetValues("es_mean")
		times := reference.Storage.GetTimes()
		if len(entries) != len(want) {
			t.Fatalf("got %d entries, want es_mean's %d rows", len(entries), len(want))
		}
		for i, entry := range entries {
			if entry.PartitionName != "es_mean" || entry.CumulativeTimesteps != times[i] ||
				!floats.Equal(entry.State, want[i]) {
				t.Fatalf("entry %d = %+v, want es_mean at %v = %v", i, entry, times[i], want[i])
			}
		}
	})

	t.Run("RunToStorage writes none of a macro config's views", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "view.log")
		writeSentinel(t, path)
		config := writeConfig(t, macroConfigYAML+fmt.Sprintf(
			"outputs:\n- {name: log, function: {type: json_log, path: %q}}\n", path))
		if _, err := RunToStorage(config); err != nil {
			t.Fatal(err)
		}
		assertSentinelIntact(t, path, "RunToStorage")
	})

	t.Run("each view's sink is opened and finalized around the replay", func(t *testing.T) {
		receiver, server := newPushReceiver(t)
		defer server.Close()
		url := "ws" + strings.TrimPrefix(server.URL, "http")
		config := writeConfig(t, macroConfigYAML+fmt.Sprintf(
			"outputs:\n- {name: dash, condition: {type: only_given_partitions, partitions: [rolling_mean]}, function: {type: websocket, url: %q}}\n", url))
		if err := runChecked(config, &SocketConfig{}); err != nil {
			t.Fatal(err)
		}
		receiver.waitForConnections(t, 1)
		if n := len(receiver.connections[0]); n != 501 {
			t.Errorf("dash got %d messages, want rolling_mean's 501", n)
		}
		for _, state := range receiver.connections[0] {
			if state.PartitionName != "rolling_mean" {
				t.Fatalf("dash received %s, which its condition excludes", state.PartitionName)
			}
		}
		if receiver.closeCodes[0] != websocket.CloseNormalClosure {
			t.Errorf("connection ended with code %d, want a normal close from Finalize",
				receiver.closeCodes[0])
		}
	})
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

// recordingSink records the order of the calls it receives.
type recordingSink struct {
	calls          []string
	configuredWith []string
}

func (r *recordingSink) Configure(settings *simulator.Settings) {
	r.calls = append(r.calls, "configure")
	for _, iteration := range settings.Iterations {
		r.configuredWith = append(r.configuredWith, iteration.Name)
	}
}

func (r *recordingSink) Output(name string, _ []float64, _ float64) {
	r.calls = append(r.calls, "output:"+name)
}

type finalizingRecordingSink struct{ recordingSink }

func (f *finalizingRecordingSink) Finalize() { f.calls = append(f.calls, "finalize") }

func TestReplayThroughViewsLifecycle(t *testing.T) {
	storage := simulator.NewStateTimeStorage()
	storage.SetValues("b", [][]float64{{1}, {2}, {3}})
	storage.SetValues("a", [][]float64{{10}, {20}, {30}})
	storage.SetTimes([]float64{0, 1, 2})

	finalizing := &finalizingRecordingSink{}
	plain := &recordingSink{}
	replayThroughViews(storage, &simulator.OutputViews{Views: []simulator.OutputView{
		{Name: "f", Condition: &simulator.EveryStepOutputCondition{}, Function: finalizing},
		{Name: "p", Condition: &simulator.EveryNStepsOutputCondition{N: 2}, Function: plain},
	}})

	wantFinalizing := []string{"configure",
		"output:a", "output:b", "output:a", "output:b", "output:a", "output:b", "finalize"}
	if strings.Join(finalizing.calls, ",") != strings.Join(wantFinalizing, ",") {
		t.Errorf("finalizing sink calls %v, want %v (configure first, rows in name order, finalize last)",
			finalizing.calls, wantFinalizing)
	}
	// every_n_steps(2) selects steps 0 and 2; the plain sink is never finalized.
	wantPlain := []string{"configure", "output:a", "output:b", "output:a", "output:b"}
	if strings.Join(plain.calls, ",") != strings.Join(wantPlain, ",") {
		t.Errorf("plain sink calls %v, want %v", plain.calls, wantPlain)
	}
	if strings.Join(finalizing.configuredWith, ",") != "a,b" {
		t.Errorf("Configure should name every partition, got %v", finalizing.configuredWith)
	}
}
