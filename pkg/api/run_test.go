package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// deadlockPartition builds a minimal partition whose only wiring is the given
// within-step params_from_upstream links, so a mutual pair forms a cycle.
func deadlockPartition(
	name string,
	upstream map[string]simulator.NamedUpstreamConfig,
) *simulator.PartitionConfig {
	return &simulator.PartitionConfig{
		Name:               name,
		InitStateValues:    []float64{0.0},
		StateHistoryDepth:  1,
		ParamsFromUpstream: upstream,
	}
}

func genFrom(configs ...*simulator.PartitionConfig) *simulator.ConfigGenerator {
	gen := simulator.NewConfigGenerator()
	for _, c := range configs {
		gen.SetPartition(c)
	}
	return gen
}

func TestCheckForDeadlock(t *testing.T) {
	t.Run("mutual within-step coupling is flagged with both names", func(t *testing.T) {
		gen := genFrom(
			deadlockPartition("prey", map[string]simulator.NamedUpstreamConfig{
				"pred_val": {Upstream: "predator"},
			}),
			deadlockPartition("predator", map[string]simulator.NamedUpstreamConfig{
				"prey_val": {Upstream: "prey"},
			}),
		)
		err := CheckForDeadlock(gen)
		if err == nil {
			t.Fatal("expected a deadlock error for a mutual params_from_upstream cycle")
		}
		msg := err.Error()
		for _, name := range []string{"prey", "predator"} {
			if !strings.Contains(msg, name) {
				t.Errorf("error message should name partition %q, got: %s", name, msg)
			}
		}
	})

	t.Run("a self within-step cycle is flagged", func(t *testing.T) {
		gen := genFrom(
			deadlockPartition("loop", map[string]simulator.NamedUpstreamConfig{
				"own": {Upstream: "loop"},
			}),
		)
		if err := CheckForDeadlock(gen); err == nil {
			t.Fatal("expected a deadlock error for a self params_from_upstream loop")
		}
	})

	t.Run("acyclic within-step wiring passes", func(t *testing.T) {
		// driver -> responder within-step, no back edge: a legal computation order.
		gen := genFrom(
			deadlockPartition("driver", nil),
			deadlockPartition("responder", map[string]simulator.NamedUpstreamConfig{
				"drv": {Upstream: "driver"},
			}),
		)
		if err := CheckForDeadlock(gen); err != nil {
			t.Errorf("acyclic within-step wiring should pass, got: %v", err)
		}
	})

	t.Run("a three-partition within-step cycle is flagged", func(t *testing.T) {
		gen := genFrom(
			deadlockPartition("a", map[string]simulator.NamedUpstreamConfig{"in": {Upstream: "c"}}),
			deadlockPartition("b", map[string]simulator.NamedUpstreamConfig{"in": {Upstream: "a"}}),
			deadlockPartition("c", map[string]simulator.NamedUpstreamConfig{"in": {Upstream: "b"}}),
		)
		if err := CheckForDeadlock(gen); err == nil {
			t.Error("expected a deadlock error for a 3-partition within-step ring")
		}
	})

	t.Run("a three-partition ring with one lag edge passes", func(t *testing.T) {
		// a -> b -> c within-step, and c -> a via a state-history (lag) read: the
		// lag edge breaks the ring, so no deadlock.
		gen := genFrom(
			&simulator.PartitionConfig{
				Name: "a", InitStateValues: []float64{0.0}, StateHistoryDepth: 1,
				ParamsAsPartitions: map[string][]string{"peer": {"c"}},
			},
			deadlockPartition("b", map[string]simulator.NamedUpstreamConfig{"in": {Upstream: "a"}}),
			deadlockPartition("c", map[string]simulator.NamedUpstreamConfig{"in": {Upstream: "b"}}),
		)
		if err := CheckForDeadlock(gen); err != nil {
			t.Errorf("a ring broken by a lag edge should pass, got: %v", err)
		}
	})

	t.Run("a mutual dependency broken by a lag-1 read passes", func(t *testing.T) {
		// predator reads prey within-step; prey reads predator via a state-history
		// read (params_as_partitions), which is lag-1 and so breaks the cycle.
		gen := genFrom(
			&simulator.PartitionConfig{
				Name:              "prey",
				InitStateValues:   []float64{0.0},
				StateHistoryDepth: 1,
				ParamsAsPartitions: map[string][]string{
					"pred_partition": {"predator"},
				},
			},
			deadlockPartition("predator", map[string]simulator.NamedUpstreamConfig{
				"prey_val": {Upstream: "prey"},
			}),
		)
		if err := CheckForDeadlock(gen); err != nil {
			t.Errorf("a cycle broken by a lag-1 read should pass, got: %v", err)
		}
	})
}

// dataOnlyEnsembleYAML is a file-backed, data-only (expressions) config with an
// ensemble run block. Its simulation components are Go strings (yaml:"-"), so they
// are supplied separately in tests, mirroring what codegen's ExtraCode does.
const dataOnlyEnsembleYAML = `main:
  partitions:
  - name: growth
    params: {rate: [0.05], noise: [0.2]}
    init_state_values: [10.0]
    state_history_depth: 1
    seed: 1
  expressions:
  - partition: growth
    fields: [{name: x}]
    outputs: ["x + rate * x * dt + noise * x * shared(normal(0,1)) * sqrt(dt)"]
run:
  mode: ensemble
  seeds: [11, 22, 33]
`

