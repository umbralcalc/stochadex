package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestEmbeddedSimulationRunIteration(t *testing.T) {
	t.Run(
		"test that the embedded simulation run iteration runs",
		func(t *testing.T) {
			embeddedSimPartitions := make([]simulator.Partition, 0)
			embeddedSimPartitions = append(
				embeddedSimPartitions,
				simulator.Partition{
					Name:      "embedded_test_partition_1",
					Iteration: &ConstantValuesIteration{},
				},
			)
			embeddedSimPartitions = append(
				embeddedSimPartitions,
				simulator.Partition{
					Name:      "embedded_test_partition_2",
					Iteration: &ConstantValuesIteration{},
				},
			)
			embeddedSettings := simulator.LoadSettingsFromYaml(
				"embedded_simulation_run_settings.yaml",
			)
			partitions := make([]simulator.Partition, 0)
			partitions = append(
				partitions,
				simulator.Partition{
					Name:      "test_partition_1",
					Iteration: &ConstantValuesIteration{},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Name:      "test_partition_2",
					Iteration: &ConstantValuesIteration{},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Name: "embedded_test_simulation_run",
					Iteration: NewEmbeddedSimulationRunIteration(
						simulator.LoadSettingsFromYaml("./embedded_simulation_run_settings.yaml"),
						&simulator.Implementations{
							Partitions:      embeddedSimPartitions,
							OutputCondition: &simulator.NilOutputCondition{},
							OutputFunction:  &simulator.NilOutputFunction{},
							TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
								MaxNumberOfSteps: 100,
							},
							TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
						},
					),
					ParamsFromUpstream: map[string]simulator.UpstreamConfig{
						"embedded_test_partition_1/test_params": {Upstream: 0},
					},
				},
			)
			for index, partition := range partitions {
				partition.Iteration.Configure(index, embeddedSettings)
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
				embeddedSettings,
				implementations,
			)
			coordinator.Run()
		},
	)
}
