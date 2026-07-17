package api

import (
	"strings"
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

func TestExpressionsResolveUpstreamsFromYaml(t *testing.T) {
	t.Run(
		"a coupled model written only as data runs and couples correctly",
		func(t *testing.T) {
			config := LoadApiRunConfigFromYaml("test_program_expression_coupled_config.yaml")
			store := simulator.NewStateTimeStorage()
			config.Main.Simulation = simulator.SimulationConfig{
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 3,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			simulator.NewPartitionCoordinator(
				config.GetConfigGenerator().GenerateConfigs(),
			).Run()

			// driver: d' = d + 2*1, from 1 -> 1, 3, 5, 7.
			driver := store.GetValues("driver")
			wantDriver := []float64{1, 3, 5, 7}
			for i := range wantDriver {
				if driver[i][0] != wantDriver[i] {
					t.Fatalf("driver: got %v, want %v", driver, wantDriver)
				}
			}
			// responder reads the driver's PREVIOUS value (a state-history read is lag-1),
			// so r' = r + 10*d_prev accumulates 1, 3, 5: 0, 10, 40, 90.
			responder := store.GetValues("responder")
			wantResponder := []float64{0, 10, 40, 90}
			for i := range wantResponder {
				if responder[i][0] != wantResponder[i] {
					t.Fatalf("responder: got %v, want %v", responder, wantResponder)
				}
			}
		},
	)
}

func TestExpressionsWorkInsideEmbeddedRuns(t *testing.T) {
	t.Run(
		"an embedded run's partition can be specified as data",
		func(t *testing.T) {
			// Wiring lives on RunConfig rather than ApiRunConfig precisely so that embedded
			// runs get it too; this pins that down.
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
			simulation := func() simulator.SimulationConfig {
				return simulator.SimulationConfig{
					OutputCondition: &simulator.NilOutputCondition{},
					OutputFunction:  &simulator.NilOutputFunction{},
					TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
						MaxNumberOfSteps: 2,
					},
					TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
					InitTimeValue:    0.0,
				}
			}
			host := newPartition("embedded_sim", &general.ConstantValuesIteration{})
			host.Params.Set("burn_in_steps", []float64{0})

			embedded := EmbeddedRunConfig{
				Name: "embedded_sim",
				Run: RunConfig{
					Partitions: []simulator.PartitionConfig{newPartition("inner", nil)},
					Expressions: []ExpressionConfig{{
						Partition: "inner",
						ExpressionIteration: general.ExpressionIteration{
							Fields:  []general.ExpressionField{{Name: "x"}},
							Outputs: []string{"x + 1"},
						},
					}},
					Simulation: simulation(),
				},
			}
			config := &ApiRunConfig{
				Main: RunConfig{
					Partitions: []simulator.PartitionConfig{host},
					Simulation: simulation(),
				},
				Embedded: []EmbeddedRunConfig{embedded},
			}

			inner := embedded.Run.GetConfigGenerator().GetPartition("inner").Iteration
			if _, ok := inner.(*general.ExpressionIteration); !ok {
				t.Errorf("embedded run's partition was not wired: got %T", inner)
			}
			// And the whole thing still builds and runs end to end.
			simulator.NewPartitionCoordinator(
				config.GetConfigGenerator().GenerateConfigs(),
			).Run()
		},
	)
}

func TestExpressionNamingAnUnknownPartitionIsRejected(t *testing.T) {
	t.Run(
		"a spec naming a partition that does not exist reports it clearly",
		func(t *testing.T) {
			config := &RunConfig{
				Partitions: []simulator.PartitionConfig{},
				Expressions: []ExpressionConfig{{
					Partition: "ghost",
					ExpressionIteration: general.ExpressionIteration{
						Fields:  []general.ExpressionField{{Name: "x"}},
						Outputs: []string{"x"},
					},
				}},
			}
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("expected a panic for an expression naming an unknown partition")
				}
				if got := stringify(r); !strings.Contains(got, "ghost") {
					t.Errorf("panic should name the offending partition, got: %q", got)
				}
			}()
			config.GetConfigGenerator()
		},
	)
}

func TestExpressionsParseInTheStringsView(t *testing.T) {
	t.Run(
		"the same YAML parses in the code-generation view and validates",
		func(t *testing.T) {
			// Both views read the same file, so an expressions field must not break the
			// templated view that drives code generation.
			config := LoadApiRunConfigStringsFromYaml("test_program_expression_config.yaml")
			if len(config.Main.Expressions) != 1 {
				t.Fatalf("got %d expression specs, want 1", len(config.Main.Expressions))
			}
			if config.Main.Expressions[0].Partition != "walk" {
				t.Errorf("spec partition: got %q, want walk",
					config.Main.Expressions[0].Partition)
			}
			// The partition carries no iteration string, and validation accepted it purely
			// because the expression spec names it.
			if config.Main.Partitions[0].Iteration != "" {
				t.Errorf("expected no iteration string, got %q",
					config.Main.Partitions[0].Iteration)
			}
		},
	)
}
