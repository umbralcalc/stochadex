package keyboard

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
				Channel: &MockKeystrokeChannel{},
			}
			iteration.Configure(0, settings)
			implementations := &simulator.Implementations{
				Iterations:      []simulator.Iteration{iteration},
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
	t.Run(
		"test that the user input iteration runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./user_input_settings.yaml")
			iteration := &UserInputIteration{
				Channel: &MockKeystrokeChannel{},
			}
			implementations := &simulator.Implementations{
				Iterations:      []simulator.Iteration{iteration},
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("test harness failed: %v", err)
			}
		},
	)
}
