package general

import (
	"go/parser"
	"strings"
	"testing"

	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distuv"

	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// The evaluator's surface: every operator, every function, and every way of getting it wrong.
// These are contract tests rather than coverage filler — an expression is a model, so an
// operator that quietly disagrees with Go is a model that quietly disagrees with its twin.

// evalScalar evaluates a one-output expression over scalar params and returns the result.
func evalScalar(t *testing.T, expr string, params map[string][]float64) float64 {
	t.Helper()
	return evalOnce(t, &ExpressionIteration{
		Fields:  []ExpressionField{{Name: "self"}},
		Outputs: []string{expr},
	}, []float64{0}, params)[0]
}

func TestExpressionOperators(t *testing.T) {
	// a and b are chosen so that no two operators agree on them, which is what makes a
	// mis-wired operator fail rather than coincide.
	params := map[string][]float64{"a": {7}, "b": {3}}
	for _, c := range []struct {
		expr string
		want float64
	}{
		{"a + b", 10},
		{"a - b", 4},
		{"a * b", 21},
		{"a / b", 7.0 / 3.0},
		{"a % b", 1},
		{"-a", -7},
		{"+a", 7},
		{"!(a - 7)", 1}, // not of zero is one
		{"!a", 0},
		{"a < b", 0},
		{"b < a", 1},
		{"a > b", 1},
		{"a <= b", 0},
		{"b <= b", 1},
		{"a >= b", 1},
		{"b >= a", 0},
		{"a == b", 0},
		{"b == b", 1},
		{"a != b", 1},
		{"b != b", 0},
		// Go's precedence, because the expressions are parsed by Go.
		{"2 + 3 * 4", 14},
		{"(2 + 3) * 4", 20},
		{"-a + b", -4},
		{"a - b - b", 1}, // left-associative, not 7-(3-3)
	} {
		t.Run(c.expr, func(t *testing.T) {
			if got := evalScalar(t, c.expr, params); got != c.want {
				t.Errorf("%s: got %v, want %v", c.expr, got, c.want)
			}
		})
	}
}

func TestExpressionLogicalOperators(t *testing.T) {
	t.Run("scalar and or short-circuit", func(t *testing.T) {
		// The untaken side is never evaluated, so an undefined name on it is not an error.
		// This is what makes a guard such as n > 0 && p(n) safe.
		for _, c := range []struct {
			expr string
			want float64
		}{
			{"0 && nope", 0},
			{"1 || nope", 1},
			{"1 && 1", 1},
			{"1 && 0", 0},
			{"0 || 0", 0},
			{"0 || 1", 1},
			// The right side is normalised to 0/1 rather than passed through.
			{"1 && 5", 1},
			{"0 || 5", 1},
		} {
			t.Run(c.expr, func(t *testing.T) {
				if got := evalScalar(t, c.expr, map[string][]float64{}); got != c.want {
					t.Errorf("%s: got %v, want %v", c.expr, got, c.want)
				}
			})
		}
	})

	t.Run("vector and or select elementwise", func(t *testing.T) {
		// With a vector on the left there is nothing to short-circuit to, so both sides are
		// evaluated and combined lane by lane.
		got := evalOnce(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 4}},
			Outputs: []string{"(v > 0) && (v < 3)"},
		}, []float64{-1, 1, 2, 5}, map[string][]float64{})
		for i, want := range []float64{0, 1, 1, 0} {
			if got[i] != want {
				t.Fatalf("&&: got %v, want [0 1 1 0]", got)
			}
		}
		got = evalOnce(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 4}},
			Outputs: []string{"(v < 0) || (v > 3)"},
		}, []float64{-1, 1, 2, 5}, map[string][]float64{})
		for i, want := range []float64{1, 0, 0, 1} {
			if got[i] != want {
				t.Fatalf("||: got %v, want [1 0 0 1]", got)
			}
		}
	})
}

