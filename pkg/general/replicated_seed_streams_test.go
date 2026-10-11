package general

import (
	"fmt"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// TestReplicatedRunGivesEveryStreamItsOwnSeed checks that no two (step,
// replica, inner partition) triples share a random stream. With an XOR-linear
// seed derivation the nesting commutes: step k's replica r equals step r's
// replica k, and within one step replica 0's partition 1 equals replica 1's
// partition 0.
func TestReplicatedRunGivesEveryStreamItsOwnSeed(t *testing.T) {
	const partitions = 3
	const replicas = 4
	const steps = 5
	// Each replica outputs one partition, so one replicated run per inner
	// partition reads every stream. They share a seed, so they derive the same
	// seeds as each other.
	runs := make([]*ReplicatedSimulationRunIteration, partitions)
	for reported := range runs {
		build := func() *ReplicatedModel {
			model := &ReplicatedModel{
				Simulation: &simulator.SimulationConfig{
					OutputCondition:      &simulator.NilOutputCondition{},
					OutputFunction:       &simulator.NilOutputFunction{},
					TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 1},
					TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
					InitTimeValue:        0,
				},
				OutputPartition: fmt.Sprintf("probe_%d", reported),
			}
			for i := 0; i < partitions; i++ {
				model.Partitions = append(model.Partitions, &simulator.PartitionConfig{
					Name:              fmt.Sprintf("probe_%d", i),
					Iteration:         &seedProbeIteration{},
					Params:            simulator.NewParams(map[string][]float64{}),
					InitStateValues:   []float64{0},
					StateHistoryDepth: 1,
					Seed:              uint64(i + 1),
				})
			}
			return model
		}
		run, err := NewReplicatedSimulationRunIteration(build, replicas, 0)
		if err != nil {
			t.Fatal(err)
		}
		run.Configure(0, &simulator.Settings{
			Iterations: []simulator.IterationSettings{{Name: "replicated", Seed: 3}},
		})
		runs[reported] = run
	}

	params := simulator.NewParams(map[string][]float64{})
	seen := make(map[float64]string)
	for step := 1; step <= steps; step++ {
		for partition, run := range runs {
			out := run.Iterate(&params, 0, nil,
				&simulator.CumulativeTimestepsHistory{CurrentStepNumber: step})
			for replica, seed := range out {
				key := fmt.Sprintf("step %d replica %d partition %d",
					step, replica, partition)
				if other, ok := seen[seed]; ok {
					t.Errorf("%s shares a random stream with %s", key, other)
				}
				seen[seed] = key
			}
		}
	}
}
