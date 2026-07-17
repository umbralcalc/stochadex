package main

import (
	"path/filepath"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestSavingLogsStandard(t *testing.T) {
	t.Run(
		"integration test: saving logs standard",
		func(t *testing.T) {
			// Written to a temporary path, not to ./data/test.log. That file is a committed
			// fixture the loading tests read, and writing to it here left the working tree
			// dirty after every run and coupled these tests by execution order — the readers
			// only saw the fixture rather than this output because Go happens to order test
			// files alphabetically.
			logFile := filepath.Join(t.TempDir(), "test.log")

			// Manually setup a simulation config
			generator := simulator.NewConfigGenerator()

			// Manually configure a simulation state partition
			generator.SetPartition(&simulator.PartitionConfig{
				Name:      "first_wiener_process",
				Iteration: &continuous.WienerProcessIteration{},
				Params: simulator.NewParams(map[string][]float64{
					"variances": {1.0, 2.0, 3.0, 4.0},
				}),
				InitStateValues:   []float64{0.0, 0.0, 0.0, 0.0},
				StateHistoryDepth: 1,
				Seed:              12345,
			})

			// Manually configure another simulation state partition
			generator.SetPartition(&simulator.PartitionConfig{
				Name:      "second_wiener_process",
				Iteration: &continuous.WienerProcessIteration{},
				Params: simulator.NewParams(map[string][]float64{
					"variances": {1.0, 2.0},
				}),
				InitStateValues:   []float64{0.0, 0.0},
				StateHistoryDepth: 1,
				Seed:              5678,
			})

			// Manually configure the extra simulation run specs
			generator.SetSimulation(&simulator.SimulationConfig{
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  simulator.NewJsonLogOutputFunction(logFile),
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 200,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{
					Stepsize: 1.0,
				},
				InitTimeValue: 0.0,
			})

			// Setup a simulation with this configuration
			coordinator := simulator.NewPartitionCoordinator(generator.GenerateConfigs())

			// Run the simulation
			coordinator.Run()

			// Read it back. Nothing else looks at this output now, so without this the test
			// would pass whether or not a single line was written.
			storage, err := analysis.NewStateTimeStorageFromJsonLogEntries(logFile)
			if err != nil {
				t.Fatalf("the log did not read back: %v", err)
			}
			if got := len(storage.GetValues("first_wiener_process")); got != 201 {
				t.Errorf("got %d logged steps, want 201 (the initial state plus 200)", got)
			}
			if got := len(storage.GetValues("second_wiener_process")[0]); got != 2 {
				t.Errorf("got a width of %d for the second process, want 2", got)
			}
		},
	)
}
