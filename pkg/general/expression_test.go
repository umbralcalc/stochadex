package general

import (
	"math"
	"strings"
	"testing"

	"gonum.org/v1/gonum/mat"

	"github.com/umbralcalc/stochadex/pkg/rng"

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

// cohortExpr ages a four-bucket cohort with an absorbing top and a guarded draw at the
// intake; readerExpr reads it through slice, a lag past the current row, width, and the same
// prefix sum written both ways. Together they put every construct that is not elementwise
// through the run harnesses.
func cohortExpr() *ExpressionIteration {
	return &ExpressionIteration{
		Fields: []ExpressionField{{Name: "bucket", Width: 4}},
		Outputs: []string{
			"each(4, age, where(age == 0, poisson(births)," +
				"where(age == 3, bucket[2] * survival + bucket[3] * survival," +
				"bucket[age - 1] * survival)))",
		},
	}
}

func readerExpr() *ExpressionIteration {
	return &ExpressionIteration{
		Fields: []ExpressionField{
			{Name: "total"}, {Name: "aged"},
			{Name: "swept", Width: 4}, {Name: "filled", Width: 4},
		},
		Upstreams: map[string]string{"coh": "cohort"},
		Bindings: []ExpressionBinding{
			// Three of the six coefficients, with the top bucket deliberately unweighted.
			{Name: "weights", Expr: "concat(slice(coefficients, 0, 3), 0)"},
			// The same exclusive prefix sum written both ways: as an each over ever-longer
			// slices, which asks for a zero-width slice at lane 0, and as a scan growing its
			// accumulator, which is the O(n) spelling. They must agree every step.
			{Name: "cumulative", Expr: "each(4, i, sum(slice(coh, 0, i)))"},
			{Name: "running",
				Expr: "slice(scan(4, i, acc, 0, concat(acc, acc[i] + coh[i])), 0, 4)"},
		},
		Outputs: []string{
			"dot(weights, coh)",
			"sum(lag(coh, 2)) + width(coefficients)",
			"clamp(sweep - cumulative, 0, coh)",
			"clamp(sweep - running, 0, coh)",
		},
	}
}

func TestExpressionConstructsRunWithHarnesses(t *testing.T) {
	// The harnesses are what catch what a unit test cannot: NaNs, a wrong state width, a
	// mutated params map, and statefulness residue, the last by running the whole simulation
	// twice and comparing. That last one is the reason this exists — each and scan bind their
	// lane index and accumulator into an environment they share with the enclosing scope, so a
	// restore that leaked would show up here and nowhere else.
	t.Run("each, scan, lag, slice, concat and width survive the harnesses", func(t *testing.T) {
		settings := simulator.LoadSettingsFromYaml("./expression_constructs_settings.yaml")
		iterations := []simulator.Iteration{cohortExpr(), readerExpr()}
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
	})
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

	t.Run("seasonality and a Gaussian CDF are expressible", func(t *testing.T) {
		// These exist because the catalogue asked for them: a seasonal driver needs sin, and
		// a threshold-exceedance probability needs the normal CDF, which is 0.5*erfc(-x/√2).
		// They must agree with the Go math package exactly, since that is what a declarative
		// model is compared against.
		e := &ExpressionIteration{
			Fields: []ExpressionField{{Name: "season"}, {Name: "p"}},
			Outputs: []string{
				"amplitude * sin(2 * pi * t / period + phase)",
				"0.5 * erfc(-(x - threshold) / (sigma * sqrt(2)))",
			},
		}
		got := evalOnce(t, e, []float64{0, 0}, map[string][]float64{
			"amplitude": {2}, "period": {12}, "phase": {0.3},
			"x": {1.4}, "threshold": {0.9}, "sigma": {0.7},
		})
		wantSeason := 2 * math.Sin(2*math.Pi*3.0/12+0.3)
		wantP := 0.5 * math.Erfc(-(1.4-0.9)/(0.7*math.Sqrt2))
		if got[0] != wantSeason {
			t.Errorf("season: got %v, want %v", got[0], wantSeason)
		}
		if got[1] != wantP {
			t.Errorf("exceedance probability: got %v, want %v", got[1], wantP)
		}
	})

	t.Run("cos and erf match the Go math package elementwise", func(t *testing.T) {
		e := &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 3}},
			Outputs: []string{"cos(v) + erf(v)"},
		}
		in := []float64{-1.25, 0.0, 2.5}
		got := evalOnce(t, e, in, map[string][]float64{})
		for i, x := range in {
			if want := math.Cos(x) + math.Erf(x); got[i] != want {
				t.Errorf("element %d: got %v, want %v", i, got[i], want)
			}
		}
	})

	t.Run("inverse trig matches the Go math package elementwise", func(t *testing.T) {
		// Inverse trig closes the solar-geometry gap recorded in
		// solar-fleet/STOCHADEX_GAPS.md: azimuth needs atan2, altitude needs asin/acos.
		e := &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 3}},
			Outputs: []string{"asin(v) + acos(v) + atan(v) + tan(v)"},
		}
		in := []float64{-0.5, 0.0, 0.5}
		got := evalOnce(t, e, in, map[string][]float64{})
		for i, x := range in {
			want := math.Asin(x) + math.Acos(x) + math.Atan(x) + math.Tan(x)
			if got[i] != want {
				t.Errorf("element %d: got %v, want %v", i, got[i], want)
			}
		}
	})

	t.Run("atan2 resolves the quadrant from both components", func(t *testing.T) {
		// A negative x is the case that separates a real atan2 from a naive
		// atan(y/x): the two agree only when x > 0. Solar azimuth needs the full
		// circle, so this is the property that matters.
		e := &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "a", Width: 2}},
			Outputs: []string{"atan2(y, x)"},
		}
		// Two (y, x) pairs: (1, -1) in quadrant II and (-1, -1) in quadrant III —
		// both with x < 0, where atan(y/x) would land wrong.
		got := evalOnce(t, e, []float64{0, 0}, map[string][]float64{
			"y": {1, -1}, "x": {-1, -1},
		})
		want := []float64{math.Atan2(1, -1), math.Atan2(-1, -1)}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("pair %d: got %v, want %v (atan2 must use both signs)", i, got[i], want[i])
			}
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

