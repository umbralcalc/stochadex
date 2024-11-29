package continuous

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// gammaJumpDistribution jumps the compound Poisson process with samples
// drawn from a gamma distribution - this is just for testing.
type gammaJumpDistribution struct {
	dist *distuv.Gamma
}

func (g *gammaJumpDistribution) NewJump(
	params *simulator.Params,
	stateElement int,
) float64 {
	g.dist.Alpha = params.GetIndex("gamma_alphas", stateElement)
	g.dist.Beta = params.GetIndex("gamma_betas", stateElement)
	return g.dist.Rand()
}

func TestCompoundPoissonProcess(t *testing.T) {
	t.Run(
		"test that the Compound Poisson process runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"compound_poisson_process_settings.yaml",
			)
			iterations := make([]simulator.Iteration, 0)
			for partitionIndex := range settings.Iterations {
				iteration := &CompoundPoissonProcessIteration{
					JumpDist: &gammaJumpDistribution{
						dist: &distuv.Gamma{
							Alpha: 1.0,
							Beta:  1.0,
							Src: rand.NewSource(
								settings.Iterations[partitionIndex].Seed,
							),
						},
					},
				}
				iteration.Configure(partitionIndex, settings)
				iterations = append(iterations, iteration)
			}
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			coordinator := simulator.NewPartitionCoordinator(
				settings,
				implementations,
			)
			coordinator.Run()
		},
	)
}
