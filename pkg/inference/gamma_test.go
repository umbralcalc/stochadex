package inference

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestGammaDataLinkingLogLikelihood(t *testing.T) {
	t.Run(
		"test that the Gamma data linking log-likelihood runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"gamma_settings.yaml",
			)
			partitions := make([]simulator.Partition, 0)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &DataGenerationIteration{
						Likelihood: &GammaLikelihoodDistribution{},
					},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &continuous.WeightedWindowedMeanIteration{
						Kernel: &kernels.ExponentialIntegrationKernel{},
					},
					ParamsFromUpstreamPartition: map[string]int{
						"latest_data_values": 0,
					},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &continuous.WeightedWindowedCovarianceIteration{
						Kernel: &kernels.ExponentialIntegrationKernel{},
					},
					ParamsFromUpstreamPartition: map[string]int{
						"latest_data_values": 0,
						"mean":               1,
					},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &DataComparisonIteration{
						Likelihood: &GammaLikelihoodDistribution{},
					},
					ParamsFromUpstreamPartition: map[string]int{
						"latest_data_values": 0,
						"mean":               1,
						"covariance_matrix":  2,
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
