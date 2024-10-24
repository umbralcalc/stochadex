package simulator

import (
	"testing"
)

func TestConfigGenerator(t *testing.T) {
	t.Run(
		"test the config generator works as intended",
		func(t *testing.T) {
			generator := NewConfigGenerator()
			generator.SetSimulation(
				&SimulationConfig{
					OutputCondition: &NilOutputCondition{},
					OutputFunction:  &NilOutputFunction{},
					TerminationCondition: &NumberOfStepsTerminationCondition{
						MaxNumberOfSteps: 100,
					},
					TimestepFunction: &ConstantTimestepFunction{
						Stepsize: 1.0,
					},
					InitTimeValue: 0.0,
				},
			)
			generator.SetPartition(
				&PartitionConfig{
					Name:      "testPartition1",
					Iteration: &doublingProcessIteration{},
					Params:    NewParams(make(map[string][]float64)),
					ParamsFromUpstream: map[string]NamedUpstreamConfig{
						"testParams": {Upstream: "testPartition2"},
					},
					InitStateValues:   []float64{0.0, 1.0, 2.0},
					Seed:              0,
					StateHistoryDepth: 1,
				},
			)
			generator.SetPartition(
				&PartitionConfig{
					Name:              "testPartition2",
					Iteration:         &doublingProcessIteration{},
					Params:            NewParams(make(map[string][]float64)),
					InitStateValues:   []float64{0.0, 1.0},
					Seed:              0,
					StateHistoryDepth: 1,
				},
			)
			settings, _ := generator.GenerateConfigs()
			if settings.Params[0].partitionName != "testPartition1" ||
				settings.Params[1].partitionName != "testPartition2" {
				panic("ordering of partitions is wrong")
			}
			coordinator := NewPartitionCoordinator(generator.GenerateConfigs())
			coordinator.Run()
		},
	)
}
