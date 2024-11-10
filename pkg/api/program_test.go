package api

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestRunWithParsedArgs(t *testing.T) {
	t.Run(
		"test the program runner with parsed args",
		func(t *testing.T) {
			config := &ApiRunConfigStrings{
				Main: RunConfigStrings{
					Partitions: []PartitionConfigStrings{
						{
							Name:          "first_wiener_process",
							Iteration:     "firstWienerProcess",
							ExtraPackages: []string{"github.com/umbralcalc/stochadex/pkg/continuous"},
							ExtraVars: []map[string]string{
								{"firstWienerProcess": "&continuous.WienerProcessIteration{}"},
							},
						},
						{
							Name:          "second_wiener_process",
							Iteration:     "secondWienerProcess",
							ExtraPackages: []string{"github.com/umbralcalc/stochadex/pkg/continuous"},
							ExtraVars: []map[string]string{
								{"secondWienerProcess": "&continuous.WienerProcessIteration{}"},
							},
						},
						{
							Name:          "embedded_sim",
							Iteration:     "constantValues",
							ExtraPackages: []string{"github.com/umbralcalc/stochadex/pkg/general"},
							ExtraVars: []map[string]string{
								{"constantValues": "&general.ConstantValuesIteration{}"},
							},
						},
					},
					Simulation: simulator.SimulationConfigStrings{
						OutputCondition:      "&simulator.NilOutputCondition{}",
						OutputFunction:       "&simulator.NilOutputFunction{}",
						TerminationCondition: "&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 100}",
						TimestepFunction:     "&simulator.ConstantTimestepFunction{Stepsize: 1.0}",
						InitTimeValue:        0.0,
					},
				},
			}
			RunWithParsedArgs(
				ParsedArgs{
					ConfigStrings: config,
					ConfigFile:    "test_program_config.yaml",
					SocketFile:    "",
				},
			)
		},
	)
	t.Run(
		"test the program runner with parsed args and embedded simulation",
		func(t *testing.T) {
			config := &ApiRunConfigStrings{
				Main: RunConfigStrings{
					Partitions: []PartitionConfigStrings{
						{
							Name:          "first_wiener_process",
							Iteration:     "firstWienerProcess",
							ExtraPackages: []string{"github.com/umbralcalc/stochadex/pkg/continuous"},
							ExtraVars: []map[string]string{
								{"firstWienerProcess": "&continuous.WienerProcessIteration{}"},
							},
						},
						{
							Name:          "second_wiener_process",
							Iteration:     "secondWienerProcess",
							ExtraPackages: []string{"github.com/umbralcalc/stochadex/pkg/continuous"},
							ExtraVars: []map[string]string{
								{"secondWienerProcess": "&continuous.WienerProcessIteration{}"},
							},
						},
						{
							Name:          "embedded_sim",
							ExtraPackages: []string{"github.com/umbralcalc/stochadex/pkg/general"},
						},
					},
					Simulation: simulator.SimulationConfigStrings{
						OutputCondition:      "&simulator.NilOutputCondition{}",
						OutputFunction:       "&simulator.NilOutputFunction{}",
						TerminationCondition: "&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 100}",
						TimestepFunction:     "&simulator.ConstantTimestepFunction{Stepsize: 1.0}",
						InitTimeValue:        0.0,
					},
				},
				Embedded: []EmbeddedRunConfigStrings{
					{
						Name: "embedded_sim",
						Run: RunConfigStrings{
							Partitions: []PartitionConfigStrings{
								{
									Name:          "first_wiener_process_embed_sim",
									Iteration:     "firstWienerProcessEmbedSim",
									ExtraPackages: []string{"github.com/umbralcalc/stochadex/pkg/continuous"},
									ExtraVars: []map[string]string{
										{"firstWienerProcessEmbedSim": "&continuous.WienerProcessIteration{}"},
									},
								},
								{
									Name:          "second_wiener_process_embed_sim",
									Iteration:     "secondWienerProcessEmbedSim",
									ExtraPackages: []string{"github.com/umbralcalc/stochadex/pkg/continuous"},
									ExtraVars: []map[string]string{
										{"secondWienerProcessEmbedSim": "&continuous.WienerProcessIteration{}"},
									},
								},
								{
									Name:          "constant_values_embed_sim",
									Iteration:     "constantValuesEmbedSim",
									ExtraPackages: []string{"github.com/umbralcalc/stochadex/pkg/general"},
									ExtraVars: []map[string]string{
										{"constantValuesEmbedSim": "&general.ConstantValuesIteration{}"},
									},
								},
							},
							Simulation: simulator.SimulationConfigStrings{
								OutputCondition:      "&simulator.NilOutputCondition{}",
								OutputFunction:       "&simulator.NilOutputFunction{}",
								TerminationCondition: "&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 100}",
								TimestepFunction:     "&simulator.ConstantTimestepFunction{Stepsize: 1.0}",
								InitTimeValue:        0.0,
							},
						},
					},
				},
			}
			RunWithParsedArgs(
				ParsedArgs{
					ConfigStrings: config,
					ConfigFile:    "test_program_embedded_config.yaml",
					SocketFile:    "",
				},
			)
		},
	)
}