// Each of these closes a gap the catalogue proved was real: a model existed that could not be
// stated as data without it. The subtests name the model that asked.

func TestExpressionStructuredAccess(t *testing.T) {
	t.Run("slice addresses a block inside a flat vector", func(t *testing.T) {
		// trywizard packs a channel's coefficients at a stride offset inside one flat param,
		// and had to write 36 terms out longhand by scalar index to reach them.
		e := &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 3}},
			Outputs: []string{"slice(coefficients, 3, 3) * 2"},
		}
		got := evalOnce(t, e, []float64{0, 0, 0}, map[string][]float64{
			"coefficients": {1, 2, 3, 4, 5, 6, 7, 8, 9},
		})
		for i, want := range []float64{8, 10, 12} {
			if got[i] != want {
				t.Fatalf("got %v, want [8 10 12]", got)
			}
		}
	})

	t.Run("concat assembles a field from differently-computed pieces", func(t *testing.T) {
		e := &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 4}},
			Outputs: []string{"concat(99, slice(v, 0, 2) + 1, 77)"},
		}
		got := evalOnce(t, e, []float64{1, 2, 3, 4}, map[string][]float64{})
		for i, want := range []float64{99, 2, 3, 77} {
			if got[i] != want {
				t.Fatalf("got %v, want [99 2 3 77]", got)
			}
		}
	})

	t.Run("a zero-width slice is empty, which is what a prefix sum needs", func(t *testing.T) {
		// The natural prefix sum asks for nothing at lane 0, and nothing is the right block to
		// ask for there. It used to panic, so the spelling that worked needed a where for the
		// answer and a max(i, 1) as well, because the untaken branch is still bounds-checked.
		e := &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 6}},
			Outputs: []string{"each(6, i, sum(slice(q, 0, i)))"},
		}
		got := evalOnce(t, e, make([]float64, 6), map[string][]float64{
			"q": {3, 2, 1, 4, 5, 6},
		})
		for i, want := range []float64{0, 3, 5, 6, 10, 15} {
			if got[i] != want {
				t.Fatalf("got %v, want [0 3 5 6 10 15]", got)
			}
		}
	})

	t.Run("a book sweep walks price levels", func(t *testing.T) {
		// The downstream case the zero-width slice was blocking: a marketable order consuming
		// across levels is a prefix sum and a clamp.
		e := &ExpressionIteration{
			Fields: []ExpressionField{{Name: "taken", Width: 6}},
			Bindings: []ExpressionBinding{
				{Name: "cumulative", Expr: "each(6, i, sum(slice(q, 0, i)))"},
			},
			Outputs: []string{"clamp(sweep - cumulative, 0, q)"},
		}
		got := evalOnce(t, e, make([]float64, 6), map[string][]float64{
			"q": {3, 2, 1, 4, 5, 6}, "sweep": {7},
		})
		for i, want := range []float64{3, 2, 1, 1, 0, 0} {
			if got[i] != want {
				t.Fatalf("got %v, want [3 2 1 1 0 0]", got)
			}
		}
	})

	t.Run("slice and concat reject an out-of-range block", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected a panic for a slice past the end")
			} else if !strings.Contains(stringifyPanic(r), "outside a width-3") {
				t.Errorf("panic should say what it ran off the end of, got: %q", r)
			}
		}()
		evalOnce(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 3}},
			Outputs: []string{"slice(v, 2, 2)"},
		}, []float64{1, 2, 3}, map[string][]float64{})
	})
}

