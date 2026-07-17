package general

import (
	"math"
	"strings"
	"testing"

	"gonum.org/v1/gonum/mat"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// stringifyPanic renders a recovered panic value for substring assertions.
func stringifyPanic(v any) string {
	switch value := v.(type) {
	case error:
		return value.Error()
	case string:
		return value
	}
	return ""
}

// driverExpr advances a two-scalar partition; responderExpr reads it, exercising a vector
// field, scalar broadcasting, upstream indexing and a reduction.
func driverExpr() *ExpressionIteration {
	return &ExpressionIteration{
		Fields:  []ExpressionField{{Name: "a"}, {Name: "b"}},
		Outputs: []string{"a + growth", "b * (1 + growth)"},
	}
}

func responderExpr() *ExpressionIteration {
	return &ExpressionIteration{
		Fields:    []ExpressionField{{Name: "v", Width: 3}, {Name: "total"}},
		Upstreams: map[string]string{"drv": "driver"},
		Outputs:   []string{"v + scale * drv[0]", "dot(v, weights)"},
	}
}

func TestExpressionIteration(t *testing.T) {
	t.Run(
		"test that the expression iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./expression_settings.yaml")
			iterations := []simulator.Iteration{driverExpr(), responderExpr()}
			for i, iteration := range iterations {
				iteration.Configure(i, settings)
			}
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			simulator.NewPartitionCoordinator(settings, implementations).Run()
		},
	)
	t.Run(
		"test that the expression iteration runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./expression_settings.yaml")
			iterations := []simulator.Iteration{driverExpr(), responderExpr()}
			for i, iteration := range iterations {
				iteration.Configure(i, settings)
			}
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("test harness failed: %v", err)
			}
		},
	)
}

// evalOnce configures a one-partition expression iteration over the given state and params
// and returns a single Iterate result, for testing evaluation semantics directly.
func evalOnce(
	t *testing.T,
	e *ExpressionIteration,
	state []float64,
	params map[string][]float64,
) []float64 {
	t.Helper()
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{{Name: "p", Seed: 1}},
	}
	e.Configure(0, settings)
	p := simulator.NewParams(params)
	histories := []*simulator.StateHistory{{
		Values:            mat.NewDense(1, len(state), state),
		StateWidth:        len(state),
		StateHistoryDepth: 1,
	}}
	ts := &simulator.CumulativeTimestepsHistory{
		Values:            mat.NewVecDense(1, []float64{3.0}),
		NextIncrement:     0.5,
		CurrentStepNumber: 7,
	}
	return e.Iterate(&p, 0, histories, ts)
}

