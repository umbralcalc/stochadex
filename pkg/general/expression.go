package general

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"strconv"

	"gonum.org/v1/gonum/stat/distuv"

	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ExpressionField names a contiguous block of a partition's state so that expressions can
// refer to it by name instead of by index. Fields are laid out in the order given, so a
// partition with fields soc and charge has state [soc, charge], and one with a 40-wide
// infectious and a 40-wide cumulative has an 80-wide state of two 40-wide blocks.
type ExpressionField struct {
	// Name is how expressions refer to this block.
	Name string `yaml:"name"`
	// Width is the number of state elements in the block, defaulting to 1.
	Width int `yaml:"width,omitempty"`
}

// ExpressionBinding is one named intermediate value in the evaluation DAG. Bindings are
// evaluated in order, and each may refer to any binding declared before it.
type ExpressionBinding struct {
	// Name is how later expressions refer to this value.
	Name string `yaml:"name"`
	// Expr is the expression computing it.
	Expr string `yaml:"expr"`
}

// ExpressionIteration is a declarative Iteration: the per-step update is given as string
// expressions rather than as Go, so a whole partition can be specified as data (and hence
// from YAML, or by an agent) with no compilation step.
//
// The update is a small DAG. Bindings are named intermediates evaluated in order, then one
// Outputs expression per field produces that field's next value. Everything is evaluated
// elementwise over vectors, with length-1 values broadcasting, so the same expression works
// for a scalar partition and a 10,000-element one.
//
// Names available to expressions:
//   - each of this partition's own fields, holding its current value;
//   - each entry of the partition's params, by key;
//   - each alias in Upstreams, holding that partition's current state (index it as alias[i]);
//   - dt, the timestep increment; t, the current cumulative time; step, the step number;
//   - pi;
//   - any binding declared earlier.
//
// Functions: where, clamp, min, max, abs, floor, exp, log, sqrt, pow, sin, cos, erf, erfc,
// fill, slice, concat, sum, dot, lag, each, iid and shared, plus the draws normal, uniform,
// exponential, poisson, gamma, beta and binomial. Draws take expressions as their parameters,
// so compound sampling composes naturally: a negative-binomial branching step is just
// poisson(gamma(shape, rate)).
//
// sin and cos carry seasonality; erfc is the primitive a Gaussian CDF is built from, as
// 0.5 * erfc(-x / sqrt(2)), which is what a probit link or a threshold-exceedance
// probability needs.
//
// # Reaching past the current element, and past the current row
//
// Everything above is elementwise over the current row, which four things in the model
// catalogue could not be written with. Each has one construct:
//
//   - slice(v, start, width) takes a block out of a vector, and concat(a, b, ...) joins
//     values, for state and params that pack several quantities end to end.
//   - lag(name, n) reads a partition's committed state n rows back, where a bare field name
//     or an Upstreams alias only ever gives row 0. The partition must keep that many rows
//     (its state_history_depth), and lag(x, 0) is x.
//   - each(n, i, expr) builds a width-n value whose element i is expr evaluated with the lane
//     index i bound. See below.
//
// each is the one construct that is not elementwise, and it is what makes an index shift,
// a per-lane guard and a per-lane draw order sayable:
//
//   - Element i may read element i-1 of something, so a cohort can age: each(60, age,
//     where(age == 0, births, survivors[age-1] * p)).
//   - Inside a lane every value is a scalar, so where is lazy there. A lane that is switched
//     off evaluates nothing and draws nothing — unlike a vector-guarded where, which must
//     evaluate both branches and so consumes randomness in every lane.
//   - Lanes run in order, so a lane's draws are taken before the next lane starts. An
//     elementwise gamma(shape, rate) followed by poisson takes every gamma before any
//     poisson; each(n, i, poisson(gamma(shape[i], rate[i]))) interleaves them per lane, the
//     way a loop over areas does.
//
// each binds only the index and returns only values, so there is still no assignment and no
// recursion, and an expression still always terminates. Draws inside it are explicit in the
// sense below, exactly as they are inside iid.
//
// Conditionals are expressions, not statements: where(cond, a, b). When cond is a scalar the
// untaken branch is not evaluated, so a guard such as where(n > 0, binomial(n, p), 0) is safe
// and draws no randomness on the guarded path. When cond is a vector both branches must be
// evaluated to select elementwise, as in NumPy, which means a vector-guarded draw consumes
// randomness in every lane. Prefer scalar guards where that matters — and see each below,
// which makes a per-element guard scalar and so gets laziness back.
//
// # How wide a draw is
//
// A draw produces one independent sample per element of its broadcast parameters, so
// poisson(rates) over a 40-wide rates gives 40 independent draws. When every parameter is a
// scalar the width is instead ambiguous: one sample reused across a field and forty
// independent ones are both reasonable readings, and both are things people mean. Rather than
// pick silently, a scalar-parameter draw is rejected unless the intent is stated:
//
//	iid(40, normal(0, 1))     forty independent samples
//	shared(normal(0, 1))      one sample, free to broadcast across a field
//
// So x + normal(0, 1) over a 40-wide x is an error rather than quietly adding the same shock
// to all forty elements. Draws with a vector parameter need no annotation.
//
// This is deliberately not a general-purpose language: there is no assignment and no
// recursion, and the only repetition is each's bounded comprehension, so an expression always
// terminates.
type ExpressionIteration struct {
	// Fields names the blocks of this partition's state, in layout order.
	Fields []ExpressionField `yaml:"fields"`
	// Upstreams maps an alias used in expressions to another partition's name, making that
	// partition's current state readable.
	Upstreams map[string]string `yaml:"upstreams,omitempty"`
	// Bindings are ordered named intermediates.
	Bindings []ExpressionBinding `yaml:"bindings,omitempty"`
	// Outputs holds one expression per entry of Fields, in the same order.
	Outputs []string `yaml:"outputs"`

	offsets        []int
	width          int
	upstreamIndex  map[string]int
	fieldIndex     map[string]int
	parsedBindings []parsedExprBinding
	parsedOutputs  []ast.Expr
	sampler        *rng.Sampler
	out            []float64
}