// testSim builds the resolved SimulationConfig a loaded config lacks (its
// simulation block is yaml:"-").
func testSim() *simulator.SimulationConfig {
	return &simulator.SimulationConfig{
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 10},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
	}
}

// writeConfig writes contents to a temp file and loads it, so sourcePath is set
// exactly as the CLI path sets it.
func writeConfig(t *testing.T, contents string) *ApiRunConfig {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}
	return LoadApiRunConfigFromYaml(path)
}

func TestRunModeParsing(t *testing.T) {
	config := writeConfig(t, dataOnlyEnsembleYAML)
	if config.Run.Mode != "ensemble" {
		t.Errorf("run.mode: want ensemble, got %q", config.Run.Mode)
	}
	if want := []uint64{11, 22, 33}; len(config.Run.Seeds) != len(want) {
		t.Fatalf("run.seeds: want %v, got %v", want, config.Run.Seeds)
	}
}

func TestEnsembleRuns(t *testing.T) {
	t.Run("one member per seed, trajectories vary by seed", func(t *testing.T) {
		config := writeConfig(t, dataOnlyEnsembleYAML)
		runs, err := ensembleRuns(config, testSim())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(runs) != 3 {
			t.Fatalf("want 3 members, got %d", len(runs))
		}
		finals := make(map[float64]bool)
		for i, run := range runs {
			if run.Seed != config.Run.Seeds[i] {
				t.Errorf("member %d: seed %d != configured %d", i, run.Seed, config.Run.Seeds[i])
			}
			values := run.Storage.GetValues("growth")
			if len(values) == 0 {
				t.Fatalf("member %d recorded no growth values", i)
			}
			finals[values[len(values)-1][0]] = true
		}
		if len(finals) != 3 {
			t.Errorf("expected 3 distinct final values across seeds, got %d", len(finals))
		}
	})

	t.Run("deterministic across repeated runs", func(t *testing.T) {
		config := writeConfig(t, dataOnlyEnsembleYAML)
		first, err := ensembleRuns(config, testSim())
		if err != nil {
			t.Fatal(err)
		}
		second, err := ensembleRuns(config, testSim())
		if err != nil {
			t.Fatal(err)
		}
		for i := range first {
			a := first[i].Storage.GetValues("growth")
			b := second[i].Storage.GetValues("growth")
			if a[len(a)-1][0] != b[len(b)-1][0] {
				t.Errorf("member %d not deterministic: %v vs %v", i, a[len(a)-1], b[len(b)-1])
			}
		}
	})

	t.Run("empty seeds is rejected", func(t *testing.T) {
		config := writeConfig(t, dataOnlyEnsembleYAML)
		config.Run.Seeds = nil
		if _, err := ensembleRuns(config, testSim()); err == nil {
			t.Error("expected an error for empty run.seeds")
		}
	})

	t.Run("in-memory config (no source path) is rejected", func(t *testing.T) {
		config := &ApiRunConfig{Run: RunModeConfig{Mode: "ensemble", Seeds: []uint64{1}}}
		if _, err := ensembleRuns(config, testSim()); err == nil {
			t.Error("expected an error when sourcePath is empty")
		}
	})
}

func TestFullyDataResolution(t *testing.T) {
	const fullyData = `main:
  partitions:
  - name: growth
    params: {rate: [0.05]}
    init_state_values: [10.0]
    state_history_depth: 1
    seed: 42
  expressions:
  - partition: growth
    fields: [{name: x}]
    outputs: ["x + rate * x * dt"]
  simulation:
    output_condition: {type: every_step}
    output_function: {type: nil}
    termination_condition: {type: number_of_steps, max_steps: 8}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(fullyData), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("load resolves the data-spec simulation components", func(t *testing.T) {
		config := LoadApiRunConfigFromYaml(path)
		sim := config.Main.Simulation
		if sim.OutputCondition == nil || sim.OutputFunction == nil ||
			sim.TerminationCondition == nil || sim.TimestepFunction == nil {
			t.Error("all four simulation components should be resolved at load")
		}
		if got := sim.TerminationCondition.(*simulator.NumberOfStepsTerminationCondition).MaxNumberOfSteps; got != 8 {
			t.Errorf("max_steps = %d, want 8", got)
		}
	})

	t.Run("a Go-string iteration config is rejected at load", func(t *testing.T) {
		// The Go-expression config path has been removed: a component given as a
		// scalar Go string (rather than a {type: ...} data spec) must no longer load.
		goPath := filepath.Join(t.TempDir(), "go.yaml")
		goYAML := `main:
  partitions:
  - name: w
    iteration: "&continuous.WienerProcessIteration{}"
    params: {variances: [1.0]}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 1
  simulation:
    output_condition: {type: every_step}
    output_function: {type: stdout}
    termination_condition: {type: number_of_steps, max_steps: 5}
    timestep_function: {type: constant, stepsize: 1.0}
`
		if err := os.WriteFile(goPath, []byte(goYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		if !didPanic(func() { LoadApiRunConfigFromYaml(goPath) }) {
			t.Error("a Go-string iteration config should be rejected at load")
		}
	})
}
