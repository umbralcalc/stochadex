package phenomena

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestPipelineStageIteration(t *testing.T) {
	t.Run(
		"test that the pipeline stage iteration runs",
		func(t *testing.T) {
			settings :=
				simulator.LoadSettingsFromYaml("./pipeline_stage_config.yaml")
			partitions := []simulator.Partition{
				{
					Iteration: &simulator.ConstantValuesIteration{},
				},
				{
					Iteration: &simulator.ConstantValuesIteration{},
				},
				{
					Iteration: &PipelineStageIteration{},
					ParamsByUpstreamPartition: map[int]string{
						0: "downstream_flow_rates",
						1: "object_dispatch_probs",
					},
				},
				{
					Iteration: &simulator.ConstantValuesIteration{},
				},
				{
					Iteration: &simulator.ConstantValuesIteration{},
				},
				{
					Iteration: &PipelineStageIteration{},
					ParamsByUpstreamPartition: map[int]string{
						3: "downstream_flow_rates",
						4: "object_dispatch_probs",
					},
				},
				{
					Iteration: &simulator.ConstantValuesIteration{},
				},
				{
					Iteration: &simulator.ConstantValuesIteration{},
				},
				{
					Iteration: &PipelineStageIteration{},
					ParamsByUpstreamPartition: map[int]string{
						6: "downstream_flow_rates",
						7: "object_dispatch_probs",
					},
				},
				{
					Iteration: &simulator.ConstantValuesIteration{},
				},
				{
					Iteration: &simulator.ConstantValuesIteration{},
				},
				{
					Iteration: &PipelineStageIteration{},
					ParamsByUpstreamPartition: map[int]string{
						9:  "downstream_flow_rates",
						10: "object_dispatch_probs",
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