func TestExpressionFunctions(t *testing.T) {
	params := map[string][]float64{"a": {7}, "b": {3}, "neg": {-2.5}}
	for _, c := range []struct {
		expr string
		want float64
	}{
		{"clamp(a, 0, 5)", 5},
		{"clamp(a, 8, 10)", 8},
		{"clamp(a, 0, 10)", 7},
		{"min(a, b)", 3},
		{"max(a, b)", 7},
		{"pow(b, 2)", 9},
		{"abs(neg)", 2.5},
		{"floor(neg)", -3},
		{"floor(a / b)", 2},
		{"exp(0)", 1},
		{"log(1)", 0},
		{"sqrt(9)", 3},
	} {
		t.Run(c.expr, func(t *testing.T) {
			if got := evalScalar(t, c.expr, params); got != c.want {
				t.Errorf("%s: got %v, want %v", c.expr, got, c.want)
			}
		})
	}

	t.Run("clamp with a low above its high pins to the low", func(t *testing.T) {
		// max-then-min, matching the Go the twins are compared against. Documented because it
		// is an order the caller can observe when the bounds cross.
		if got := evalScalar(t, "clamp(5, 10, 0)", map[string][]float64{}); got != 0 {
			t.Errorf("got %v, want 0 (min applied last)", got)
		}
	})

	t.Run("elementwise over vectors, not just scalars", func(t *testing.T) {
		got := evalOnce(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 3}},
			Outputs: []string{"clamp(abs(v), 0, 2)"},
		}, []float64{-5, 1, 3}, map[string][]float64{})
		for i, want := range []float64{2, 1, 2} {
			if got[i] != want {
				t.Fatalf("got %v, want [2 1 2]", got)
			}
		}
	})
}

func TestExpressionBroadcasting(t *testing.T) {
	// Length-1 broadcasts against any length, from either side, and a genuine mismatch is an
	// error rather than a silent truncation to the shorter.
	t.Run("a scalar broadcasts from either side", func(t *testing.T) {
		got := evalOnce(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 3}},
			Outputs: []string{"k - v + (v - k)"},
		}, []float64{1, 2, 3}, map[string][]float64{"k": {10}})
		for i := range got {
			if got[i] != 0 {
				t.Fatalf("scalar-vector and vector-scalar disagree: %v", got)
			}
		}
	})

	t.Run("mismatched widths are rejected, naming both", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected a panic combining widths 3 and 2")
			}
			msg := stringifyPanic(r)
			if !strings.Contains(msg, "3") || !strings.Contains(msg, "2") {
				t.Errorf("panic should name both widths, got: %q", msg)
			}
		}()
		evalOnce(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 3}},
			Outputs: []string{"v + pair"},
		}, []float64{1, 2, 3}, map[string][]float64{"pair": {1, 2}})
	})
}