func TestExpressionEach(t *testing.T) {
	t.Run("an index shift ages a cohort", func(t *testing.T) {
		// business-survival's shape, and the reason it had no declarative twin: element i of
		// the next state reads element i-1 of the current one, with an absorbing top bucket
		// taking both the inflow from below and its own survivors. Elementwise cannot say it.
		e := &ExpressionIteration{
			Fields: []ExpressionField{{Name: "cohort", Width: 5}},
			Outputs: []string{
				"each(5, age, where(age == 0, births," +
					"where(age == 4, cohort[3] * survival + cohort[4] * survival," +
					"cohort[age - 1] * survival)))",
			},
		}
		got := evalOnce(t, e, []float64{100, 80, 60, 40, 20}, map[string][]float64{
			"births": {10}, "survival": {0.5},
		})
		// age 0 takes births; 1..3 take half of the bucket below; 4 is absorbing, taking half
		// of bucket 3 plus half of its own.
		for i, want := range []float64{10, 50, 40, 30, 30} {
			if got[i] != want {
				t.Fatalf("got %v, want [10 50 40 30 30]", got)
			}
		}
	})

	t.Run("a skipped lane draws nothing", func(t *testing.T) {
		// measles skips inactive areas entirely. A vector-guarded where must evaluate both
		// branches, so it draws in every lane; inside each, the guard is scalar and lazy.
		// Two lanes are active, so exactly two draws are taken and the third lane's identical
		// specification cannot have consumed one.
		mk := func(output string) *ExpressionIteration {
			return &ExpressionIteration{
				Fields:  []ExpressionField{{Name: "v"}},
				Outputs: []string{output},
			}
		}
		// Lanes 0 and 2 active: the values drawn must be the first two of the stream.
		gated := evalOnce(t, mk("sum(each(3, i, where(active[i] > 0, normal(0, 1), 0)))"),
			[]float64{0}, map[string][]float64{"active": {1, 0, 1}})
		// The same stream, drawn without any gating at all.
		ungated := evalOnce(t, mk("sum(iid(2, normal(0, 1)))"),
			[]float64{0}, map[string][]float64{})
		if gated[0] != ungated[0] {
			t.Fatalf("the inactive lane consumed randomness: gated=%v ungated=%v",
				gated[0], ungated[0])
		}
	})

	t.Run("draws interleave in lane order", func(t *testing.T) {
		// The other half of the measles blocker: an elementwise gamma takes all its draws
		// before any poisson, where a Go loop interleaves them per area. Inside each, a lane
		// takes its gamma and its poisson before the next lane starts, so the stream matches.
		interleaved := evalOnce(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 2}},
			Outputs: []string{"each(2, i, gamma(shape, rate) + uniform(0, 1))"},
		}, []float64{0, 0}, map[string][]float64{"shape": {2}, "rate": {1}})

		// Reproduce the same stream by hand, in the order a per-lane loop would take it.
		sampler := rng.New(1)
		want := make([]float64, 2)
		for i := range want {
			want[i] = sampler.Gamma(2, 1) + sampler.Uniform(0, 1)
		}
		for i := range want {
			if interleaved[i] != want[i] {
				t.Fatalf("lane %d: got %v, want %v — draws are not interleaving per lane",
					i, interleaved[i], want[i])
			}
		}
	})

	t.Run("each binds the index without leaking it", func(t *testing.T) {
		e := &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v"}},
			Outputs: []string{"sum(each(3, i, i)) + i"},
		}
		got := evalOnce(t, e, []float64{0}, map[string][]float64{"i": {100}})
		// each's i shadows the param inside the loop (0+1+2), and the param is intact after.
		if got[0] != 103 {
			t.Fatalf("got %v, want 103 (3 from the loop plus the outer i of 100)", got[0])
		}
	})

	t.Run("a lane must produce a scalar, and the index must be a name", func(t *testing.T) {
		for _, c := range []struct {
			name, expr string
		}{
			{"vector-valued lane", "each(2, i, fill(3, i))"},
			{"index is not a name", "each(2, 7, 1)"},
			{"count below one", "each(0, i, 1)"},
		} {
			t.Run(c.name, func(t *testing.T) {
				defer func() {
					if recover() == nil {
						t.Fatalf("expected a panic for %s", c.name)
					}
				}()
				evalOnce(t, &ExpressionIteration{
					Fields:  []ExpressionField{{Name: "v"}},
					Outputs: []string{c.expr},
				}, []float64{0}, map[string][]float64{})
			})
		}
	})
}

