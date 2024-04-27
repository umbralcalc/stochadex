package phenomena

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestHistogramPipelineStageIteration(t *testing.T) {
	t.Run(
		"test that the histogram pipeline stage iteration runs",
		func(t *testing.T) {
			settings :=
				simulator.LoadSettingsFromYaml("./histogram_pipeline_stage_settings.yaml")
			partitions := []simulator.Partition{
				{
					Iteration: &simulator.ConstantValuesIteration{},
				},
				{
					Iteration: &simulator.ConstantValuesIteration{},
				},
				{
					Iteration: &HistogramPipelineStageIteration{},
					ParamsFromUpstreamPartition: map[string]int{
						"downstream_flow_rates": 0,
						"object_dispatch_probs": 1,
					},
				},
				{
					Iteration: &simulator.ConstantValuesIteration{},
				},
				{
					Iteration: &simulator.ConstantValuesIteration{},
				},
				{
					Iteration: &HistogramPipelineStageIteration{},
					ParamsFromUpstreamPartition: map[string]int{
						"downstream_flow_rates": 3,
						"object_dispatch_probs": 4,
					},
				},
				{
					Iteration: &simulator.ConstantValuesIteration{},
				},
				{
					Iteration: &simulator.ConstantValuesIteration{},
				},
				{
					Iteration: &HistogramPipelineStageIteration{},
					ParamsFromUpstreamPartition: map[string]int{
						"downstream_flow_rates": 6,
						"object_dispatch_probs": 7,
					},
				},
				{
					Iteration: &simulator.ConstantValuesIteration{},
				},
				{
					Iteration: &simulator.ConstantValuesIteration{},
				},
				{
					Iteration: &HistogramPipelineStageIteration{},
					ParamsFromUpstreamPartition: map[string]int{
						"downstream_flow_rates": 9,
						"object_dispatch_probs": 10,
					},
				},
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
				TimestepFunction: simulator.NewExponentialDistributionTimestepFunction(
					2.0, settings.Seeds[0],
				),
			}
			coordinator := simulator.NewPartitionCoordinator(
				settings,
				implementations,
			)
			coordinator.Run()
		},
	)
}