func TestExpressionBindings(t *testing.T) {
	// Every model twin leans on bindings, and nothing in this package covered them.
	t.Run("are evaluated in order and are visible to later ones", func(t *testing.T) {
		e := &ExpressionIteration{
			Fields: []ExpressionField{{Name: "x"}},
			Bindings: []ExpressionBinding{
				{Name: "doubled", Expr: "x * 2"},
				{Name: "plus_one", Expr: "doubled + 1"},
				{Name: "squared", Expr: "plus_one * plus_one"},
			},
			Outputs: []string{"squared"},
		}
		// x=3 -> 6 -> 7 -> 49
		if got := evalOnce(t, e, []float64{3}, map[string][]float64{})[0]; got != 49 {
			t.Fatalf("got %v, want 49", got)
		}
	})

	t.Run("a binding may shadow a param", func(t *testing.T) {
		e := &ExpressionIteration{
			Fields:   []ExpressionField{{Name: "x"}},
			Bindings: []ExpressionBinding{{Name: "k", Expr: "99"}},
			Outputs:  []string{"k"},
		}
		if got := evalOnce(t, e, []float64{0}, map[string][]float64{"k": {1}})[0]; got != 99 {
			t.Fatalf("the binding should win over the param, got %v", got)
		}
	})

	t.Run("a forward reference is an unknown name", func(t *testing.T) {
		// Bindings are ordered, not a solved graph, so referring ahead is an error rather
		// than working by accident.
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected a panic for a forward reference")
			}
			msg := stringifyPanic(r)
			if !strings.Contains(msg, "unknown name") || !strings.Contains(msg, "later") {
				t.Errorf("panic should report the forward name as unknown, got: %q", msg)
			}
		}()
		evalOnce(t, &ExpressionIteration{
			Fields: []ExpressionField{{Name: "x"}},
			Bindings: []ExpressionBinding{
				{Name: "early", Expr: "later + 1"},
				{Name: "later", Expr: "1"},
			},
			Outputs: []string{"early"},
		}, []float64{0}, map[string][]float64{})
	})

	t.Run("bindings are eager, unlike a scalar where's branches", func(t *testing.T) {
		// Load-bearing, and the reason every twin puts a guarded draw inside the where rather
		// than in a binding the where then selects: a binding that draws, draws every step,
		// even when nothing reads it. Proven by the stream position rather than by the
		// discarded value, which is invisible by construction.
		withBinding := evalOnce(t, &ExpressionIteration{
			Fields:   []ExpressionField{{Name: "x"}},
			Bindings: []ExpressionBinding{{Name: "unread", Expr: "shared(normal(0, 1))"}},
			Outputs:  []string{"where(x > 100, unread, 0) + shared(normal(0, 1))"},
		}, []float64{1}, map[string][]float64{})

		withoutBinding := evalOnce(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "x"}},
			Outputs: []string{"where(x > 100, 0, 0) + shared(normal(0, 1))"},
		}, []float64{1}, map[string][]float64{})

		sampler := rng.New(1)
		first, second := sampler.Normal(0, 1), sampler.Normal(0, 1)
		if withoutBinding[0] != first {
			t.Fatalf("without a binding the visible draw should be the stream's first, "+
				"got %v want %v", withoutBinding[0], first)
		}
		if withBinding[0] != second {
			t.Fatalf("the unread binding did not draw: the visible draw is %v, and it should "+
				"be the stream's second (%v) because the binding consumed the first (%v)",
				withBinding[0], second, first)
		}
	})
}

func TestExpressionDrawsMatchTheSampler(t *testing.T) {
	// Every draw must be exactly pkg/rng's, in the order written: that identity is the whole
	// basis on which a declarative twin is compared to compiled Go.
	for _, c := range []struct {
		name, expr string
		want       func(s *rng.Sampler) float64
	}{
		{"normal", "iid(1, normal(2, 3))", func(s *rng.Sampler) float64 {
			return s.Normal(2, 3)
		}},
		{"uniform", "iid(1, uniform(1, 5))", func(s *rng.Sampler) float64 {
			return s.Uniform(1, 5)
		}},
		{"exponential", "iid(1, exponential(2))", func(s *rng.Sampler) float64 {
			return s.Exponential(2)
		}},
		{"poisson", "iid(1, poisson(4))", func(s *rng.Sampler) float64 {
			return s.Poisson(4)
		}},
		{"gamma", "iid(1, gamma(2, 3))", func(s *rng.Sampler) float64 {
			return s.Gamma(2, 3)
		}},
		{"beta", "iid(1, beta(2, 3))", func(s *rng.Sampler) float64 {
			return s.Beta(2, 3)
		}},
		{"binomial", "iid(1, binomial(10, 0.5))", func(s *rng.Sampler) float64 {
			return distuv.Binomial{N: 10, P: 0.5, Src: s.Rand()}.Rand()
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := evalScalar(t, c.expr, map[string][]float64{})
			// evalOnce seeds every partition with 1.
			if want := c.want(rng.New(1)); got != want {
				t.Errorf("%s: got %v, want %v — the draw is not the sampler's", c.name, got, want)
			}
		})
	}

	t.Run("a vector parameter draws once per element, in order", func(t *testing.T) {
		got := evalOnce(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 3}},
			Outputs: []string{"poisson(rates)"},
		}, []float64{0, 0, 0}, map[string][]float64{"rates": {1, 5, 20}})
		sampler := rng.New(1)
		for i, rate := range []float64{1, 5, 20} {
			if want := sampler.Poisson(rate); got[i] != want {
				t.Fatalf("element %d: got %v, want %v", i, got[i], want)
			}
		}
	})
}

