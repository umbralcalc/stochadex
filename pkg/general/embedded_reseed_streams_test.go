package general

import (
	"fmt"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// seedProbeIteration reports the seed its partition was configured with, so a
// test can see which random stream each inner partition was handed. The top 53
// bits are kept, which a float64 holds exactly.
type seedProbeIteration struct{ seed float64 }

func (s *seedProbeIteration) Configure(partitionIndex int, settings *simulator.Settings) {
	s.seed = float64(settings.Iterations[partitionIndex].Seed >> 11)
}

func (s *seedProbeIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return []float64{s.seed}
}

// TestEmbeddedReseedGivesEveryStepAndPartitionItsOwnStream checks that, with
// reseeding on, no two (outer step, inner partition) pairs share a random
// stream. An XOR-linear seed derivation fails this: step s's partition i and
// step i's partition s would both be base ^ (s+1)K ^ (i+1)K.
func TestEmbeddedReseedGivesEveryStepAndPartitionItsOwnStream(t *testing.T) {
	const partitions = 4
	const steps = 6
	inner := simulator.NewConfigGenerator()
	inner.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.NilOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 1},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	for i := 0; i < partitions; i++ {
		inner.SetPartition(&simulator.PartitionConfig{
			Name:              fmt.Sprintf("probe_%d", i),
			Iteration:         &seedProbeIteration{},
			Params:            simulator.NewParams(map[string][]float64{}),
			InitStateValues:   []float64{0},
			StateHistoryDepth: 1,
			Seed:              uint64(i + 1),
		})
	}
	embedded := NewEmbeddedSimulationRunIteration(inner.GenerateConfigs())
	embedded.SetReseedBase(99)

	outer := simulator.NewConfigGenerator()
	outer.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.NilOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 1},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	outer.SetPartition(&simulator.PartitionConfig{
		Name:              "embedded",
		Iteration:         embedded,
		Params:            simulator.NewParams(map[string][]float64{"burn_in_steps": {0}}),
		InitStateValues:   make([]float64, partitions),
		StateHistoryDepth: 1,
		Seed:              0,
	})
	outerSettings, outerImpl := outer.GenerateConfigs()
	coordinator := simulator.NewPartitionCoordinator(outerSettings, outerImpl)
	params := &outerSettings.Iterations[0].Params

	seen := make(map[float64]string)
	for step := 1; step <= steps; step++ {
		coordinator.Shared.TimestepsHistory.CurrentStepNumber = step
		row := embedded.Iterate(params, 0, coordinator.Shared.StateHistories,
			coordinator.Shared.TimestepsHistory)
		for partition, seed := range row {
			key := fmt.Sprintf("step %d partition %d", step, partition)
			if other, ok := seen[seed]; ok {
				t.Errorf("%s shares a random stream with %s", key, other)
			}
			seen[seed] = key
		}
	}
}