type parsedExprBinding struct {
	name string
	expr ast.Expr
}

func (e *ExpressionIteration) fieldWidth(i int) int {
	if w := e.Fields[i].Width; w != 0 {
		return w
	}
	return 1
}

// Configure resolves the state layout, resolves upstream partition names to indices, parses
// every expression once, and seeds the draw sampler from the partition's seed. It panics on a
// malformed specification: a partition that cannot be built is a configuration error, not
// something to discover mid-run.
func (e *ExpressionIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	if len(e.Outputs) != len(e.Fields) {
		panic(fmt.Sprintf(
			"expression: %d outputs for %d fields; there must be exactly one per field",
			len(e.Outputs), len(e.Fields)))
	}
	e.offsets = make([]int, len(e.Fields))
	e.fieldIndex = make(map[string]int, len(e.Fields))
	e.width = 0
	for i, f := range e.Fields {
		if f.Name == "" {
			panic("expression: field at index " + strconv.Itoa(i) + " has no name")
		}
		if f.Width < 0 {
			panic("expression: field " + f.Name + " has negative width")
		}
		e.offsets[i] = e.width
		e.width += e.fieldWidth(i)
		e.fieldIndex[f.Name] = i
	}
	e.out = make([]float64, e.width)

	e.upstreamIndex = make(map[string]int, len(e.Upstreams))
	for alias, name := range e.Upstreams {
		found := -1
		for i, it := range settings.Iterations {
			if it.Name == name {
				found = i
				break
			}
		}
		if found < 0 {
			panic("expression: upstream partition " + name + " (alias " + alias + ") not found")
		}
		e.upstreamIndex[alias] = found
	}

	e.parsedBindings = make([]parsedExprBinding, 0, len(e.Bindings))
	for _, b := range e.Bindings {
		parsed, err := parser.ParseExpr(b.Expr)
		if err != nil {
			panic("expression: parsing binding " + b.Name + ": " + err.Error())
		}
		e.parsedBindings = append(e.parsedBindings, parsedExprBinding{b.Name, parsed})
	}
	e.parsedOutputs = make([]ast.Expr, len(e.Outputs))
	for i, o := range e.Outputs {
		parsed, err := parser.ParseExpr(o)
		if err != nil {
			panic("expression: parsing output for field " +
				e.Fields[i].Name + ": " + err.Error())
		}
		e.parsedOutputs[i] = parsed
	}

	e.sampler = rng.New(settings.Iterations[partitionIndex].Seed)
}

