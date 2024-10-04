package api

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestRunWithParsedArgs(t *testing.T) {
	t.Run(
		"test the program runner with parsed args",
		func(t *testing.T) {
			config := &StochadexConfigImplementationsStrings{
				Simulation: SimulationConfigImplementationStrings{
					Implementations: simulator.ImplementationStrings{
						Partitions: []simulator.PartitionStrings{
							{
								Iteration: "firstWienerProcess",
							},
							{
								Iteration: "secondWienerProcess",
							},
							{
								Iteration: "constantValues",
							},
						},
						OutputCondition:      "&simulator.NilOutputCondition{}",
						OutputFunction:       "&simulator.NilOutputFunction{}",
						TerminationCondition: "&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 100}",
						TimestepFunction:     "&simulator.ConstantTimestepFunction{Stepsize: 1.0}",
					},
				},
				ExtraVarsByPackage: []map[string][]map[string]string{
					{
						"github.com/umbralcalc/stochadex/pkg/general": {
							{"constantValues": "&general.ConstantValuesIteration{}"},
						},
					},
					{
						"github.com/umbralcalc/stochadex/pkg/continuous": {
							{"firstWienerProcess": "&continuous.WienerProcessIteration{}"},
							{"secondWienerProcess": "&continuous.WienerProcessIteration{}"},
						},
					},
				},
			}
			RunWithParsedArgs(
				"program_config.yaml",
				config,
				&DashboardConfig{},
			)
		},
	)
	t.Run(
		"test the program runner with parsed args and embedded simulation",
		func(t *testing.T) {
			config := &StochadexConfigImplementationsStrings{
				Simulation: SimulationConfigImplementationStrings{
					Implementations: simulator.ImplementationStrings{
						Partitions: []simulator.PartitionStrings{
							{
								Iteration: "firstWienerProcess",
							},
							{
								Iteration: "secondWienerProcess",
							},
							{
								Iteration: "embeddedSim",
							},
						},
						OutputCondition:      "&simulator.NilOutputCondition{}",
						OutputFunction:       "&simulator.NilOutputFunction{}",
						TerminationCondition: "&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 100}",
						TimestepFunction:     "&simulator.ConstantTimestepFunction{Stepsize: 1.0}",
					},
				},
				EmbeddedSimulations: []map[string]SimulationConfigImplementationStrings{
					{"embeddedSim": {
						Implementations: simulator.ImplementationStrings{
							Partitions: []simulator.PartitionStrings{
								{
									Iteration: "firstWienerProcessEmbedSim",
								},
								{
									Iteration: "secondWienerProcessEmbedSim",
								},
								{
									Iteration: "constantValuesEmbedSim",
								},
							},
							OutputCondition:      "&simulator.NilOutputCondition{}",
							OutputFunction:       "&simulator.NilOutputFunction{}",
							TerminationCondition: "&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 100}",
							TimestepFunction:     "&simulator.ConstantTimestepFunction{Stepsize: 1.0}",
						},
					}},
				},
				ExtraVarsByPackage: []map[string][]map[string]string{
					{
						"github.com/umbralcalc/stochadex/pkg/general": {
							{"constantValuesEmbedSim": "&general.ConstantValuesIteration{}"},
						},
					},
					{
						"github.com/umbralcalc/stochadex/pkg/continuous": {
							{"firstWienerProcess": "&continuous.WienerProcessIteration{}"},
							{"secondWienerProcess": "&continuous.WienerProcessIteration{}"},
							{"firstWienerProcessEmbedSim": "&continuous.WienerProcessIteration{}"},
							{"secondWienerProcessEmbedSim": "&continuous.WienerProcessIteration{}"},
						},
					},
				},
			}
			RunWithParsedArgs(
				"program_config.yaml",
				config,
				&DashboardConfig{},
			)
		},
	)
}
