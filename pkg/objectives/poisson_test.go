package objectives

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/phenomena"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestPoissonLogLikelihood(t *testing.T) {
	t.Run(
		"test that the Poisson data linking log-likelihood runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"poisson_config.yaml",
			)
			iterations := make([][]simulator.Iteration, 0)
			iterations = append(
				iterations,
				[]simulator.Iteration{
					&DataGenerationIteration{
						DataLinking: &PoissonDataLinkingLogLikelihood{},
					},
					&phenomena.WeightedWindowedMeanIteration{
						Kernel: &kernels.ExponentialIntegrationKernel{},
					},
				},
			)
			iterations = append(
				iterations,
				[]simulator.Iteration{
					&ObjectiveIteration{
						DataLinking: &PoissonDataLinkingLogLikelihood{},
					},
				},
			)
			index := 0
			for _, serialIterations := range iterations {
				for _, iteration := range serialIterations {
					iteration.Configure(index, settings)
					index += 1
				}
			}
			store := make([][][]float64, len(settings.StateWidths))
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.VariableStoreOutputFunction{Store: store},
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