func TestExpressionScan(t *testing.T) {
	// each's lanes are independent and each must produce a scalar, so nothing a lane computes
	// reaches the next one. scan threads an accumulator of any width through them, and the
	// width is the point: the case that asked for this carries which slots are taken, not a
	// running total.

	t.Run("a lane sees what earlier lanes took", func(t *testing.T) {
		// The blocking case: k simultaneous arrivals claim the first k free slots, so each
		// arrival must see the slots the ones before it just took. It is not a prefix sum —
		// what accumulates is the occupancy vector — and it has no each spelling at all.
		//
		// The first free slot is the length of the leading run of occupied ones, which is how
		// many prefixes of the accumulator are entirely full.
		firstFree := "sum(each(6, p, where(sum(slice(acc, 0, p + 1)) == p + 1, 1, 0)))"
		e := &ExpressionIteration{
			Fields: []ExpressionField{{Name: "occupancy", Width: 6}},
			Outputs: []string{
				"scan(3, a, acc, occupied, each(6, j, where(j == " + firstFree +
					", 1, acc[j])))",
			},
		}
		got := evalOnce(t, e, make([]float64, 6), map[string][]float64{
			"occupied": {0, 1, 0, 0, 1, 0},
		})
		// Slots 0, 2 and 3 are the first three free ones, and all three are now taken.
		for i, want := range []float64{1, 1, 1, 1, 1, 0} {
			if got[i] != want {
				t.Fatalf("got %v, want [1 1 1 1 1 0]", got)
			}
		}
	})

	t.Run("order identity survives the allocation", func(t *testing.T) {
		// The output this was actually wanted for: not just which slots filled, but which
		// arrival got which — queue position per order. Two accumulators are one accumulator
		// packed end to end, six slots followed by three arrival records seeded at -1.
		firstFree := "sum(each(6, p, where(sum(slice(acc, 0, p + 1)) == p + 1, 1, 0)))"
		e := &ExpressionIteration{
			Fields: []ExpressionField{{Name: "occupancy", Width: 6}, {Name: "slot", Width: 3}},
			Bindings: []ExpressionBinding{
				{Name: "allocated", Expr: "scan(3, a, acc, concat(occupied, fill(3, -1)), " +
					"concat(each(6, j, where(j == " + firstFree + ", 1, acc[j])), " +
					"each(3, k, where(k == a, " + firstFree + ", acc[6 + k]))))"},
			},
			Outputs: []string{"slice(allocated, 0, 6)", "slice(allocated, 6, 3)"},
		}
		got := evalOnce(t, e, make([]float64, 9), map[string][]float64{
			"occupied": {0, 1, 0, 0, 1, 0},
		})
		for i, want := range []float64{1, 1, 1, 1, 1, 0, 0, 2, 3} {
			if got[i] != want {
				t.Fatalf("got %v, want [1 1 1 1 1 0 | 0 2 3]", got)
			}
		}
	})

	t.Run("a growing accumulator gives prefix sums in one pass", func(t *testing.T) {
		// The same answer as each(6, i, sum(slice(q, 0, i))), but each lane extends the value
		// rather than re-summing from the start. The accumulator is seeded with the exclusive
		// prefix's leading zero, so the last lane's extra entry is sliced off.
		e := &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 6}},
			Outputs: []string{"slice(scan(6, i, acc, 0, concat(acc, acc[i] + q[i])), 0, 6)"},
		}
		got := evalOnce(t, e, make([]float64, 6), map[string][]float64{
			"q": {3, 2, 1, 4, 5, 6},
		})
		for i, want := range []float64{0, 3, 5, 6, 10, 15} {
			if got[i] != want {
				t.Fatalf("got %v, want [0 3 5 6 10 15]", got)
			}
		}
	})

	t.Run("a running maximum, threaded and as a sequence", func(t *testing.T) {
		final := evalOnce(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "m"}},
			Outputs: []string{"scan(5, i, m, v[0], max(m, v[i]))"},
		}, []float64{0}, map[string][]float64{"v": {2, 7, 3, 9, 4}})
		if final[0] != 9 {
			t.Fatalf("got %v, want 9", final[0])
		}
		sequence := evalOnce(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "m", Width: 5}},
			Outputs: []string{"slice(scan(5, i, acc, v[0], concat(acc, max(acc[i], v[i]))), 1, 5)"},
		}, make([]float64, 5), map[string][]float64{"v": {2, 7, 3, 9, 4}})
		for i, want := range []float64{2, 7, 7, 9, 9} {
			if sequence[i] != want {
				t.Fatalf("got %v, want [2 7 7 9 9]", sequence)
			}
		}
	})

	t.Run("no lanes gives the initial value", func(t *testing.T) {
		// A fold over nothing is its seed, which is the right answer rather than an edge case,
		// and it is what lets the lane count come from data.
		got := evalOnce(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v"}},
			Outputs: []string{"scan(0, i, acc, 42, acc + 1)"},
		}, []float64{0}, map[string][]float64{})
		if got[0] != 42 {
			t.Fatalf("got %v, want 42", got[0])
		}
	})

	t.Run("scan binds both names without leaking either", func(t *testing.T) {
		// Both bindings go into the environment shared with the enclosing scope, so both must
		// be put back. The seed is evaluated before either is bound, so it means the outer ones.
		e := &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v"}},
			Outputs: []string{"scan(3, i, acc, 0, acc + i) + i * 10 + acc * 100"},
		}
		got := evalOnce(t, e, []float64{0}, map[string][]float64{"i": {1}, "acc": {2}})
		// The fold is 0, then 1, then 3; the params are intact afterwards.
		if got[0] != 3+10+200 {
			t.Fatalf("got %v, want 213", got[0])
		}
	})

	t.Run("draws inside a lane are per lane, in lane order", func(t *testing.T) {
		// scan is explicit about draw width for the same reason each is: a lane is one
		// evaluation, so a scalar-parameter draw in it means one sample per lane.
		got := evalOnce(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v"}},
			Outputs: []string{"scan(3, i, acc, 0, acc + normal(0, 1))"},
		}, []float64{0}, map[string][]float64{})
		sampler := rng.New(1)
		want := 0.0
		for i := 0; i < 3; i++ {
			want += sampler.Normal(0, 1)
		}
		if got[0] != want {
			t.Fatalf("got %v, want %v — draws are not one per lane in order", got[0], want)
		}
	})
}

