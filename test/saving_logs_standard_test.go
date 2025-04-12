package main

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestSavingLogsStandard(t *testing.T) {
	t.Run(
		"integration test: saving logs standard",
		func(t *testing.T) {
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
				OutputFunction:  simulator.NewJsonLogOutputFunction("./data/test.log"),
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
		},
	)
}
