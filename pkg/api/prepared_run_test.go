package api

import (
	"sync/atomic"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// configureCounts counts Configure calls on configure_counter iterations.
// Generating a run's configs configures every iteration, so the count is how
// many times a run was generated.
var configureCounts atomic.Int64

type configureCounterIteration struct{}

func (c *configureCounterIteration) Configure(int, *simulator.Settings) { configureCounts.Add(1) }

func (c *configureCounterIteration) Iterate(
	params *simulator.Params,
	_ int,
	_ []*simulator.StateHistory,
	_ *simulator.CumulativeTimestepsHistory,
) []float64 {
	return []float64{params.Get("value")[0]}
}

func init() {
	RegisterIteration("configure_counter", func(simulator.ComponentSpec) (simulator.Iteration, error) {
		return &configureCounterIteration{}, nil
	})
}

const configureCounterYAML = `main:
  partitions:
  - {name: counted, iteration: {type: configure_counter}, params: {value: [1.0]}, init_state_values: [0.0], state_history_depth: 1, seed: 1}
  simulation:
    output_condition: {type: every_step}
    output_function: {type: nil}
    termination_condition: {type: number_of_steps, max_steps: 3}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`

// TestARunGeneratesItsConfigsOnce guards against generating a run twice — once
// to validate its wiring and again to run it — which reconfigured every
// iteration, re-creating each one's random source: 3 allocations per partition
// per run, found by benchmarking against v0.19.0.
func TestARunGeneratesItsConfigsOnce(t *testing.T) {
	runs := map[string]func(config *ApiRunConfig) error{
		"Run": func(config *ApiRunConfig) error {
			Run(config, &SocketConfig{})
			return nil
		},
		"RunToStorage": func(config *ApiRunConfig) error {
			_, err := RunToStorage(config)
			return err
		},
		"RunWith, writing the config's outputs": func(config *ApiRunConfig) error {
			_, err := RunWith(config, CaptureView("all", nil), WithConfigOutputs())
			return err
		},
	}
	for name, run := range runs {
		t.Run(name+" configures each iteration once", func(t *testing.T) {
			config := writeConfig(t, configureCounterYAML)
			before := configureCounts.Load()
			if err := run(config); err != nil {
				t.Fatal(err)
			}
			if n := configureCounts.Load() - before; n != 1 {
				t.Errorf("the iteration was configured %d times in one run, want 1", n)
			}
		})
	}
	t.Run("the run still records what it should", func(t *testing.T) {
		result, err := RunToStorage(writeConfig(t, configureCounterYAML))
		if err != nil {
			t.Fatal(err)
		}
		if rows := result.Storage.GetValues("counted"); len(rows) != 4 || rows[3][0] != 1 {
			t.Errorf("recorded %v, want 4 rows ending in [1]", rows)
		}
	})
}