func TestExpressionPrefixSpellingsAgree(t *testing.T) {
	// The two ways of saying a prefix sum are the two gaps this pair of constructs closed, and
	// a model may be written either way, so they have to be the same function. Random data,
	// because a hand-picked vector is where an off-by-one hides.
	sampler := rng.New(11)
	q := make([]float64, 8)
	for i := range q {
		q[i] = sampler.Uniform(-5, 5)
	}
	viaEach := evalOnce(t, &ExpressionIteration{
		Fields:  []ExpressionField{{Name: "v", Width: 8}},
		Outputs: []string{"each(8, i, sum(slice(q, 0, i)))"},
	}, make([]float64, 8), map[string][]float64{"q": q})
	viaScan := evalOnce(t, &ExpressionIteration{
		Fields:  []ExpressionField{{Name: "v", Width: 8}},
		Outputs: []string{"slice(scan(8, i, acc, 0, concat(acc, acc[i] + q[i])), 0, 8)"},
	}, make([]float64, 8), map[string][]float64{"q": q})
	// Floating-point addition is not associative, but both spellings add in the same order, so
	// they agree exactly rather than to a tolerance.
	for i := range viaEach {
		if viaEach[i] != viaScan[i] {
			t.Fatalf("element %d: each gives %v, scan gives %v", i, viaEach[i], viaScan[i])
		}
	}
}

