package inference

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/phenomena"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestNegativeBinomialLinkingLogLikelihood(t *testing.T) {
	t.Run(
		"test that the Negative Binomial data linking log-likelihood runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"negative_binomial_config.yaml",
			)
			partitions := make([]simulator.Partition, 0)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &DataGenerationIteration{
						Likelihood: &NegativeBinomialLikelihoodDistribution{},
					},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &phenomena.WeightedWindowedMeanIteration{
						Kernel: &kernels.ExponentialIntegrationKernel{},
					},
					ParamsByUpstreamPartition: map[int]string{
						0: "latest_data_values",
					},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &phenomena.WeightedWindowedCovarianceIteration{
						Kernel: &kernels.ExponentialIntegrationKernel{},
					},
					ParamsByUpstreamPartition: map[int]string{
						0: "latest_data_values",
						1: "mean",
					},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &DataComparisonIteration{
						Likelihood: &NegativeBinomialLikelihoodDistribution{},
					},
					ParamsByUpstreamPartition: map[int]string{
						0: "latest_data_values",
						1: "mean",
						2: "covariance_matrix",
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
