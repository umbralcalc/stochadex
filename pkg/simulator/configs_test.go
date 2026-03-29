package simulator

import (
	"fmt"
	"strings"
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
			if settings.Iterations[0].Params.partitionName != "testPartition1" ||
				settings.Iterations[1].Params.partitionName != "testPartition2" {
				t.Error("ordering of partitions is wrong")
			}
			coordinator := NewPartitionCoordinator(generator.GenerateConfigs())
			coordinator.Run()
		},
	)
	t.Run(
		"params_from_upstream rejects out-of-range indices at generate time",
		func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("expected panic for out-of-range upstream index")
				}
				if !strings.Contains(fmt.Sprint(r), "out of range") {
					t.Fatalf("unexpected panic: %v", r)
				}
			}()
			generator := NewConfigGenerator()
			generator.SetSimulation(
				&SimulationConfig{
					OutputCondition: &NilOutputCondition{},
					OutputFunction:  &NilOutputFunction{},
					TerminationCondition: &NumberOfStepsTerminationCondition{
						MaxNumberOfSteps: 2,
					},
					TimestepFunction: &ConstantTimestepFunction{Stepsize: 1.0},
					InitTimeValue:    0.0,
				},
			)
			generator.SetPartition(
				&PartitionConfig{
					Name:              "upstream",
					Iteration:         &doublingProcessIteration{},
					Params:            NewParams(make(map[string][]float64)),
					InitStateValues:   []float64{0.0},
					Seed:              0,
					StateHistoryDepth: 1,
				},
			)
			generator.SetPartition(
				&PartitionConfig{
					Name:      "downstream",
					Iteration: &doublingProcessIteration{},
					Params:    NewParams(make(map[string][]float64)),
					ParamsFromUpstream: map[string]NamedUpstreamConfig{
						"p": {Upstream: "upstream", Indices: []int{3}},
					},
					InitStateValues:   []float64{0.0},
					Seed:              0,
					StateHistoryDepth: 1,
				},
			)
			generator.GenerateConfigs()
		},
	)
}
