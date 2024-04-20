package inference

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestPosteriorMeanIteration(t *testing.T) {
	t.Run(
		"test that the posterior mean iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"posterior_mean_settings.yaml",
			)
			partitions := make([]simulator.Partition, 0)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &DataGenerationIteration{
						Likelihood: &NormalLikelihoodDistribution{},
					},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &simulator.ConstantValuesIteration{},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &DataGenerationIteration{
						Likelihood: &NormalLikelihoodDistribution{},
					},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &simulator.ConstantValuesIteration{},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &PosteriorLogNormalisationIteration{},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &PosteriorMeanIteration{},
					ParamsFromUpstreamPartition: map[string]int{
						"posterior_log_normalisation": 4,
					},
				},
			)
			for index, partition := range partitions {
				partition.Iteration.Configure(index, settings)
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
