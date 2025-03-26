package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestValuesFunctionVectorSumIteration(t *testing.T) {
	t.Run(
		"test that the values function vector sum iteration runs",
		func(t *testing.T) {
			settings :=
				simulator.LoadSettingsFromYaml("./values_function_vector_sum_settings.yaml")
			iterations := []simulator.Iteration{
				&ConstantValuesIteration{},
				&ParamValuesIteration{},
				&ValuesFunctionVectorSumIteration{
					Function: OtherValuesFunction,
					Kernel:   &kernels.ExponentialIntegrationKernel{},
				},
				&ValuesFunctionVectorSumIteration{
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
	t.Run(
		"test that the values function vector sum iteration runs with harnesses",
		func(t *testing.T) {
			settings :=
				simulator.LoadSettingsFromYaml("./values_function_vector_sum_settings.yaml")
			iterations := []simulator.Iteration{
				&ConstantValuesIteration{},
				&ParamValuesIteration{},
				&ValuesFunctionVectorSumIteration{
					Function: OtherValuesFunction,
					Kernel:   &kernels.ExponentialIntegrationKernel{},
				},
				&ValuesFunctionVectorSumIteration{
					Function: DataValuesFunction,
					Kernel:   &kernels.ExponentialIntegrationKernel{},
				},
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
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("test harness failed: %v", err)
			}
		},
	)
}
