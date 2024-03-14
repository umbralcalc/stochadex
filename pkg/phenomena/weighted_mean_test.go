package phenomena

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestWeightedMeanIteration(t *testing.T) {
	t.Run(
		"test that the weighted mean iteration runs",
		func(t *testing.T) {
			settings :=
				simulator.LoadSettingsFromYaml("./weighted_mean_config.yaml")
			partitions := []simulator.Partition{
				{Iteration: &WeightedMeanIteration{}},
				{Iteration: &WienerProcessIteration{}},
				{Iteration: &WienerProcessIteration{}},
				{Iteration: &WienerProcessIteration{}},
				{Iteration: &WienerProcessIteration{}},
			}
			for index, partition := range partitions {
				partition.Iteration.Configure(index, settings)
			}
			implementations := &simulator.Implementations{
				Partitions:      partitions,
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
