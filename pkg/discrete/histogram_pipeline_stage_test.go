package discrete

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
						"downstream_flow_rates":    0,
						"entity_dispatch_probs":    1,
						"entity_from_partition_12": 12,
					},
					ParamsFromIndices: map[string][]int{
						"entity_from_partition_12": {0},
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
						"downstream_flow_rates":   3,
						"entity_dispatch_probs":   4,
						"entity_from_partition_2": 2,
					},
					ParamsFromIndices: map[string][]int{
						"entity_from_partition_2": {5},
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
						"downstream_flow_rates":   6,
						"entity_dispatch_probs":   7,
						"entity_from_partition_5": 5,
					},
					ParamsFromIndices: map[string][]int{
						"entity_from_partition_5": {5},
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
						"downstream_flow_rates":   9,
						"entity_dispatch_probs":   10,
						"entity_from_partition_5": 5,
						"entity_from_partition_8": 8,
					},
					ParamsFromIndices: map[string][]int{
						"entity_from_partition_5": {6},
						"entity_from_partition_8": {5},
					},
				},
				{
					Iteration: &simulator.CopyValuesIteration{},
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
