package api

import (
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// TestRegisterIteration covers the downstream-iteration hook: a package layered
// above pkg/api (cmd/stochadex-full, which carries the cgo ONNX Runtime) can
// contribute a {type: ...} iteration spelling without the engine importing it.
func TestRegisterIteration(t *testing.T) {
	var gotSpec simulator.ComponentSpec
	RegisterIteration("test_iteration", func(
		spec simulator.ComponentSpec,
	) (simulator.Iteration, error) {
		gotSpec = spec
		return &general.ConstantValuesIteration{}, nil
	})

	t.Run("a registered iteration is dispatched, with its whole spec", func(t *testing.T) {
		iteration, err := ResolveIteration(simulator.ComponentSpec{
			Type:   "test_iteration",
			Fields: map[string]interface{}{"model_path": "m.onnx", "depth": 3},
		})
		if err != nil {
			t.Fatalf("ResolveIteration: %v", err)
		}
		if iteration == nil {
			t.Fatal("registered builder returned a nil iteration")
		}
		// The builder must receive Type and Fields verbatim — that is how a
		// downstream iteration reads its own options (a model path, input wiring).
		if gotSpec.Type != "test_iteration" ||
			gotSpec.Fields["model_path"] != "m.onnx" || gotSpec.Fields["depth"] != 3 {
			t.Errorf("spec not passed through: %+v", gotSpec)
		}
	})

	t.Run("an unknown iteration reports it may be in the distributed build", func(t *testing.T) {
		_, err := ResolveIteration(simulator.ComponentSpec{Type: "no_such_iteration"})
		if err == nil {
			t.Fatal("expected an error for an unknown iteration")
		}
		if !strings.Contains(err.Error(), "no_such_iteration") {
			t.Errorf("error should name the unknown type, got: %v", err)
		}
	})

	t.Run("shadowing a core registry name panics", func(t *testing.T) {
		// A downstream module must not be able to silently redefine a core spelling.
		defer func() {
			if recover() == nil {
				t.Error("expected a panic when claiming a core name")
			}
		}()
		RegisterIteration("wiener_process", func(
			simulator.ComponentSpec,
		) (simulator.Iteration, error) {
			return nil, nil
		})
	})

	t.Run("registering the same name twice panics", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("expected a panic on duplicate registration")
			}
		}()
		RegisterIteration("test_iteration", func(
			simulator.ComponentSpec,
		) (simulator.Iteration, error) {
			return nil, nil
		})
	})
}
