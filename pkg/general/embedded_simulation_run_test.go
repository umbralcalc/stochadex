package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestEmbeddedSimulationRunIteration(t *testing.T) {
	t.Run(
		"test that the embedded simulation run iteration runs",
		func(t *testing.T) {
			embeddedSimIterations := []simulator.Iteration{
				&FromHistoryIteration{},
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
					MaxNumberOfSteps: 300,
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
				&FromHistoryIteration{},
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
					MaxNumberOfSteps: 300,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("test harness failed: %v", err)
			}
		},
	)
}

// TestEmbeddedSimulationRunReseeding covers the opt-in re-entrant mode. The
// default is a stream, so identical inputs give different answers as the inner
// RNG advances; with a reseed base set, the run becomes a function of its inputs
// and repeats. Both halves are asserted, because the default must not change.
func TestEmbeddedSimulationRunReseeding(t *testing.T) {
	build := func() (*EmbeddedSimulationRunIteration, *simulator.Params, *simulator.PartitionCoordinator) {
		inner := simulator.NewConfigGenerator()
		inner.SetSimulation(&simulator.SimulationConfig{
			OutputCondition:      &simulator.NilOutputCondition{},
			OutputFunction:       &simulator.NilOutputFunction{},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 1},
			TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			InitTimeValue:        0,
		})
		inner.SetPartition(&simulator.PartitionConfig{
			Name:              "walk",
			Iteration:         &continuous.WienerProcessIteration{},
			Params:            simulator.NewParams(map[string][]float64{"variances": {1.0}}),
			InitStateValues:   []float64{0},
			StateHistoryDepth: 1,
			Seed:              7,
		})
		embedded := NewEmbeddedSimulationRunIteration(inner.GenerateConfigs())

		outer := simulator.NewConfigGenerator()
		outer.SetSimulation(&simulator.SimulationConfig{
			OutputCondition:      &simulator.NilOutputCondition{},
			OutputFunction:       &simulator.NilOutputFunction{},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 1},
			TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			InitTimeValue:        0,
		})
		outer.SetPartition(&simulator.PartitionConfig{
			Name:      "embedded",
			Iteration: embedded,
			Params: simulator.NewParams(map[string][]float64{
				"burn_in_steps": {0}, "walk/init_state_values": {0},
			}),
			InitStateValues:   []float64{0},
			StateHistoryDepth: 1,
			Seed:              0,
		})
		outerSettings, outerImpl := outer.GenerateConfigs()
		coordinator := simulator.NewPartitionCoordinator(outerSettings, outerImpl)
		return embedded, &outerSettings.Iterations[0].Params, coordinator
	}

	runFourTimes := func(
		embedded *EmbeddedSimulationRunIteration,
		params *simulator.Params,
		coordinator *simulator.PartitionCoordinator,
	) []float64 {
		out := make([]float64, 0, 4)
		for i := 0; i < 4; i++ {
			row := embedded.Iterate(
				params, 0, coordinator.Shared.StateHistories, coordinator.Shared.TimestepsHistory)
			out = append(out, row[0])
		}
		return out
	}

	t.Run("the default stays a stream", func(t *testing.T) {
		results := runFourTimes(build())
		for _, value := range results[1:] {
			if value != results[0] {
				return // differing values are the expected stream behaviour
			}
		}
		t.Fatalf("identical results %v; the default must keep advancing the inner "+
			"RNG, or existing configs silently change", results)
	})

	t.Run("a reseed base makes runs reproducible", func(t *testing.T) {
		embedded, params, coordinator := build()
		embedded.SetReseedBase(99)
		results := runFourTimes(embedded, params, coordinator)
		for _, value := range results[1:] {
			if value != results[0] {
				t.Fatalf("reseeded runs must repeat for identical inputs, got %v", results)
			}
		}

		// A different base must give a different answer, or the base is inert.
		other, otherParams, otherCoordinator := build()
		other.SetReseedBase(100)
		if otherResults := runFourTimes(other, otherParams, otherCoordinator); otherResults[0] == results[0] {
			t.Errorf("reseed base is not reaching the inner iterations: %v", otherResults[0])
		}
	})
}
