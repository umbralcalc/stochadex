package simulator

import (
	"errors"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

func TestComponentSpecUnmarshal(t *testing.T) {
	t.Run("a scalar string is rejected (no Go-expression form)", func(t *testing.T) {
		var spec ComponentSpec
		if err := yaml.Unmarshal([]byte(`"&simulator.EveryStepOutputCondition{}"`), &spec); err == nil {
			t.Error("a scalar string component spec should be rejected")
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

	t.Run("a key YAML reads as a boolean is rejected, at any depth", func(t *testing.T) {
		// Re-encoded for the typed decode, the key n would arrive as false.
		for _, c := range []struct{ doc, at string }{
			{"{type: expression, upstreams: {n: source}}", "upstreams has a key"},
			{"{type: run, run: {partitions: [{params: {yes: [1]}}]}}",
				"run.partitions[0].params has a key"},
		} {
			var spec ComponentSpec
			err := yaml.Unmarshal([]byte(c.doc), &spec)
			if err == nil {
				t.Fatalf("%s: expected an error for a boolean key", c.doc)
			}
			for _, fragment := range []string{c.at, "quote the key"} {
				if !strings.Contains(err.Error(), fragment) {
					t.Errorf("%s: error %q does not mention %q", c.doc, err, fragment)
				}
			}
		}
	})

	t.Run("positive control: quoted keys and boolean values are kept", func(t *testing.T) {
		var spec ComponentSpec
		doc := `{type: expression, upstreams: {"n": source}, list: [{"y": 1}], flag: yes}`
		if err := yaml.Unmarshal([]byte(doc), &spec); err != nil {
			t.Fatal(err)
		}
		upstreams := spec.Fields["upstreams"].(map[interface{}]interface{})
		if upstreams["n"] != "source" {
			t.Errorf("upstreams = %v, want the key n kept as written", upstreams)
		}
		if spec.Fields["flag"] != true {
			t.Errorf("flag = %v, want a boolean value left alone", spec.Fields["flag"])
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

	t.Run("an omitted component is left nil and execution_strategy resolves", func(t *testing.T) {
		strings := SimulationConfigStrings{
			TimestepFunction:  ComponentSpec{Type: "constant", Fields: map[string]interface{}{"stepsize": 1.0}},
			ExecutionStrategy: ComponentSpec{Type: "inline"},
		}
		config, err := strings.ResolveDataComponents()
		if err != nil {
			t.Fatal(err)
		}
		if config.OutputCondition != nil {
			t.Error("an omitted component should be left nil")
		}
		if config.TimestepFunction == nil {
			t.Error("the data component should still be resolved")
		}
		if _, ok := config.ExecutionStrategy.(*InlineExecution); !ok {
			t.Errorf("execution_strategy = %T, want *InlineExecution", config.ExecutionStrategy)
		}
	})

	t.Run("an omitted execution_strategy resolves to nil (default policy)", func(t *testing.T) {
		strings := SimulationConfigStrings{
			TimestepFunction: ComponentSpec{Type: "constant", Fields: map[string]interface{}{"stepsize": 1.0}},
		}
		config, err := strings.ResolveDataComponents()
		if err != nil {
			t.Fatal(err)
		}
		if config.ExecutionStrategy != nil {
			t.Errorf("omitted execution_strategy should be nil, got %T", config.ExecutionStrategy)
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

func TestComponentTypes(t *testing.T) {
	resolvers := map[string]func(ComponentSpec) (interface{}, error){
		"execution_strategy":    func(s ComponentSpec) (interface{}, error) { return ResolveExecutionStrategy(s) },
		"output_condition":      func(s ComponentSpec) (interface{}, error) { return ResolveOutputCondition(s) },
		"output_function":       func(s ComponentSpec) (interface{}, error) { return ResolveOutputFunction(s) },
		"termination_condition": func(s ComponentSpec) (interface{}, error) { return ResolveTerminationCondition(s) },
		"timestep_function":     func(s ComponentSpec) (interface{}, error) { return ResolveTimestepFunction(s) },
	}
	t.Run("every family has a resolver", func(t *testing.T) {
		if len(ComponentFamilies) != len(resolvers) {
			t.Fatalf("ComponentFamilies is %v; give each family a resolver here", ComponentFamilies)
		}
		for _, family := range ComponentFamilies {
			if resolvers[family] == nil {
				t.Errorf("no resolver for family %s", family)
			}
		}
	})
	t.Run("every listed type resolves, and an unknown one does not", func(t *testing.T) {
		for family, resolve := range resolvers {
			names := ComponentTypes(family)
			if len(names) == 0 || !sort.StringsAreSorted(names) {
				t.Fatalf("%s: want a sorted, non-empty list, got %v", family, names)
			}
			for _, name := range names {
				// Fields may be missing, but the type must be known.
				if _, err := resolve(ComponentSpec{Type: name}); err != nil &&
					strings.Contains(err.Error(), "unknown data-spec type") {
					t.Errorf("%s %s is listed but does not resolve: %v", family, name, err)
				}
			}
			_, err := resolve(ComponentSpec{Type: "no_such_type"})
			if err == nil || !strings.Contains(err.Error(), "unknown data-spec type") {
				t.Errorf("%s: an unknown type gave %v", family, err)
			}
		}
	})
	t.Run("a downstream registration is listed", func(t *testing.T) {
		RegisterComponent("termination_condition", "test_listed_downstream",
			func(ComponentSpec) (interface{}, error) {
				return &NumberOfStepsTerminationCondition{MaxNumberOfSteps: 1}, nil
			})
		defer delete(extraComponentBuilders["termination_condition"], "test_listed_downstream")
		if !slices.Contains(ComponentTypes("termination_condition"), "test_listed_downstream") {
			t.Error("a registered termination condition is not listed")
		}
	})
	t.Run("a built-in with fields, and a failing downstream builder", func(t *testing.T) {
		condition, err := ResolveOutputCondition(ComponentSpec{Type: "only_given_partitions",
			Fields: map[string]interface{}{"partitions": []interface{}{"a", "b"}}})
		if err != nil {
			t.Fatal(err)
		}
		if got := condition.(*OnlyGivenPartitionsOutputCondition).Partitions; len(got) != 2 ||
			!got["a"] || !got["b"] {
			t.Errorf("got %v", got)
		}
		RegisterComponent("output_condition", "test_failing_downstream",
			func(ComponentSpec) (interface{}, error) { return nil, errors.New("bad spec") })
		defer delete(extraComponentBuilders["output_condition"], "test_failing_downstream")
		if _, err := ResolveOutputCondition(ComponentSpec{Type: "test_failing_downstream"}); err == nil ||
			err.Error() != "bad spec" {
			t.Errorf("want the builder's error, got %v", err)
		}
	})
	t.Run("an unknown family lists nothing", func(t *testing.T) {
		if names := ComponentTypes("no_such_family"); names != nil {
			t.Errorf("got %v", names)
		}
	})
}