func TestExpressionSemantics(t *testing.T) {
	t.Run("broadcasts scalars across a vector field and reduces", func(t *testing.T) {
		e := &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 3}, {Name: "s"}},
			Outputs: []string{"v * k + 1", "sum(v) + dot(v, w)"},
		}
		got := evalOnce(t, e, []float64{1, 2, 3, 0}, map[string][]float64{
			"k": {10}, "w": {1, 2, 3},
		})
		// v*10+1 = [11,21,31]; sum(v)=6; dot(v,w)=1+4+9=14 -> 20
		want := []float64{11, 21, 31, 20}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %v, want %v", got, want)
			}
		}
	})

	t.Run("exposes dt, t and step", func(t *testing.T) {
		e := &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "x"}},
			Outputs: []string{"dt * 100 + t * 10 + step"},
		}
		got := evalOnce(t, e, []float64{0}, map[string][]float64{})
		if want := 0.5*100 + 3.0*10 + 7.0; got[0] != want {
			t.Fatalf("got %v, want %v", got[0], want)
		}
	})

	t.Run("scalar where does not evaluate the untaken branch", func(t *testing.T) {
		// If the guard were eager, `undefined_name` would panic. Laziness is what makes
		// guards like where(n > 0, binomial(n, p), 0) safe.
		e := &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "x"}},
			Outputs: []string{"where(x > 100, undefined_name, 42)"},
		}
		got := evalOnce(t, e, []float64{1}, map[string][]float64{})
		if got[0] != 42 {
			t.Fatalf("got %v, want 42", got[0])
		}
	})

	t.Run("vector where selects elementwise", func(t *testing.T) {
		e := &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 3}},
			Outputs: []string{"where(v > 1, v * 10, 0)"},
		}
		got := evalOnce(t, e, []float64{0, 2, 3}, map[string][]float64{})
		want := []float64{0, 20, 30}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %v, want %v", got, want)
			}
		}
	})

	t.Run("draws are reproducible for a given seed", func(t *testing.T) {
		mk := func() *ExpressionIteration {
			return &ExpressionIteration{
				Fields:  []ExpressionField{{Name: "v", Width: 4}},
				Outputs: []string{"normal(fill(4, 0), 1)"},
			}
		}
		a := evalOnce(t, mk(), []float64{0, 0, 0, 0}, map[string][]float64{})
		b := evalOnce(t, mk(), []float64{0, 0, 0, 0}, map[string][]float64{})
		for i := range a {
			if a[i] != b[i] {
				t.Fatalf("same seed gave different streams: %v vs %v", a, b)
			}
		}
	})

	t.Run("an unannotated scalar-parameter draw is rejected", func(t *testing.T) {
		// The whole point: silently adding the same shock to every element of a wide field
		// is a modelling bug, and both readings are things people mean, so the ambiguity is
		// refused rather than guessed at.
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected a panic for an ambiguous scalar-parameter draw")
			}
			msg := stringifyPanic(r)
			if !strings.Contains(msg, "iid(") || !strings.Contains(msg, "shared(") {
				t.Errorf("panic should name both fixes, got: %q", msg)
			}
		}()
		evalOnce(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 4}},
			Outputs: []string{"v + normal(0, 1)"},
		}, []float64{0, 0, 0, 0}, map[string][]float64{})
	})

	t.Run("iid gives independent samples, shared gives one", func(t *testing.T) {
		independent := evalOnce(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 4}},
			Outputs: []string{"iid(4, normal(0, 1))"},
		}, []float64{0, 0, 0, 0}, map[string][]float64{})
		if independent[0] == independent[1] && independent[1] == independent[2] {
			t.Fatalf("iid should sample independently per element, got %v", independent)
		}
		one := evalOnce(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 4}},
			Outputs: []string{"v + shared(normal(0, 1))"},
		}, []float64{0, 0, 0, 0}, map[string][]float64{})
		for i := 1; i < 4; i++ {
			if one[i] != one[0] {
				t.Fatalf("shared should reuse one sample across the field, got %v", one)
			}
		}
	})

	t.Run("a vector-parameter draw needs no annotation", func(t *testing.T) {
		got := evalOnce(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 3}},
			Outputs: []string{"poisson(rates)"},
		}, []float64{0, 0, 0}, map[string][]float64{"rates": {5, 5, 5}})
		if len(got) != 3 {
			t.Fatalf("got width %d, want 3", len(got))
		}
	})

	t.Run("compound draws compose (poisson of a gamma)", func(t *testing.T) {
		// The negative-binomial branching shape the measles model needs.
		e := &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "n"}},
			Outputs: []string{"iid(1, poisson(gamma(shape, rate)))"},
		}
		got := evalOnce(t, e, []float64{0}, map[string][]float64{
			"shape": {20}, "rate": {2},
		})
		if got[0] < 0 || math.IsNaN(got[0]) {
			t.Fatalf("expected a non-negative count, got %v", got[0])
		}
	})

	t.Run("a scalar output broadcasts across its field", func(t *testing.T) {
		e := &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 3}},
			Outputs: []string{"7"},
		}
		got := evalOnce(t, e, []float64{0, 0, 0}, map[string][]float64{})
		for i := range got {
			if got[i] != 7 {
				t.Fatalf("got %v, want all 7s", got)
			}
		}
	})
}

func TestExpressionConfigErrors(t *testing.T) {
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{{Name: "p", Seed: 1}},
	}
	assertPanics := func(name string, e *ExpressionIteration) {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("expected a panic for %s", name)
				}
			}()
			e.Configure(0, settings)
		})
	}
	assertPanics("outputs not matching fields", &ExpressionIteration{
		Fields:  []ExpressionField{{Name: "a"}, {Name: "b"}},
		Outputs: []string{"a"},
	})
	assertPanics("unparseable expression", &ExpressionIteration{
		Fields:  []ExpressionField{{Name: "a"}},
		Outputs: []string{"a +"},
	})
	assertPanics("unknown upstream partition", &ExpressionIteration{
		Fields:    []ExpressionField{{Name: "a"}},
		Upstreams: map[string]string{"x": "nope"},
		Outputs:   []string{"a"},
	})
	assertPanics("unnamed field", &ExpressionIteration{
		Fields:  []ExpressionField{{Name: ""}},
		Outputs: []string{"1"},
	})
}
