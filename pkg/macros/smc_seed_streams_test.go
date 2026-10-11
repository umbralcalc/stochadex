package macros

import (
	"fmt"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// seedProbeIteration reports the seed its partition was configured with, so a
// test can see which random stream each particle's partitions were handed. The
// top 53 bits are kept, which a float64 holds exactly.
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

// TestSMCParticleEvaluationGivesEveryStreamItsOwnSeed checks that no two
// (round, particle, inner partition) triples share a random stream. With an
// XOR-linear seed derivation the nesting commutes: round k's particle r equals
// round r's particle k, and within one round particle 0's partition 1 equals
// particle 1's partition 0.
func TestSMCParticleEvaluationGivesEveryStreamItsOwnSeed(t *testing.T) {
	const partitions = 3
	const particles = 4
	const rounds = 5
	// The evaluator reports one partition per particle, so one evaluator per
	// inner partition reads every stream. They share a seed, so they derive
	// the same seeds as each other.
	evaluators := make([]*SMCParticleEvaluationIteration, partitions)
	for reported := range evaluators {
		build := func() *SMCInnerSimConfig {
			config := &SMCInnerSimConfig{
				Simulation: &simulator.SimulationConfig{
					OutputCondition:      &simulator.NilOutputCondition{},
					OutputFunction:       &simulator.NilOutputFunction{},
					TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 1},
					TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
					InitTimeValue:        0,
				},
				LoglikePartition: fmt.Sprintf("probe_%d", reported),
				ParamForwarding:  map[string][]int{},
			}
			for i := 0; i < partitions; i++ {
				config.Partitions = append(config.Partitions, &simulator.PartitionConfig{
					Name:              fmt.Sprintf("probe_%d", i),
					Iteration:         &seedProbeIteration{},
					Params:            simulator.NewParams(map[string][]float64{}),
					InitStateValues:   []float64{0},
					StateHistoryDepth: 1,
					Seed:              uint64(i + 1),
				})
			}
			return config
		}
		evaluators[reported] = NewSMCParticleEvaluationIteration(build, particles, 1)
		evaluators[reported].Configure(0, &simulator.Settings{
			Iterations: []simulator.IterationSettings{{Name: "eval", Seed: 3}},
		})
	}

	params := simulator.NewParams(map[string][]float64{
		"particle_params": make([]float64, particles),
	})
	seen := make(map[float64]string)
	for round := 1; round <= rounds; round++ {
		for partition, evaluator := range evaluators {
			out := evaluator.Iterate(&params, 0, nil,
				&simulator.CumulativeTimestepsHistory{CurrentStepNumber: round})
			for particle, seed := range out {
				key := fmt.Sprintf("round %d particle %d partition %d",
					round, particle, partition)
				if other, ok := seen[seed]; ok {
					t.Errorf("%s shares a random stream with %s", key, other)
				}
				seen[seed] = key
			}
		}
	}
}
