package simulator

import (
	"testing"

	"gopkg.in/yaml.v2"
)

func TestComponentSpecUnmarshal(t *testing.T) {
	t.Run("a scalar string becomes GoExpr", func(t *testing.T) {
		var spec ComponentSpec
		if err := yaml.Unmarshal([]byte(`"&simulator.EveryStepOutputCondition{}"`), &spec); err != nil {
			t.Fatal(err)
		}
		if spec.IsData() {
			t.Error("a string spec should not be data")
		}
		if spec.GoExpr != "&simulator.EveryStepOutputCondition{}" {
			t.Errorf("GoExpr = %q", spec.GoExpr)
		}
	})

	t.Run("a mapping becomes a data spec with type and fields", func(t *testing.T) {
		var spec ComponentSpec
		if err := yaml.Unmarshal([]byte("{type: constant, stepsize: 0.5}"), &spec); err != nil {
			t.Fatal(err)
		}
		if !spec.IsData() {
			t.Fatal("a mapping spec should be data")
		}
		if spec.Type != "constant" {
			t.Errorf("Type = %q", spec.Type)
		}
		if _, ok := spec.Fields["stepsize"]; !ok {
			t.Errorf("stepsize should be in Fields: %v", spec.Fields)
		}
	})

	t.Run("a mapping without a type is rejected", func(t *testing.T) {
		var spec ComponentSpec
		if err := yaml.Unmarshal([]byte("{stepsize: 0.5}"), &spec); err == nil {
			t.Error("expected an error for a data spec with no type")
		}
	})
}

func TestResolveComponents(t *testing.T) {
	t.Run("each family resolves its data-spec members", func(t *testing.T) {
		if _, err := ResolveOutputCondition(ComponentSpec{Type: "every_step"}); err != nil {
			t.Errorf("every_step: %v", err)
		}
		oc, err := ResolveOutputCondition(ComponentSpec{
			Type: "every_n_steps", Fields: map[string]interface{}{"n": 5},
		})
		if err != nil {
			t.Fatalf("every_n_steps: %v", err)
		}
		if oc.(*EveryNStepsOutputCondition).N != 5 {
			t.Errorf("N = %d, want 5", oc.(*EveryNStepsOutputCondition).N)
		}
		tc, err := ResolveTerminationCondition(ComponentSpec{
			Type: "number_of_steps", Fields: map[string]interface{}{"max_steps": 100},
		})
		if err != nil {
			t.Fatalf("number_of_steps: %v", err)
		}
		if tc.(*NumberOfStepsTerminationCondition).MaxNumberOfSteps != 100 {
			t.Error("max_steps not applied")
		}
		tf, err := ResolveTimestepFunction(ComponentSpec{
			Type: "constant", Fields: map[string]interface{}{"stepsize": 0.25},
		})
		if err != nil {
			t.Fatalf("constant: %v", err)
		}
		if tf.(*ConstantTimestepFunction).Stepsize != 0.25 {
			t.Error("stepsize not applied")
		}
		if _, err := ResolveOutputFunction(ComponentSpec{Type: "stdout"}); err != nil {
			t.Errorf("stdout: %v", err)
		}
	})

	t.Run("an integer stepsize resolves as a float", func(t *testing.T) {
		// yaml delivers an unquoted whole number as int; numeric fields accept it.
		tf, err := ResolveTimestepFunction(ComponentSpec{
			Type: "constant", Fields: map[string]interface{}{"stepsize": 1},
		})
		if err != nil {
			t.Fatal(err)
		}
		if tf.(*ConstantTimestepFunction).Stepsize != 1.0 {
			t.Error("integer stepsize should resolve to 1.0")
		}
	})

	t.Run("an unknown type is rejected", func(t *testing.T) {
		if _, err := ResolveTimestepFunction(ComponentSpec{Type: "bogus"}); err == nil {
			t.Error("expected an error for an unknown type")
		}
	})

	t.Run("an unknown field is rejected", func(t *testing.T) {
		_, err := ResolveTimestepFunction(ComponentSpec{
			Type:   "constant",
			Fields: map[string]interface{}{"stepsize": 1.0, "stpsize": 2.0},
		})
		if err == nil {
			t.Error("expected an error for an unknown field (typo)")
		}
	})

	t.Run("a missing required field is rejected", func(t *testing.T) {
		if _, err := ResolveTimestepFunction(ComponentSpec{Type: "constant"}); err == nil {
			t.Error("expected an error for a missing stepsize")
		}
	})

	t.Run("a wrong field type is rejected", func(t *testing.T) {
		_, err := ResolveTimestepFunction(ComponentSpec{
			Type: "constant", Fields: map[string]interface{}{"stepsize": "fast"},
		})
		if err == nil {
			t.Error("expected an error for a string stepsize")
		}
	})
}

