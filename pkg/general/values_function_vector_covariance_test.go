package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestValuesFunctionVectorCovarianceIteration(t *testing.T) {
	t.Run(
		"test that the values function vector covariance iteration runs",
		func(t *testing.T) {
			settings :=
				simulator.LoadSettingsFromYaml("./values_function_vector_covariance_settings.yaml")
			iterations := []simulator.Iteration{
				&ConstantValuesIteration{},
				&ValuesFunctionVectorMeanIteration{
					Function: DataValuesFunction,
					Kernel:   &kernels.ExponentialIntegrationKernel{},
				},
				&ValuesFunctionVectorCovarianceIteration{
					Function: DataValuesFunction,
					Kernel:   &kernels.ExponentialIntegrationKernel{},
				},
			}
			for index, iteration := range iterations {
				iteration.Configure(index, settings)
			}
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.NilOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
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