// Iterate evaluates the bindings in order and then each field's output expression,
// concatenating the results into the next state.
func (e *ExpressionIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	env := make(exprEnv, len(e.Fields)+len(params.Map)+len(e.upstreamIndex)+3)
	state := stateHistories[partitionIndex].Values.RawRowView(0)
	for i, f := range e.Fields {
		env[f.Name] = exprValue(state[e.offsets[i] : e.offsets[i]+e.fieldWidth(i)])
	}
	for name, values := range params.Map {
		env[name] = exprValue(values)
	}
	for alias, index := range e.upstreamIndex {
		env[alias] = exprValue(stateHistories[index].Values.RawRowView(0))
	}
	env["dt"] = exprValue{timestepsHistory.NextIncrement}
	env["t"] = exprValue{timestepsHistory.Values.AtVec(0)}
	env["step"] = exprValue{float64(timestepsHistory.CurrentStepNumber)}
	env["pi"] = exprValue{math.Pi}

	// Resolved lazily rather than copied into env like the current row: a partition may hold
	// a deep history, and almost no expression reads past row 0.
	lag := func(name string, row int) exprValue {
		read := func(index int, from, width int) exprValue {
			history := stateHistories[index]
			if row < 0 || row >= history.StateHistoryDepth {
				panic(fmt.Sprintf(
					"expression: lag(%s, %d) is outside the %d rows %s keeps; raise its "+
						"state_history_depth", name, row, history.StateHistoryDepth, name))
			}
			return exprValue(history.Values.RawRowView(row)[from : from+width])
		}
		if index, ok := e.upstreamIndex[name]; ok {
			return read(index, 0, stateHistories[index].StateWidth)
		}
		if i, ok := e.fieldIndex[name]; ok {
			return read(partitionIndex, e.offsets[i], e.fieldWidth(i))
		}
		panic("expression: lag needs an upstream alias or one of this partition's own " +
			"fields, got " + name)
	}

	ctx := &exprCtx{env: env, sampler: e.sampler, lag: lag}
	for _, b := range e.parsedBindings {
		env[b.name] = ctx.eval(b.expr)
	}
	for i, o := range e.parsedOutputs {
		w := e.fieldWidth(i)
		v := ctx.eval(o)
		switch len(v) {
		case w:
			copy(e.out[e.offsets[i]:], v)
		case 1: // a scalar broadcasts across the whole field
			for k := 0; k < w; k++ {
				e.out[e.offsets[i]+k] = v[0]
			}
		default:
			panic(fmt.Sprintf(
				"expression: output for field %s produced width %d, want %d or 1",
				e.Fields[i].Name, len(v), w))
		}
	}
	return e.out
}

// exprValue is the single value type: a vector, where length 1 means a scalar and broadcasts
// against any other length.
type exprValue []float64

type exprEnv map[string]exprValue

// exprCtx carries the evaluation environment. drawsAreExplicit records whether evaluation is
// inside an iid, shared or each call, which is where a scalar-parameter draw is unambiguous.
type exprCtx struct {
	env              exprEnv
	sampler          *rng.Sampler
	lag              func(name string, row int) exprValue
	drawsAreExplicit bool
}

func (c *exprCtx) explicit() *exprCtx {
	return &exprCtx{env: c.env, sampler: c.sampler, lag: c.lag, drawsAreExplicit: true}
}

