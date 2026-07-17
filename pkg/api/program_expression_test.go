package api

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// An expression specification supplies a partition's iteration as data, so — unlike the
// `iteration` field, which is a Go expression needing code generation — a config built only
// from expressions runs with no Go toolchain at all. These tests cover that contract:
// validation accepts the partition, the generator wires the iteration, and a config off disk
// runs end to end.

func TestValidateAcceptsExpressionBackedPartitions(t *testing.T) {
	t.Run(
		"a partition with no iteration is valid when an expression spec matches",
		func(t *testing.T) {
			config := &ApiRunConfigStrings{
				Main: RunConfigStrings{
					Partitions: []PartitionConfigStrings{
						{Name: "declarative"}, // no iteration, but matched below
					},
					Expressions: []ExpressionConfig{{Partition: "declarative"}},
				},
			}
			if didPanic(func() { validateApiRunConfigStrings(config) }) {
				t.Error("validation panicked despite a matching expression spec")
			}
		},
	)
	t.Run(
		"a partition with neither an iteration, an embedded run nor an expression panics",
		func(t *testing.T) {
			config := &ApiRunConfigStrings{
				Main: RunConfigStrings{
					Partitions:  []PartitionConfigStrings{{Name: "orphan"}},
					Expressions: []ExpressionConfig{{Partition: "somebody_else"}},
				},
			}
			if !didPanic(func() { validateApiRunConfigStrings(config) }) {
				t.Error("expected a panic for a partition no expression spec names")
			}
		},
	)
}

func TestGetConfigGeneratorWiresExpressions(t *testing.T) {
	t.Run(
		"the named partition gets an ExpressionIteration and others are untouched",
		func(t *testing.T) {
			newPartition := func(name string, iteration simulator.Iteration) simulator.PartitionConfig {
				p := simulator.PartitionConfig{
					Name:              name,
					Iteration:         iteration,
					Params:            simulator.NewParams(make(map[string][]float64)),
					InitStateValues:   []float64{0.0},
					StateHistoryDepth: 1,
				}
				p.Init()
				return p
			}
			config := &RunConfig{
				Partitions: []simulator.PartitionConfig{
					newPartition("plain", &general.ConstantValuesIteration{}),
					// Declared with no iteration at all: the expression supplies it.
					newPartition("declarative", nil),
				},
				Expressions: []ExpressionConfig{{
					Partition: "declarative",
					ExpressionIteration: general.ExpressionIteration{
						Fields:  []general.ExpressionField{{Name: "x"}},
						Outputs: []string{"x + 1"},
					},
				}},
				Simulation: simulator.SimulationConfig{
					OutputCondition: &simulator.NilOutputCondition{},
					OutputFunction:  &simulator.NilOutputFunction{},
					TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
						MaxNumberOfSteps: 2,
					},
					TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
				},
			}

			generator := config.GetConfigGenerator()

			wired := generator.GetPartition("declarative").Iteration
			if _, ok := wired.(*general.ExpressionIteration); !ok {
				t.Errorf("declarative partition was not wired: got %T", wired)
			}
			plain := generator.GetPartition("plain").Iteration
			if _, ok := plain.(*general.ConstantValuesIteration); !ok {
				t.Errorf("plain partition was unexpectedly swapped: got %T", plain)
			}
			simulator.NewPartitionCoordinator(generator.GenerateConfigs()).Run()
		},
	)
}

func TestExpressionOnlyConfigRunsFromYaml(t *testing.T) {
	t.Run(
		"a config whose partition is specified only as data loads and runs",
		func(t *testing.T) {
			config := LoadApiRunConfigFromYaml("test_program_expression_config.yaml")

			// The inlined specification must have survived the YAML round-trip.
			if len(config.Main.Expressions) != 1 {
				t.Fatalf("got %d expression specs, want 1", len(config.Main.Expressions))
			}
			spec := config.Main.Expressions[0]
			if spec.Partition != "walk" {
				t.Errorf("spec partition: got %q, want walk", spec.Partition)
			}
			if len(spec.Fields) != 1 || spec.Fields[0].Name != "x" {
				t.Errorf("inlined fields not parsed: %+v", spec.Fields)
			}
			if len(spec.Outputs) != 1 || spec.Outputs[0] != "x + drift * dt" {
				t.Errorf("inlined outputs not parsed: %+v", spec.Outputs)
			}
			// The partition itself declares no iteration; the expression supplies it.
			if config.Main.Partitions[0].Iteration != nil {
				t.Errorf("expected no iteration on the partition, got %T",
					config.Main.Partitions[0].Iteration)
			}

			// Simulation is not YAML-loaded on RunConfig, so supply it here.
			store := simulator.NewStateTimeStorage()
			config.Main.Simulation = simulator.SimulationConfig{
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 4,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}

			generator := config.GetConfigGenerator()
			if _, ok := generator.GetPartition("walk").Iteration.(*general.ExpressionIteration); !ok {
				t.Fatalf("walk was not wired to an ExpressionIteration")
			}
			simulator.NewPartitionCoordinator(generator.GenerateConfigs()).Run()

			// x' = x + drift*dt, with drift 0.5 and dt 1: 0, 0.5, 1, 1.5, 2.
			got := store.GetValues("walk")
			want := []float64{0, 0.5, 1.0, 1.5, 2.0}
			if len(got) != len(want) {
				t.Fatalf("got %d rows, want %d: %v", len(got), len(want), got)
			}
			for i := range want {
				if got[i][0] != want[i] {
					t.Fatalf("row %d: got %v, want %v (all: %v)", i, got[i][0], want[i], got)
				}
			}
		},
	)
}
