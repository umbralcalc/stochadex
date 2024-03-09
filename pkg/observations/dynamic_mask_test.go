package observations

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/phenomena"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestDynamicMaskStateObservationIteration(t *testing.T) {
	t.Run(
		"test that the dynamic mask state observation works",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"dynamic_mask_config.yaml",
			)
			iterations := make([][]simulator.Iteration, 0)
			iterations = append(
				iterations,
				[]simulator.Iteration{
					&phenomena.WienerProcessIteration{},
					&simulator.ConstantValuesIteration{},
					&DynamicMaskStateObservationIteration{},
				},
			)
			index := 0
			for _, serialIterations := range iterations {
				for _, iteration := range serialIterations {
					iteration.Configure(index, settings)
					index += 1
				}
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
