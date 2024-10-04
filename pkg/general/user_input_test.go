package general

import (
	"testing"

	"github.com/eiannone/keyboard"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// MockKeystrokeChannel is the mock method for testing.
type MockKeystrokeChannel struct{}

func (m *MockKeystrokeChannel) Get(
	partitionIndex int,
	settings *simulator.Settings,
) (<-chan keyboard.KeyEvent, error) {
	mockChannel := make(chan keyboard.KeyEvent)
	return mockChannel, nil
}

func TestUserInput(t *testing.T) {
	t.Run(
		"test that the user input iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./user_input_settings.yaml")
			iteration := &UserInputIteration{
				Iteration: &ParamValuesIteration{},
				Channel:   &MockKeystrokeChannel{},
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
