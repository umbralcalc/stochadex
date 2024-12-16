package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestEmbeddedSimulationRunIteration(t *testing.T) {
	t.Run(
		"test that the embedded simulation run iteration runs",
		func(t *testing.T) {
			embeddedSimIterations := []simulator.Iteration{
				&ConstantValuesIteration{},
				&ConstantValuesIteration{},
			}
			embeddedSettings := simulator.LoadSettingsFromYaml(
				"embedded_simulation_run_settings_1.yaml",
			)
			settings := simulator.LoadSettingsFromYaml(
				"embedded_simulation_run_settings_2.yaml",
			)
			iterations := []simulator.Iteration{
				&ConstantValuesIteration{},
				&ConstantValuesIteration{},
				NewEmbeddedSimulationRunIteration(
					embeddedSettings,
					&simulator.Implementations{
						Iterations:      embeddedSimIterations,
						OutputCondition: &simulator.NilOutputCondition{},
						OutputFunction:  &simulator.NilOutputFunction{},
						TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
							MaxNumberOfSteps: 100,
						},
						TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
					},
				),
			}
			for index, iteration := range iterations {
				iteration.Configure(index, settings)
			}
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.NilOutputCondition{},
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
		"test that the embedded simulation run iteration runs with harnesses",
		func(t *testing.T) {
			embeddedSimIterations := []simulator.Iteration{
				&ConstantValuesIteration{},
				&ConstantValuesIteration{},
			}
			embeddedSettings := simulator.LoadSettingsFromYaml(
				"embedded_simulation_run_settings_1.yaml",
			)
			settings := simulator.LoadSettingsFromYaml(
				"embedded_simulation_run_settings_2.yaml",
			)
			iterations := []simulator.Iteration{
				&ConstantValuesIteration{},
				&ConstantValuesIteration{},
				NewEmbeddedSimulationRunIteration(
					embeddedSettings,
					&simulator.Implementations{
						Iterations:      embeddedSimIterations,
						OutputCondition: &simulator.NilOutputCondition{},
						OutputFunction:  &simulator.NilOutputFunction{},
						TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
							MaxNumberOfSteps: 100,
						},
						TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
					},
				),
			}
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.NilOutputCondition{},
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