func TestExpressionRejectsMalformedExpressions(t *testing.T) {
	for _, c := range []struct {
		name, expr, wantIn string
	}{
		{"unknown name", "nope + 1", "unknown name"},
		{"unknown function", "wibble(1)", "unknown function"},
		{"unsupported call target", "pkg.fn(1)", "unsupported call target"},
		{"unsupported syntax", "self.field", "unsupported syntax"},
		{"unsupported operator", "self << 1", "unsupported operator"},
		{"a string is not a number", `"hello"`, "bad numeric literal"},
		{"wrong arity", "sqrt(1, 2)", "takes 1 arguments"},
		{"concat needs two", "concat(1)", "at least 2"},
		{"index must be scalar", "pair[pair]", "index must be a scalar"},
		{"index out of range", "pair[9]", "out of range"},
		{"iid count must be scalar", "sum(iid(pair, 1))", "count must be a scalar"},
		{"iid count below one", "sum(iid(0, 1))", "at least 1"},
		{"iid lane must be scalar", "sum(iid(2, fill(3, 1)))", "scalar-valued expression"},
		{"each count must be scalar", "sum(each(pair, i, 1))", "count must be a scalar"},
		{"fill width must be scalar", "sum(fill(pair, 1))", "width must be a scalar"},
		{"fill width below one", "sum(fill(0, 1))", "at least 1"},
		{"slice bounds must be scalar", "sum(slice(pair, pair, 1))", "must be scalars"},
		{"slice width below one", "sum(slice(pair, 0, 0))", "at least 1"},
		{"lag needs a name", "lag(1 + 1, 0)", "must be an upstream alias"},
		{"lag row must be scalar", "lag(self, pair)", "row must be a scalar"},
	} {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("expected a panic for %s", c.name)
				}
				if msg := stringifyPanic(r); !strings.Contains(msg, c.wantIn) {
					t.Errorf("panic should mention %q, got: %q", c.wantIn, msg)
				}
			}()
			evalScalar(t, c.expr, map[string][]float64{"pair": {1, 2}})
		})
	}
}

func TestExpressionConfigureRejectsBadSpecs(t *testing.T) {
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{{Name: "p", Seed: 1}},
	}
	for _, c := range []struct {
		name   string
		e      *ExpressionIteration
		wantIn string
	}{
		{"negative field width", &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: -1}},
			Outputs: []string{"1"},
		}, "negative width"},
		{"unparseable binding", &ExpressionIteration{
			Fields:   []ExpressionField{{Name: "v"}},
			Bindings: []ExpressionBinding{{Name: "b", Expr: "1 +"}},
			Outputs:  []string{"1"},
		}, "parsing binding b"},
		{"unparseable output names its field", &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "the_field"}},
			Outputs: []string{"1 +"},
		}, "the_field"},
	} {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("expected a panic for %s", c.name)
				}
				if msg := stringifyPanic(r); !strings.Contains(msg, c.wantIn) {
					t.Errorf("panic should mention %q, got: %q", c.wantIn, msg)
				}
			}()
			c.e.Configure(0, settings)
		})
	}
}

func TestExpressionOutputWidthMustFitItsField(t *testing.T) {
	t.Run("a width that is neither the field's nor 1 is rejected", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected a panic for a mis-sized output")
			}
			msg := stringifyPanic(r)
			if !strings.Contains(msg, "v") || !strings.Contains(msg, "width 2") {
				t.Errorf("panic should name the field and the width, got: %q", msg)
			}
		}()
		evalOnce(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 3}},
			Outputs: []string{"fill(2, 1)"},
		}, []float64{0, 0, 0}, map[string][]float64{})
	})
}