func TestExpressionNonFiniteIndex(t *testing.T) {
	// Found while writing business-survival's twin, whose first width probe was NaN-poisoned.
	// int(NaN) neither panics nor is portable: on arm64 it is 0, so a NaN index passes a
	// bounds check and silently reads element 0; on amd64 it is the most negative int and the
	// same expression panics. An answer that depends on the architecture is worse than a
	// failure, and these cards claim to be architecture-stable.
	for _, c := range []struct {
		name, expr string
	}{
		{"index", "v[bad]"},
		{"iid count", "sum(iid(bad, 1))"},
		{"each count", "sum(each(bad, i, 1))"},
		{"each lane index", "sum(each(2, i, v[i * bad]))"},
		{"scan count", "scan(bad, i, acc, 0, acc)"},
		{"fill width", "sum(fill(bad, 1))"},
		{"slice start", "sum(slice(v, bad, 1))"},
		{"slice width", "sum(slice(v, 0, bad))"},
	} {
		for _, bad := range []struct {
			label string
			value float64
		}{
			{"NaN", math.NaN()},
			{"+Inf", math.Inf(1)},
			{"-Inf", math.Inf(-1)},
		} {
			t.Run(c.name+" of "+bad.label, func(t *testing.T) {
				defer func() {
					if recover() == nil {
						t.Fatalf("a %s %s was accepted rather than rejected", bad.label, c.name)
					}
				}()
				evalOnce(t, &ExpressionIteration{
					Fields:  []ExpressionField{{Name: "v", Width: 3}},
					Outputs: []string{c.expr},
				}, []float64{111, 222, 333}, map[string][]float64{"bad": {bad.value}})
			})
		}
	}
}

