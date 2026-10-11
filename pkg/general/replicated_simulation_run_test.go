package general

import (
	"fmt"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// echoModel is one replica's model: a partition echoing its two-wide
// param_values, and a Wiener walk, so a replica's output can carry its
// parameters or its noise.
func echoModel(output string, forwarding map[string][]int) func() *ReplicatedModel {
	return func() *ReplicatedModel {
		return &ReplicatedModel{
			Partitions: []*simulator.PartitionConfig{{
				Name:              "echo",
				Iteration:         &ParamValuesIteration{},
				Params:            simulator.NewParams(map[string][]float64{"param_values": {0, 0}}),
				InitStateValues:   []float64{0, 0},
				StateHistoryDepth: 1,
			}, {
				Name:              "walk",
				Iteration:         &continuous.WienerProcessIteration{},
				Params:            simulator.NewParams(map[string][]float64{"variances": {1}}),
				InitStateValues:   []float64{0},
				StateHistoryDepth: 1,
			}},
			Simulation: &simulator.SimulationConfig{
				OutputCondition:      &simulator.NilOutputCondition{},
				OutputFunction:       &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 2},
				TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1},
			},
			OutputPartition: output,
			ParamForwarding: forwarding,
		}
	}
}

// replicatedRun runs a width-wide replicated partition, fed replica_params by
// a constant partition when there are any, for steps outer steps, and returns
// its rows.
func replicatedRun(t *testing.T, iteration *ReplicatedSimulationRunIteration, params []float64, width, steps int) [][]float64 {
	t.Helper()
	generator := simulator.NewConfigGenerator()
	generator.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: steps},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1},
	})
	replicas := &simulator.PartitionConfig{
		Name:              "replicas",
		Iteration:         iteration,
		Params:            simulator.NewParams(map[string][]float64{}),
		InitStateValues:   make([]float64, width),
		StateHistoryDepth: 1,
		Seed:              11,
	}
	if len(params) > 0 {
		generator.SetPartition(&simulator.PartitionConfig{
			Name:              "params",
			Iteration:         &ConstantValuesIteration{},
			Params:            simulator.NewParams(map[string][]float64{}),
			InitStateValues:   params,
			StateHistoryDepth: 1,
		})
		replicas.Params.Set("replica_params", make([]float64, len(params)))
		replicas.ParamsFromUpstream = map[string]simulator.NamedUpstreamConfig{
			"replica_params": {Upstream: "params"}}
	}
	generator.SetPartition(replicas)
	settings, implementations := generator.GenerateConfigs()
	store := simulator.NewStateTimeStorage()
	implementations.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	t.Run("passes the harnesses", func(t *testing.T) {
		if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
			t.Fatal(err)
		}
	})
	settings, implementations = generator.GenerateConfigs()
	implementations.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	simulator.NewPartitionCoordinator(settings, implementations).Run()
	return store.GetValues("replicas")
}

func TestReplicatedSimulationRunIteration(t *testing.T) {
	t.Run("each replica gets its own model and its own slice of the parameters", func(t *testing.T) {
		iteration, err := NewReplicatedSimulationRunIteration(
			echoModel("echo", map[string][]int{"echo/param_values": {1, 0}}), 3, 2)
		if err != nil {
			t.Fatal(err)
		}
		for i := range iteration.Simulations {
			for j := i + 1; j < len(iteration.Simulations); j++ {
				if iteration.Simulations[i] == iteration.Simulations[j] {
					t.Fatalf("replicas %d and %d share a model", i, j)
				}
			}
		}
		rows := replicatedRun(t, iteration, []float64{1, 2, 3, 4, 5, 6}, 6, 2)
		// The forwarding swaps each replica's pair, and the rows concatenate.
		if want := []float64{2, 1, 4, 3, 6, 5}; !floats.Equal(rows[len(rows)-1], want) {
			t.Errorf("got %v, want %v", rows[len(rows)-1], want)
		}
	})

	t.Run("no two (step, replica) runs share a stream", func(t *testing.T) {
		iteration, err := NewReplicatedSimulationRunIteration(echoModel("walk", nil), 3, 0)
		if err != nil {
			t.Fatal(err)
		}
		// A one-wide output: the walk's final value per replica. Steps 1-3 and
		// replicas 0-2 include (step 1, replica 2) and (step 2, replica 1),
		// which a commuting seed derivation would put on one stream.
		rows := replicatedRun(t, iteration, nil, 3, 3)
		seen := map[float64]string{}
		for step, row := range rows[1:] {
			for replica, value := range row {
				run := fmt.Sprintf("(step %d, replica %d)", step+1, replica)
				if other, ok := seen[value]; ok {
					t.Errorf("%s drew %v, as %s did", run, value, other)
				}
				seen[value] = run
			}
		}
	})

	cases := []struct {
		name             string
		output           string
		forwarding       map[string][]int
		replicas, params int
		want             string
	}{
		{"no replicas", "echo", nil, 0, 1, "replicas must be > 0"},
		{"a negative parameter count", "echo", nil, 1, -1, "params_per_replica must be >= 0"},
		{"an unknown output partition", "absent", nil, 1, 1, `output partition "absent" is not in the replicated model`},
		{"a malformed forwarding key", "echo", map[string][]int{"echo": {0}}, 1, 1, "must be \"partition_name/param_name\""},
		{"a forwarding key with an empty param", "echo", map[string][]int{"echo/": {0}}, 1, 1, "must be \"partition_name/param_name\""},
		{"forwarding to an unknown partition", "echo", map[string][]int{"absent/x": {0}}, 1, 1, `names partition "absent"`},
		{"forwarding a parameter past the replica's own", "echo", map[string][]int{"echo/param_values": {1}}, 1, 1, "sends parameter 1, but each replica has 1"},
		{"forwarding a negative parameter", "echo", map[string][]int{"echo/param_values": {-1}}, 1, 1, "sends parameter -1"},
	}
	for _, c := range cases {
		t.Run(c.name+" is an error", func(t *testing.T) {
			_, err := NewReplicatedSimulationRunIteration(echoModel(c.output, c.forwarding), c.replicas, c.params)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected an error containing %q, got %v", c.want, err)
			}
		})
	}

	t.Run("Configure refuses an iteration built without the constructor", func(t *testing.T) {
		for want, iteration := range map[string]*ReplicatedSimulationRunIteration{
			"no replica simulations":        {},
			"ParamsPerReplica must be >= 0": {Simulations: []*simulator.ReentrantSimulation{nil}, ParamsPerReplica: -1},
		} {
			func() {
				defer func() {
					if message, _ := recover().(string); !strings.Contains(message, want) {
						t.Errorf("Configure should panic with %q, got %q", want, message)
					}
				}()
				iteration.Configure(0, &simulator.Settings{Iterations: []simulator.IterationSettings{{}}})
			}()
		}
	})
}