func exprBool(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// broadcastLen returns the result length for combining a and b, or panics on a mismatch.
func broadcastLen(a, b exprValue, what string) int {
	switch {
	case len(a) == len(b):
		return len(a)
	case len(a) == 1:
		return len(b)
	case len(b) == 1:
		return len(a)
	}
	panic(fmt.Sprintf("expression: cannot combine widths %d and %d in %s", len(a), len(b), what))
}

func at(v exprValue, i int) float64 {
	if len(v) == 1 {
		return v[0]
	}
	return v[i]
}

func zipExpr(a, b exprValue, what string, f func(x, y float64) float64) exprValue {
	n := broadcastLen(a, b, what)
	out := make(exprValue, n)
	for i := 0; i < n; i++ {
		out[i] = f(at(a, i), at(b, i))
	}
	return out
}

func mapExpr(a exprValue, f func(x float64) float64) exprValue {
	out := make(exprValue, len(a))
	for i, x := range a {
		out[i] = f(x)
	}
	return out
}

func (c *exprCtx) eval(node ast.Expr) exprValue {
	switch n := node.(type) {
	case *ast.BasicLit:
		v, err := strconv.ParseFloat(n.Value, 64)
		if err != nil {
			panic("expression: bad numeric literal " + n.Value)
		}
		return exprValue{v}
	case *ast.Ident:
		v, ok := c.env[n.Name]
		if !ok {
			panic("expression: unknown name " + n.Name)
		}
		return v
	case *ast.ParenExpr:
		return c.eval(n.X)
	case *ast.UnaryExpr:
		v := c.eval(n.X)
		switch n.Op {
		case token.SUB:
			return mapExpr(v, func(x float64) float64 { return -x })
		case token.ADD:
			return v
		case token.NOT:
			return mapExpr(v, func(x float64) float64 { return exprBool(x == 0) })
		}
	case *ast.IndexExpr:
		v := c.eval(n.X)
		idx := c.eval(n.Index)
		if len(idx) != 1 {
			panic("expression: index must be a scalar")
		}
		i := int(idx[0])
		if i < 0 || i >= len(v) {
			panic(fmt.Sprintf("expression: index %d out of range for width %d", i, len(v)))
		}
		return exprValue{v[i]}
	case *ast.BinaryExpr:
		return c.evalBinary(n)
	case *ast.CallExpr:
		return c.evalCall(n)
	}
	panic("expression: unsupported syntax")
}

func (c *exprCtx) evalBinary(n *ast.BinaryExpr) exprValue {
	// && and || short-circuit only when the left side is a scalar, matching where's rule.
	if n.Op == token.LAND || n.Op == token.LOR {
		l := c.eval(n.X)
		if len(l) == 1 {
			if n.Op == token.LAND && l[0] == 0 {
				return exprValue{0}
			}
			if n.Op == token.LOR && l[0] != 0 {
				return exprValue{1}
			}
			return mapExpr(c.eval(n.Y), func(y float64) float64 { return exprBool(y != 0) })
		}
		r := c.eval(n.Y)
		if n.Op == token.LAND {
			return zipExpr(l, r, "&&", func(x, y float64) float64 {
				return exprBool(x != 0 && y != 0)
			})
		}
		return zipExpr(l, r, "||", func(x, y float64) float64 {
			return exprBool(x != 0 || y != 0)
		})
	}
	l := c.eval(n.X)
	r := c.eval(n.Y)
	op := n.Op.String()
	switch n.Op {
	case token.ADD:
		return zipExpr(l, r, op, func(x, y float64) float64 { return x + y })
	case token.SUB:
		return zipExpr(l, r, op, func(x, y float64) float64 { return x - y })
	case token.MUL:
		return zipExpr(l, r, op, func(x, y float64) float64 { return x * y })
	case token.QUO:
		return zipExpr(l, r, op, func(x, y float64) float64 { return x / y })
	case token.REM:
		return zipExpr(l, r, op, math.Mod)
	case token.LSS:
		return zipExpr(l, r, op, func(x, y float64) float64 { return exprBool(x < y) })
	case token.GTR:
		return zipExpr(l, r, op, func(x, y float64) float64 { return exprBool(x > y) })
	case token.LEQ:
		return zipExpr(l, r, op, func(x, y float64) float64 { return exprBool(x <= y) })
	case token.GEQ:
		return zipExpr(l, r, op, func(x, y float64) float64 { return exprBool(x >= y) })
	case token.EQL:
		return zipExpr(l, r, op, func(x, y float64) float64 { return exprBool(x == y) })
	case token.NEQ:
		return zipExpr(l, r, op, func(x, y float64) float64 { return exprBool(x != y) })
	}
	panic("expression: unsupported operator " + op)
}

func (c *exprCtx) evalCall(n *ast.CallExpr) exprValue {
	ident, ok := n.Fun.(*ast.Ident)
	if !ok {
		panic("expression: unsupported call target")
	}
	name := ident.Name
	need := func(k int) {
		if len(n.Args) != k {
			panic(fmt.Sprintf("expression: %s takes %d arguments, got %d", name, k, len(n.Args)))
		}
	}
	arg := func(i int) exprValue { return c.eval(n.Args[i]) }

	switch name {
	case "where":
		// Lazy on a scalar condition, so a guarded branch neither divides by zero nor
		// consumes randomness.
		need(3)
		cond := arg(0)
		if len(cond) == 1 {
			if cond[0] != 0 {
				return arg(1)
			}
			return arg(2)
		}
		a, b := arg(1), arg(2)
		out := make(exprValue, len(cond))
		for i := range cond {
			if cond[i] != 0 {
				out[i] = at(a, i)
			} else {
				out[i] = at(b, i)
			}
		}
		return out
	case "iid":
		// Evaluate the expression n times over, giving n independent samples.
		need(2)
		nv := arg(0)
		if len(nv) != 1 {
			panic("expression: iid's count must be a scalar")
		}
		count := int(nv[0])
		if count < 1 {
			panic("expression: iid's count must be at least 1")
		}
		inner := c.explicit()
		out := make(exprValue, count)
		for i := 0; i < count; i++ {
			v := inner.eval(n.Args[1])
			if len(v) != 1 {
				panic(fmt.Sprintf(
					"expression: iid expects a scalar-valued expression, got width %d", len(v)))
			}
			out[i] = v[0]
		}
		return out
	case "shared":
		// One evaluation whose result may broadcast: the explicit way to say that a single
		// sample is meant to apply across a whole field.
		need(1)
		return c.explicit().eval(n.Args[0])
	case "each":
		// iid with the lane index in scope: a vector of width count whose element i is the
		// expression evaluated with that i bound.
		//
		// This is the one construct that is not elementwise, and it buys three things nothing
		// else can. Element i may read element i-1 of something (a cohort ages), so an index
		// shift becomes sayable. Everything inside a lane is a scalar, so where is lazy per
		// lane and a skipped lane draws nothing. And lanes run in order, so a draw per lane
		// interleaves the way a Go loop does rather than taking every gamma before any
		// poisson. It is a bounded comprehension with no assignment and no recursion, so an
		// expression still always terminates.
		need(3)
		countValue := arg(0)
		if len(countValue) != 1 {
			panic("expression: each's count must be a scalar")
		}
		count := int(countValue[0])
		if count < 1 {
			panic("expression: each's count must be at least 1")
		}
		index, ok := n.Args[1].(*ast.Ident)
		if !ok {
			panic("expression: each's second argument must be a name to bind the lane " +
				"index to, as in each(40, i, ...)")
		}
		inner := c.explicit()
		shadowed, wasBound := inner.env[index.Name]
		out := make(exprValue, count)
		for i := 0; i < count; i++ {
			inner.env[index.Name] = exprValue{float64(i)}
			v := inner.eval(n.Args[2])
			if len(v) != 1 {
				panic(fmt.Sprintf(
					"expression: each expects a scalar-valued expression per lane, got "+
						"width %d", len(v)))
			}
			out[i] = v[0]
		}
		// The env is shared with the enclosing scope, so the binding must not outlive the loop.
		if wasBound {
			inner.env[index.Name] = shadowed
		} else {
			delete(inner.env, index.Name)
		}
		return out
	case "lag":
		// A read of a partition's committed state further back than the current row, which is
		// all a bare name or an upstreams alias ever gives.
		need(2)
		name, ok := n.Args[0].(*ast.Ident)
		if !ok {
			panic("expression: lag's first argument must be an upstream alias or a field name")
		}
		rowValue := arg(1)
		if len(rowValue) != 1 {
			panic("expression: lag's row must be a scalar")
		}
		if c.lag == nil {
			panic("expression: lag is unavailable here")
		}
		return c.lag(name.Name, int(rowValue[0]))
	}

	switch name {
	case "clamp":
		need(3)
		x, lo, hi := arg(0), arg(1), arg(2)
		return zipExpr(zipExpr(x, lo, name, math.Max), hi, name, math.Min)
	case "min":
		need(2)
		return zipExpr(arg(0), arg(1), name, math.Min)
	case "max":
		need(2)
		return zipExpr(arg(0), arg(1), name, math.Max)
	case "pow":
		need(2)
		return zipExpr(arg(0), arg(1), name, math.Pow)
	case "abs":
		need(1)
		return mapExpr(arg(0), math.Abs)
	case "floor":
		need(1)
		return mapExpr(arg(0), math.Floor)
	case "exp":
		need(1)
		return mapExpr(arg(0), math.Exp)
	case "log":
		need(1)
		return mapExpr(arg(0), math.Log)
	case "sqrt":
		need(1)
		return mapExpr(arg(0), math.Sqrt)
	case "sin":
		need(1)
		return mapExpr(arg(0), math.Sin)
	case "cos":
		need(1)
		return mapExpr(arg(0), math.Cos)
	case "erf":
		need(1)
		return mapExpr(arg(0), math.Erf)
	case "erfc":
		// The primitive a Gaussian CDF is built from: 0.5 * erfc(-x / sqrt(2)). Kept as
		// erfc rather than offering the CDF directly so it matches math.Erfc exactly, which
		// is what lets a model expressed here agree with compiled Go to rounding.
		need(1)
		return mapExpr(arg(0), math.Erfc)
	case "fill":
		need(2)
		nv, x := arg(0), arg(1)
		if len(nv) != 1 {
			panic("expression: fill's width must be a scalar")
		}
		w := int(nv[0])
		if w < 1 {
			panic("expression: fill's width must be at least 1")
		}
		out := make(exprValue, w)
		for i := 0; i < w; i++ {
			out[i] = at(x, i)
		}
		return out
	case "slice":
		// A block of a vector, for state and params that pack several quantities end to end:
		// the nine coefficients of a channel inside one flat thirty-six-wide param, say.
		need(3)
		v, fromValue, widthValue := arg(0), arg(1), arg(2)
		if len(fromValue) != 1 || len(widthValue) != 1 {
			panic("expression: slice's start and width must be scalars")
		}
		from, width := int(fromValue[0]), int(widthValue[0])
		if width < 1 {
			panic("expression: slice's width must be at least 1")
		}
		if from < 0 || from+width > len(v) {
			panic(fmt.Sprintf(
				"expression: slice(%d, %d) is outside a width-%d value", from, width, len(v)))
		}
		return append(make(exprValue, 0, width), v[from:from+width]...)
	case "concat":
		// The other half of slice: assemble a field from pieces that are computed
		// differently, such as a cohort's boundary buckets against its interior.
		if len(n.Args) < 2 {
			panic("expression: concat takes at least 2 arguments, got " +
				strconv.Itoa(len(n.Args)))
		}
		out := make(exprValue, 0, len(n.Args))
		for i := range n.Args {
			out = append(out, arg(i)...)
		}
		return out
	case "sum":
		need(1)
		total := 0.0
		for _, x := range arg(0) {
			total += x
		}
		return exprValue{total}
	case "dot":
		need(2)
		a, b := arg(0), arg(1)
		nn := broadcastLen(a, b, name)
		total := 0.0
		for i := 0; i < nn; i++ {
			total += at(a, i) * at(b, i)
		}
		return exprValue{total}
	case "normal":
		need(2)
		return c.draw2(name, arg(0), arg(1), c.sampler.Normal)
	case "uniform":
		need(2)
		return c.draw2(name, arg(0), arg(1), c.sampler.Uniform)
	case "gamma":
		need(2)
		return c.draw2(name, arg(0), arg(1), c.sampler.Gamma)
	case "beta":
		need(2)
		return c.draw2(name, arg(0), arg(1), c.sampler.Beta)
	case "binomial":
		need(2)
		// pkg/rng leaves Binomial on distuv, whose three-branch algorithm was not worth
		// reimplementing. Draw from the sampler's own generator so a partition still has
		// exactly one stream and runs stay reproducible.
		return c.draw2(name, arg(0), arg(1), func(nn, p float64) float64 {
			return distuv.Binomial{N: nn, P: p, Src: c.sampler.Rand()}.Rand()
		})
	case "exponential":
		need(1)
		return c.draw1(name, arg(0), c.sampler.Exponential)
	case "poisson":
		need(1)
		return c.draw1(name, arg(0), c.sampler.Poisson)
	}
	panic("expression: unknown function " + name)
}

// checkDrawWidth rejects a draw whose parameters are all scalars unless the caller has said
// which reading is meant. See the type doc under "How wide a draw is".
func (c *exprCtx) checkDrawWidth(name string, width int) {
	if width == 1 && !c.drawsAreExplicit {
		panic(fmt.Sprintf(
			"expression: %s has only scalar parameters, so its width is ambiguous; "+
				"write iid(n, %s(...)) for n independent samples, or shared(%s(...)) for "+
				"one sample reused across the field",
			name, name, name))
	}
}

// draw1 applies a one-parameter draw elementwise; each element is an independent sample.
func (c *exprCtx) draw1(name string, a exprValue, f func(x float64) float64) exprValue {
	c.checkDrawWidth(name, len(a))
	return mapExpr(a, f)
}

// draw2 applies a two-parameter draw elementwise, broadcasting the parameters; each element
// is an independent sample.
func (c *exprCtx) draw2(
	name string,
	a, b exprValue,
	f func(x, y float64) float64,
) exprValue {
	n := broadcastLen(a, b, name)
	c.checkDrawWidth(name, n)
	out := make(exprValue, n)
	for i := 0; i < n; i++ {
		out[i] = f(at(a, i), at(b, i))
	}
	return out
}
