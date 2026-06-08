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
	t.Run(
		"SetGlobalSeed assigns deterministic per-partition seeds",
		func(t *testing.T) {
			newGenerator := func() *ConfigGenerator {
				generator := NewConfigGenerator()
				generator.SetSimulation(
					&SimulationConfig{
						OutputCondition: &NilOutputCondition{},
						OutputFunction:  &NilOutputFunction{},
						TerminationCondition: &NumberOfStepsTerminationCondition{
							MaxNumberOfSteps: 1,
						},
						TimestepFunction: &ConstantTimestepFunction{Stepsize: 1.0},
						InitTimeValue:    0.0,
					},
				)
				// Several partitions so that randomized map iteration order
				// would surface as differing per-partition seed assignments.
				for i := 0; i < 8; i++ {
					generator.SetPartition(
						&PartitionConfig{
							Name:              fmt.Sprintf("partition%d", i),
							Iteration:         &doublingProcessIteration{},
							Params:            NewParams(make(map[string][]float64)),
							InitStateValues:   []float64{0.0},
							Seed:              0,
							StateHistoryDepth: 1,
						},
					)
				}
				return generator
			}
			seedsByName := func(g *ConfigGenerator) map[string]uint64 {
				out := make(map[string]uint64)
				for _, name := range g.partitionConfigOrdering.Names {
					out[name] = g.partitionConfigOrdering.ConfigByName[name].Seed
				}
				return out
			}

			first := newGenerator()
			first.SetGlobalSeed(42)
			want := seedsByName(first)

			// Calling again on the same generator must be idempotent...
			first.SetGlobalSeed(42)
			for name, seed := range seedsByName(first) {
				if seed != want[name] {
					t.Errorf(
						"repeated SetGlobalSeed changed %s: got %d, want %d",
						name, seed, want[name],
					)
				}
			}

			// ...and a fresh generator with the same global seed must derive
			// the same per-partition seeds.
			second := newGenerator()
			second.SetGlobalSeed(42)
			for name, seed := range seedsByName(second) {
				if seed != want[name] {
					t.Errorf(
						"fresh generator differs for %s: got %d, want %d",
						name, seed, want[name],
					)
				}
			}
		},
	)
}
