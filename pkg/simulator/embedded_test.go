package simulator

import (
	"testing"
)

// constantValuesTestIteration leaves the values set by the initial conditions
// unchanged for all time.
type constantValuesTestIteration struct {
}

func (c *constantValuesTestIteration) Configure(
	partitionIndex int,
	settings *Settings,
) {
}

func (c *constantValuesTestIteration) Iterate(
	params Params,
	partitionIndex int,
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) []float64 {
	return stateHistories[partitionIndex].Values.RawRowView(0)
}

func TestEmbeddedSimulationRunIteration(t *testing.T) {
	t.Run(
		"test that the embedded simulation run iteration runs",
		func(t *testing.T) {
			embeddedSimPartitions := make([]Partition, 0)
			embeddedSimPartitions = append(
				embeddedSimPartitions,
				Partition{
					Iteration: &constantValuesTestIteration{},
				},
			)
			embeddedSimPartitions = append(
				embeddedSimPartitions,
				Partition{
					Iteration: &constantValuesTestIteration{},
				},
			)
			embeddedSettings := LoadSettingsFromYaml(
				"embedded_settings.yaml",
			)
			partitions := make([]Partition, 0)
			partitions = append(
				partitions,
				Partition{
					Iteration: &constantValuesTestIteration{},
				},
			)
			partitions = append(
				partitions,
				Partition{
					Iteration: &constantValuesTestIteration{},
				},
			)
			partitions = append(
				partitions,
				Partition{
					Iteration: NewEmbeddedSimulationRunIteration(
						LoadSettingsFromYaml("./test_settings.yaml"),
						&Implementations{
							Partitions:      embeddedSimPartitions,
							OutputCondition: &NilOutputCondition{},
							OutputFunction:  &NilOutputFunction{},
							TerminationCondition: &NumberOfStepsTerminationCondition{
								MaxNumberOfSteps: 100,
							},
							TimestepFunction: &ConstantTimestepFunction{Stepsize: 1.0},
						},
					),
				},
			)
			for index, partition := range partitions {
				partition.Iteration.Configure(index, embeddedSettings)
			}
			implementations := &Implementations{
				Partitions:      partitions,
				OutputCondition: &NilOutputCondition{},
				OutputFunction:  &NilOutputFunction{},
				TerminationCondition: &NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				TimestepFunction: &ConstantTimestepFunction{Stepsize: 1.0},
			}
			coordinator := NewPartitionCoordinator(
				embeddedSettings,
				implementations,
			)
			coordinator.Run()
		},
	)
}
