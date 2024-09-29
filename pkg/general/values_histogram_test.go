package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestValuesHistogram(t *testing.T) {
	t.Run(
		"test that the values histogram iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./values_histogram_settings.yaml")
			iterationOne := &ConstantValuesIteration{}
			iterationOne.Configure(0, settings)
			iterationTwo := &ValuesHistogramIteration{}
			iterationTwo.Configure(1, settings)
			iterationThree := &ValuesHistogramIteration{}
			iterationThree.Configure(2, settings)
			partitions := []simulator.Partition{
				{Iteration: iterationOne},
				{Iteration: iterationTwo},
				{Iteration: iterationThree},
			}
			implementations := &simulator.Implementations{
				Partitions:      partitions,
				OutputCondition: &simulator.EveryStepOutputCondition{},
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