func TestExpressionLagReadsAnUpstream(t *testing.T) {
	// The own-field path is covered elsewhere; this is the alias path, which is what
	// trywizard's match_state actually uses.
	e := &ExpressionIteration{
		Fields:    []ExpressionField{{Name: "x"}},
		Upstreams: map[string]string{"other": "source"},
		Outputs:   []string{"lag(other, 2)[1]"},
	}
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{
			{Name: "p", Seed: 1},
			{Name: "source", Seed: 2},
		},
	}
	e.Configure(0, settings)
	params := simulator.NewParams(map[string][]float64{})
	histories := []*simulator.StateHistory{
		{
			Values:            mat.NewDense(1, 1, []float64{0}),
			StateWidth:        1,
			StateHistoryDepth: 1,
		},
		{
			Values:            mat.NewDense(3, 2, []float64{1, 10, 2, 20, 3, 30}),
			StateWidth:        2,
			StateHistoryDepth: 3,
		},
	}
	got := e.Iterate(&params, 0, histories, &simulator.CumulativeTimestepsHistory{
		Values:            mat.NewVecDense(1, []float64{0}),
		NextIncrement:     1,
		CurrentStepNumber: 1,
	})
	if got[0] != 30 {
		t.Fatalf("lag(other, 2)[1]: got %v, want 30", got[0])
	}
}

func TestExpressionLagWithoutHistoriesSaysSo(t *testing.T) {
	// Iterate always supplies the resolver, so this guard is unreachable through the public
	// surface and is tested directly rather than left as an untested assertion. It exists so
	// that a future evaluation path which forgets to wire it fails by saying what is missing,
	// instead of dereferencing a nil func.
	t.Run("a context with no history resolver reports it", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected a panic when lag has no resolver")
			}
			if msg := stringifyPanic(r); !strings.Contains(msg, "lag is unavailable") {
				t.Errorf("panic should say lag is unavailable, got: %q", msg)
			}
		}()
		parsed, err := parser.ParseExpr("lag(x, 1)")
		if err != nil {
			t.Fatalf("parsing: %v", err)
		}
		ctx := &exprCtx{env: exprEnv{"x": exprValue{1}}, sampler: rng.New(1)}
		ctx.eval(parsed)
	})
}

func TestExpressionDoesNotMutateWhatItReads(t *testing.T) {
	// The environment aliases the params map and the state history rather than copying them,
	// so any function that wrote through its argument would corrupt the caller. The harness
	// checks params; this pins the state too, and says which value moved.
	e := &ExpressionIteration{
		Fields: []ExpressionField{{Name: "v", Width: 3}},
		Bindings: []ExpressionBinding{
			{Name: "scaled", Expr: "v * k"},
			{Name: "shifted", Expr: "concat(slice(v, 1, 2), 0)"},
		},
		Outputs: []string{"scaled + shifted + iid(3, normal(0, 1))"},
	}
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{{Name: "p", Seed: 1}},
	}
	e.Configure(0, settings)

	state := []float64{1, 2, 3}
	params := simulator.NewParams(map[string][]float64{"k": {10}, "spare": {4, 5}})
	histories := []*simulator.StateHistory{{
		Values:            mat.NewDense(1, 3, state),
		StateWidth:        3,
		StateHistoryDepth: 1,
	}}
	e.Iterate(&params, 0, histories, &simulator.CumulativeTimestepsHistory{
		Values:            mat.NewVecDense(1, []float64{0}),
		NextIncrement:     1,
		CurrentStepNumber: 1,
	})

	for i, want := range []float64{1, 2, 3} {
		if got := histories[0].Values.At(0, i); got != want {
			t.Errorf("state element %d was mutated: got %v, want %v", i, got, want)
		}
	}
	if got := params.Map["k"][0]; got != 10 {
		t.Errorf("param k was mutated: got %v, want 10", got)
	}
	for i, want := range []float64{4, 5} {
		if got := params.Map["spare"][i]; got != want {
			t.Errorf("param spare element %d was mutated: got %v, want %v", i, got, want)
		}
	}
}
