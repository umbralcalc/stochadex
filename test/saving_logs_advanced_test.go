package main

import (
	"path/filepath"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestSavingLogsAdvanced(t *testing.T) {
	t.Run(
		"integration test: saving logs advanced",
		func(t *testing.T) {
			// Written to a temporary path, not to ./data/test.log — see the note in
			// saving_logs_standard_test.go.
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

			// Create a higher performance logging channel
			logChannel := simulator.NewJsonLogChannelOutputFunction(logFile)

			// Manually configure the extra simulation run specs
			generator.SetSimulation(&simulator.SimulationConfig{
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  logChannel,
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

			// Closed explicitly rather than deferred: this channel buffers, so the log is not
			// complete until the flush, and the read below would race it.
			logChannel.Close()

			// Read it back. Nothing else looks at this output now, so without this the test
			// would pass whether or not a single line was written.
			storage, err := analysis.NewStateTimeStorageFromJsonLogEntries(logFile)
			if err != nil {
				t.Fatalf("the log did not read back: %v", err)
			}
			if got := len(storage.GetValues("first_wiener_process")); got != 201 {
				t.Errorf("got %d logged steps, want 201 (the initial state plus 200)", got)
			}
		},
	)
}