func TestExpressionWidth(t *testing.T) {
	t.Run("reports how many elements a value has", func(t *testing.T) {
		e := &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v", Width: 3}},
			Outputs: []string{"width(rates) + width(v) * 10 + width(k) * 100"},
		}
		got := evalOnce(t, e, []float64{0, 0, 0}, map[string][]float64{
			"rates": {1, 2, 3, 4, 5}, "k": {7},
		})
		// A scalar is width 1, not width 0.
		if want := 5.0 + 30 + 100; got[0] != want {
			t.Fatalf("got %v, want %v", got[0], want)
		}
	})

	t.Run("the obvious hand-rolled alternative is the one that is wrong", func(t *testing.T) {
		// Why this exists rather than leaving people to spell it themselves: sum(0*x + 1)
		// reads like a width probe and silently is not, because 0 * NaN is NaN. Any real
		// vector carrying a NaN turns the probe into NaN, and a NaN index is undefined.
		e := &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "v"}},
			Outputs: []string{"width(x)"},
		}
		got := evalOnce(t, e, []float64{0}, map[string][]float64{
			"x": {1, math.NaN(), 3},
		})
		if got[0] != 3 {
			t.Fatalf("width must not care what the values are, got %v", got[0])
		}
	})
}

func TestExpressionLag(t *testing.T) {
	// trywizard's match_state ages yellow cards out by differencing against a partition's
	// state ten rows back. A bare name and an upstreams alias both give row 0 only.
	newHistory := func(rows [][]float64) *simulator.StateHistory {
		flat := make([]float64, 0, len(rows)*len(rows[0]))
		for _, r := range rows {
			flat = append(flat, r...)
		}
		return &simulator.StateHistory{
			Values:            mat.NewDense(len(rows), len(rows[0]), flat),
			StateWidth:        len(rows[0]),
			StateHistoryDepth: len(rows),
		}
	}

	evalWithHistory := func(t *testing.T, e *ExpressionIteration) []float64 {
		t.Helper()
		settings := &simulator.Settings{
			Iterations: []simulator.IterationSettings{{Name: "p", Seed: 1}},
		}
		e.Configure(0, settings)
		params := simulator.NewParams(map[string][]float64{})
		histories := []*simulator.StateHistory{
			newHistory([][]float64{{5, 50}, {4, 40}, {3, 30}}),
		}
		return e.Iterate(&params, 0, histories, &simulator.CumulativeTimestepsHistory{
			Values:            mat.NewVecDense(1, []float64{0}),
			NextIncrement:     1,
			CurrentStepNumber: 1,
		})
	}

	t.Run("reads a field's own state further back than the current row", func(t *testing.T) {
		got := evalWithHistory(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "a"}, {Name: "b"}},
			Outputs: []string{"lag(a, 2)", "lag(b, 1)"},
		})
		if got[0] != 3 || got[1] != 40 {
			t.Fatalf("got %v, want [3 40]", got)
		}
	})

	t.Run("lag at row 0 is the current row", func(t *testing.T) {
		got := evalWithHistory(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "a"}, {Name: "b"}},
			Outputs: []string{"lag(a, 0) - a", "lag(b, 0) - b"},
		})
		if got[0] != 0 || got[1] != 0 {
			t.Fatalf("lag(x, 0) must equal x, got %v", got)
		}
	})

	t.Run("reading past the kept history says so", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected a panic for a lag past the history")
			}
			msg := stringifyPanic(r)
			if !strings.Contains(msg, "state_history_depth") {
				t.Errorf("panic should name the fix, got: %q", msg)
			}
		}()
		evalWithHistory(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "a"}, {Name: "b"}},
			Outputs: []string{"lag(a, 3)", "b"},
		})
	})

	t.Run("an unknown name is rejected", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected a panic for lagging a name that is not a field or alias")
			}
		}()
		evalWithHistory(t, &ExpressionIteration{
			Fields:  []ExpressionField{{Name: "a"}, {Name: "b"}},
			Outputs: []string{"lag(some_param, 1)", "b"},
		})
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
