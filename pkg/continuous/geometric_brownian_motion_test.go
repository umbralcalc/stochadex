package continuous

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestGeometricBrownianMotion(t *testing.T) {
	t.Run(
		"test that the geometric Brownian motion runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"geometric_brownian_motion_settings.yaml",
			)
			partitions := make([]simulator.Partition, 0)
			for partitionIndex := range settings.StateWidths {
				iteration := &GeometricBrownianMotionIteration{}
				iteration.Configure(partitionIndex, settings)
				partitions = append(partitions, simulator.Partition{Iteration: iteration})
			}
			store := make([][][]float64, len(settings.StateWidths))
			implementations := &simulator.Implementations{
				Partitions:      partitions,
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
