package inference

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestNegativeBinomialLinkingLogLikelihood(t *testing.T) {
	t.Run(
		"test that the Negative Binomial data linking log-likelihood runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"negative_binomial_settings.yaml",
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
					Iteration: &general.ValuesFunctionVectorMeanIteration{
						Function: general.DataValuesFunction,
						Kernel:   &kernels.ExponentialIntegrationKernel{},
					},
					ParamsFromUpstream: map[string]simulator.UpstreamConfig{
						"latest_data_values": {Upstream: 0},
					},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &general.ValuesFunctionVectorMeanIteration{
						Function: general.DataValuesVarianceFunction,
						Kernel:   &kernels.ExponentialIntegrationKernel{},
					},
					ParamsFromUpstream: map[string]simulator.UpstreamConfig{
						"latest_data_values": {Upstream: 0},
						"mean":               {Upstream: 1},
					},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &DataComparisonIteration{
						Likelihood: &NegativeBinomialLikelihoodDistribution{},
					},
					ParamsFromUpstream: map[string]simulator.UpstreamConfig{
						"latest_data_values": {Upstream: 0},
						"mean":               {Upstream: 1},
						"variance":           {Upstream: 2},
					},
				},
			)
			for index, partition := range partitions {
				partition.Iteration.Configure(index, settings)
			}
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Partitions:      partitions,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
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
