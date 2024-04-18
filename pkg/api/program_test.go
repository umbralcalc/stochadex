package api

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestRunWithParsedArgs(t *testing.T) {
	t.Run(
		"test the program runner with parsed args",
		func(t *testing.T) {
			implementations := &ImplementationsConfigStrings{
				Simulator: simulator.ImplementationStrings{
					Partitions: []simulator.PartitionStrings{
						{
							Iteration: "firstWienerProcess",
						},
						{
							Iteration: "actions",
						},
						{
							Iteration: "someAdditiveActor",
							ParamsFromUpstreamPartition: map[string]int{
								"action": 1,
							},
						},
					},
					OutputCondition:      "&simulator.NilOutputCondition{}",
					OutputFunction:       "&simulator.NilOutputFunction{}",
					TerminationCondition: "&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 100}",
					TimestepFunction:     "&simulator.ConstantTimestepFunction{Stepsize: 1.0}",
				},
				ExtraVarsByPackage: []map[string][]map[string]string{
					{
						"github.com/umbralcalc/stochadex/pkg/phenomena": {
							{"firstWienerProcess": "&phenomena.WienerProcessIteration{}"},
							{"secondWienerProcess": "&phenomena.WienerProcessIteration{}"},
							{"actions": "&phenomena.WienerProcessIteration{}"},
						},
					},
					{
						"github.com/umbralcalc/stochadex/pkg/actors": {
							{
								"actor":             "&actors.AdditiveActor{}",
								"someAdditiveActor": "&actors.ActorIteration{Iteration: secondWienerProcess, Actor: actor}",
							},
						},
					},
				},
			}
			RunWithParsedArgs("program_config.yaml", implementations, &DashboardConfig{})
		},
	)
}