func TestResolveDataComponents(t *testing.T) {
	t.Run("a fully-data simulation resolves all four components", func(t *testing.T) {
		strings := SimulationConfigStrings{
			OutputCondition:      ComponentSpec{Type: "every_step"},
			OutputFunction:       ComponentSpec{Type: "nil"},
			TerminationCondition: ComponentSpec{Type: "number_of_steps", Fields: map[string]interface{}{"max_steps": 10}},
			TimestepFunction:     ComponentSpec{Type: "constant", Fields: map[string]interface{}{"stepsize": 1.0}},
			InitTimeValue:        2.5,
		}
		if !strings.FullyData() {
			t.Fatal("should report FullyData")
		}
		config, err := strings.ResolveDataComponents()
		if err != nil {
			t.Fatal(err)
		}
		if config.OutputCondition == nil || config.OutputFunction == nil ||
			config.TerminationCondition == nil || config.TimestepFunction == nil {
			t.Error("all four components should be resolved")
		}
		if config.InitTimeValue != 2.5 {
			t.Errorf("InitTimeValue = %v, want 2.5", config.InitTimeValue)
		}
	})

	t.Run("a Go-expression component is left nil for codegen", func(t *testing.T) {
		strings := SimulationConfigStrings{
			OutputCondition:  ComponentSpec{GoExpr: "&simulator.EveryStepOutputCondition{}"},
			TimestepFunction: ComponentSpec{Type: "constant", Fields: map[string]interface{}{"stepsize": 1.0}},
		}
		if strings.FullyData() {
			t.Error("a Go-expression component means not fully data")
		}
		config, err := strings.ResolveDataComponents()
		if err != nil {
			t.Fatal(err)
		}
		if config.OutputCondition != nil {
			t.Error("the Go-expression component should be left nil")
		}
		if config.TimestepFunction == nil {
			t.Error("the data component should still be resolved")
		}
	})
}

func TestRegisterComponentAndResolveExtra(t *testing.T) {
	// A downstream-registered timestep function resolves via the fallback.
	RegisterComponent("timestep_function", "test_constant_downstream",
		func(spec ComponentSpec) (interface{}, error) {
			return &ConstantTimestepFunction{Stepsize: 7.0}, nil
		})
	resolved, err := ResolveTimestepFunction(ComponentSpec{Type: "test_constant_downstream"})
	if err != nil {
		t.Fatalf("resolving a downstream-registered component: %v", err)
	}
	if got := resolved.(*ConstantTimestepFunction).Stepsize; got != 7.0 {
		t.Errorf("downstream builder not used: stepsize %v", got)
	}
}

func TestRegisterComponentDuplicatePanics(t *testing.T) {
	RegisterComponent("output_function", "dup_test",
		func(ComponentSpec) (interface{}, error) { return &NilOutputFunction{}, nil })
	defer func() {
		if recover() == nil {
			t.Error("expected a panic registering a duplicate component name")
		}
	}()
	RegisterComponent("output_function", "dup_test",
		func(ComponentSpec) (interface{}, error) { return &NilOutputFunction{}, nil })
}
