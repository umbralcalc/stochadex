package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// addOneTestFunction is just for testing.
func addOneTestFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	output := stateHistories[partitionIndex].Values.RawRowView(0)
	for i := range output {
		output[i] += 1
	}
	return output
}

func TestSerialIterations(t *testing.T) {
	t.Run(
		"test that serial iterations runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./serial_iterations_settings.yaml")
			iteration := &SerialIterationsIteration{
				Iterations: []simulator.Iteration{
					&ValuesFunctionIteration{Function: addOneTestFunction},
					&ValuesFunctionIteration{Function: addOneTestFunction},
				},
			}
			iteration.Configure(0, settings)
			partitions := []simulator.Partition{{Iteration: iteration}}
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
