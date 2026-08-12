package lob

// Does declarative.yaml — this model as data, with no Go — reproduce the model stub.go
// builds in code? For this entry the question is inverted from the rest of the catalogue:
// the declarative form is the one the downstream repo actually runs, and the Go is the
// re-expression. So this test is what pins the Go to the data.
//
// Two independent checks, because they fail in different ways. The step-for-step tests
// compare single Iterate calls over randomised inputs, which catches a mis-stated formula
// exactly. The suite test re-runs the model's own claim computations against the declarative
// build, which catches what per-step agreement cannot: wrong wiring, wrong param values,
// wrong state layout, a missing partition.
//
// The oracle is EXACT (~1e-12). The bespoke stochastic partitions draw from rng.New(seed) —
// the same generator the evaluator's sampler is, seeded the same way — and take the same
// number of draws per step in the same order (arr_bid, arr_ask, churn ×2, cancellation ×2,
// then one marketable Poisson and one Binomial), so the two run on one shared stream and
// equivalence is decidable directly. Agreement is asserted to a tight tolerance rather than
// bit-for-bit: compiled Go may contract a + b*c into a fused multiply-add, which rounds
// differently from the evaluator's separate operations. The deterministic partitions (book,
// observables) take no draws and agree for the same reason value-for-value.

import (
	"math"
	"math/rand/v2"
	"testing"

	"gonum.org/v1/gonum/mat"

	"github.com/umbralcalc/stochadex/models/cardgen"
	"github.com/umbralcalc/stochadex/pkg/api"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// tolerance is well above FMA rounding and well below any difference a real modelling
// discrepancy would produce.
const tolerance = 1e-12

// declarativeBuildStub assembles the model from declarative.yaml, matching BuildStub's
// signature so it can be dropped into the behaviour helpers. The YAML holds the model; the
// run knobs BuildStub takes as arguments (the damping exponent, horizon, seed) are injected
// here exactly as BuildStub injects them into its Go partitions — including the per-partition
// seed offsets, so both forms run on identical streams.
func declarativeBuildStub(
	dampingGamma float64,
	numSteps int,
	seed uint64,
) *simulator.ConfigGenerator {
	config := api.LoadApiRunConfigFromYaml("declarative.yaml")
	config.Main.Simulation = simulator.SimulationConfig{
		OutputCondition: &simulator.EveryStepOutputCondition{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: numSteps,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:    0.0,
	}
	gen := config.Main.GetConfigGenerator()
	gen.GetPartition("flows").Params.Map["damping_gamma"] = []float64{dampingGamma}
	gen.GetPartition("activity").Seed = seed
	gen.GetPartition("flows").Seed = seed + 1
	gen.GetPartition("book").Seed = seed + 2
	gen.GetPartition("observables").Seed = seed + 3
	return gen
}

// declarativeIteration returns the expression iteration the YAML supplies for a partition.
func declarativeIteration(t *testing.T, partition string) *general.ExpressionIteration {
	t.Helper()
	config := declarativeBuildStub(DefaultDampingGamma, 10, 0).GetPartition(partition)
	iteration, ok := config.Iteration.(*general.ExpressionIteration)
	if !ok {
		t.Fatalf("%s is not expression-backed: got %T", partition, config.Iteration)
	}
	return iteration
}

// assertClose fails when two values differ by more than the FMA-scale tolerance, comparing
// relatively once the magnitude exceeds 1.
func assertClose(t *testing.T, got, want float64, context string) float64 {
	t.Helper()
	d := math.Abs(got - want)
	if s := math.Abs(want); s > 1 {
		d /= s
	}
	if d > tolerance {
		t.Fatalf("%s: declarative=%v bespoke=%v deviation=%g", context, got, want, d)
	}
	return d
}

func hist(width int, row []float64) *simulator.StateHistory {
	return &simulator.StateHistory{
		Values:            mat.NewDense(1, width, append([]float64{}, row...)),
		StateWidth:        width,
		StateHistoryDepth: 1,
	}
}

func tsAt(step int, dt float64) *simulator.CumulativeTimestepsHistory {
	return &simulator.CumulativeTimestepsHistory{
		Values:            mat.NewVecDense(1, []float64{float64(step) * dt}),
		NextIncrement:     dt,
		CurrentStepNumber: step,
	}
}

// TestDeclarativeActivityMatchesBespoke — the AR(1) driver, one Gamma draw per step. Sweeps
// the Gamma shape across its three algorithm branches (< 0.2, [0.2, 1), ≥ 1) so the
// comparison is not silently confined to one sampler path.
func TestDeclarativeActivityMatchesBespoke(t *testing.T) {
	bespoke := &ActivityIteration{}
	declarative := declarativeIteration(t, "activity")
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{{Name: "activity", Seed: 5}},
	}
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	rng := rand.New(rand.NewPCG(11, 12))
	branches := map[string]int{}
	maxDev := 0.0

	const cases = 20000
	for i := 0; i < cases; i++ {
		// Choose a shape that lands in each Gamma branch a meaningful fraction of the time.
		var shape float64
		switch i % 3 {
		case 0:
			shape = 0.05 + rng.Float64()*0.1 // < 0.2 rejection loop
			branches["shape_small"]++
		case 1:
			shape = 0.2 + rng.Float64()*0.75 // [0.2, 1) boosted Marsaglia–Tsang
			branches["shape_mid"]++
		default:
			shape = 1.0 + rng.Float64()*4 // ≥ 1 Marsaglia–Tsang
			branches["shape_large"]++
		}
		params := simulator.NewParams(map[string][]float64{
			"persistence":    {rng.Float64()},
			"activity_shape": {shape},
			"activity_rate":  {0.1 + rng.Float64()*2},
		})
		state := []float64{rng.Float64() * 10}
		ts := tsAt(i+1, 0.5+rng.Float64()*3)

		want := bespoke.Iterate(&params, 0, []*simulator.StateHistory{hist(1, state)}, ts)
		got := declarative.Iterate(&params, 0, []*simulator.StateHistory{hist(1, state)}, ts)
		maxDev = math.Max(maxDev, assertClose(t, got[0], want[0], "activity"))
	}
	for _, b := range []string{"shape_small", "shape_mid", "shape_large"} {
		if branches[b] == 0 {
			t.Errorf("Gamma branch %q never exercised", b)
		}
	}
	t.Logf("branches: %v  max deviation: %g", branches, maxDev)
}

// flowsParams builds a randomised, valid set of flow params (activity and act_ref strictly
// positive so the damping pow is well-defined). cancelRatePos and shockStep let a caller
// force the cancellation-Poisson and the price-shock branches.
func flowsParams(rng *rand.Rand, dt float64, cancelRate, shockStep, shockSize float64) simulator.Params {
	return simulator.NewParams(map[string][]float64{
		"limit_rate":    {0.5 + rng.Float64()*5},
		"arrival_decay": {0.1 + rng.Float64()*0.6},
		"cancel_rate":   {cancelRate},
		"arrival_scale": {5 + rng.Float64()*40},
		"churn_rate":    {0.5 + rng.Float64()*3},
		"act_ref":       {2 + rng.Float64()*4},
		"damping_gamma": {rng.Float64() * 1.2},
		"market_rate":   {0.2 + rng.Float64()*3},
		"market_size":   {1 + rng.Float64()*7},
		"half":          {0.5},
		"shock_step":    {shockStep},
		"shock_size":    {shockSize},
		"activity":      {0.5 + rng.Float64()*8},
	})
}

// TestDeclarativeFlowsMatchesBespoke — the stochastic flows partition (48 Poissons + 1
// Binomial per step). The draw count is fixed, so the two stay in lockstep regardless of
// params; the branches swept here are the deterministic ones the outputs turn on (the
// cancellation clip and the price shock) plus small and large Poisson rates, which cross the
// sampler's λ=10 algorithm switch.
func TestDeclarativeFlowsMatchesBespoke(t *testing.T) {
	bespoke := &FlowsIteration{}
	declarative := declarativeIteration(t, "flows")
	// settings.Iterations[0] is flows (seed → sampler; book_partition → the lag read);
	// index 1 is book, so the upstreams alias book_prev resolves.
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{
			{Name: "flows", Seed: 9, Params: simulator.NewParams(map[string][]float64{"book_partition": {1}})},
			{Name: "book", Seed: 0},
		},
	}
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	rng := rand.New(rand.NewPCG(21, 22))
	branches := map[string]int{}
	maxDev := 0.0

	const cases = 8000
	for i := 0; i < cases; i++ {
		dt := []float64{0.5, 1.0, 2.0}[i%3]
		cancelRate := 0.0
		if i%2 == 0 {
			cancelRate = rng.Float64() * 0.5
			branches["cancel_on"]++
		} else {
			branches["cancel_off"]++
		}
		step := i + 1
		shockStep := -1.0
		shockSize := 0.0
		if i%5 == 0 {
			shockStep = float64(step) // fire the price shock this step
			shockSize = rng.Float64() * 20
			branches["shock_fired"]++
		} else {
			branches["shock_quiet"]++
		}
		params := flowsParams(rng, dt, cancelRate, shockStep, shockSize)

		// A resting book one step back: small depths so the cancellation clip (churn > bid)
		// bites often, exercising clip_binds.
		bookRow := make([]float64, 2*Levels+1)
		for j := 0; j < 2*Levels; j++ {
			bookRow[j] = math.Floor(rng.Float64() * 6)
		}
		flowsHist := hist(4*Levels+3, make([]float64, 4*Levels+3))
		bookHist := hist(2*Levels+1, bookRow)
		hs := []*simulator.StateHistory{flowsHist, bookHist}
		ts := tsAt(step, dt)

		want := bespoke.Iterate(&params, 0, hs, ts)
		got := declarative.Iterate(&params, 0, hs, ts)

		if len(got) != len(want) {
			t.Fatalf("case %d: width %d vs %d", i, len(got), len(want))
		}
		clipSeen := got[4*Levels+2]
		if clipSeen > 0 {
			branches["clip_bit"]++
		}
		for k := range want {
			maxDev = math.Max(maxDev, assertClose(t, got[k], want[k], "flows"))
		}
	}
	for _, b := range []string{"cancel_on", "cancel_off", "shock_fired", "shock_quiet", "clip_bit"} {
		if branches[b] == 0 {
			t.Errorf("flows branch %q never exercised", b)
		}
	}
	t.Logf("branches: %v  max deviation: %g", branches, maxDev)
}

// TestDeclarativeBookMatchesBespoke — the deterministic matching engine. Sweeps marketable
// order sizes from zero to large so the book-walk clamp is exercised both partially (an order
// stopped inside a level) and fully (an order sweeping several levels), and a no-trade step.
func TestDeclarativeBookMatchesBespoke(t *testing.T) {
	bespoke := &BookIteration{}
	declarative := declarativeIteration(t, "book")
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{{Name: "book", Seed: 0}},
	}
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	rng := rand.New(rand.NewPCG(31, 32))
	branches := map[string]int{}
	maxDev := 0.0

	const cases = 20000
	for i := 0; i < cases; i++ {
		// Flow layout: arr_bid[8], arr_ask[8], can_bid[8], can_ask[8], buy, sell, clip.
		flow := make([]float64, 4*Levels+3)
		bookRow := make([]float64, 2*Levels+1)
		for j := 0; j < 2*Levels; j++ {
			bookRow[j] = math.Floor(rng.Float64() * 8)       // resting depth
			flow[2*Levels+j] = math.Floor(rng.Float64() * 3) // cancellations (≤ resting handled by flows)
			flow[j] = math.Floor(rng.Float64() * 4)          // arrivals
		}
		// Order sizes: sometimes 0, sometimes small, sometimes large enough to sweep out.
		var buy, sell float64
		switch i % 4 {
		case 0:
			buy, sell = 0, 0
			branches["no_trade"]++
		case 1:
			buy, sell = math.Floor(rng.Float64()*4), math.Floor(rng.Float64()*4)
			branches["small"]++
		default:
			buy, sell = math.Floor(rng.Float64()*60), math.Floor(rng.Float64()*60)
			branches["large"]++
		}
		flow[4*Levels] = buy
		flow[4*Levels+1] = sell

		params := simulator.NewParams(map[string][]float64{"flow": flow})
		hs := []*simulator.StateHistory{hist(2*Levels+1, bookRow)}
		ts := tsAt(i+1, 1.0)

		want := bespoke.Iterate(&params, 0, hs, ts)
		got := declarative.Iterate(&params, 0, hs, ts)
		if got[2*Levels] > 0 {
			branches["swept"]++
		}
		for k := range want {
			maxDev = math.Max(maxDev, assertClose(t, got[k], want[k], "book"))
		}
	}
	for _, b := range []string{"no_trade", "small", "large", "swept"} {
		if branches[b] == 0 {
			t.Errorf("book branch %q never exercised", b)
		}
	}
	t.Logf("branches: %v  max deviation: %g", branches, maxDev)
}

// TestDeclarativeObservablesMatchesBespoke — the deterministic observables. Sweeps between a
// live two-sided book (a real spread) and a one-sided book (the empty-spread sentinel), and
// varies which level is nearest the touch so the best-level selector is exercised.
func TestDeclarativeObservablesMatchesBespoke(t *testing.T) {
	bespoke := &ObservablesIteration{}
	declarative := declarativeIteration(t, "observables")
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{
			{Name: "observables", Seed: 0, Params: simulator.NewParams(map[string][]float64{"book_partition": {1}})},
			{Name: "book", Seed: 0},
		},
	}
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	rng := rand.New(rand.NewPCG(41, 42))
	branches := map[string]int{}
	maxDev := 0.0

	const cases = 20000
	for i := 0; i < cases; i++ {
		flow := make([]float64, 4*Levels+3)
		for j := range flow {
			flow[j] = math.Floor(rng.Float64() * 5)
		}
		bookNow := make([]float64, 2*Levels+1)
		bookPrev := make([]float64, 2*Levels+1)
		for j := 0; j < 2*Levels+1; j++ {
			bookNow[j] = math.Floor(rng.Float64() * 6)
			bookPrev[j] = math.Floor(rng.Float64() * 6)
		}
		// Force each occupancy regime: a one-sided book (empty bids) hits the sentinel; an
		// empty near-touch level exercises the best-level walk past a gap.
		switch i % 4 {
		case 0:
			for j := 0; j < Levels; j++ {
				bookNow[j] = 0 // empty bid side → one-sided
			}
			branches["one_sided"]++
		case 1:
			bookNow[0] = 0 // gap at the touch on the bid side
			branches["bid_gap"]++
		default:
			branches["two_sided"]++
		}

		params := simulator.NewParams(map[string][]float64{
			"flow":         flow,
			"book_now":     bookNow,
			"empty_spread": {DefaultEmptySpread},
		})
		hs := []*simulator.StateHistory{hist(6, make([]float64, 6)), hist(2*Levels+1, bookPrev)}
		ts := tsAt(i+1, 1.0)

		want := bespoke.Iterate(&params, 0, hs, ts)
		got := declarative.Iterate(&params, 0, hs, ts)
		if got[4] >= DefaultEmptySpread {
			branches["sentinel_out"]++
		} else {
			branches["live_spread"]++
		}
		for k := range want {
			maxDev = math.Max(maxDev, assertClose(t, got[k], want[k], "observables"))
		}
	}
	for _, b := range []string{"one_sided", "bid_gap", "two_sided", "sentinel_out", "live_spread"} {
		if branches[b] == 0 {
			t.Errorf("observables branch %q never exercised", b)
		}
	}
	t.Logf("branches: %v  max deviation: %g", branches, maxDev)
}

// TestDeclarativeAnswersTheSameClaims — the suite oracle: every response claim recomputed
// against the declarative build must still hold, and hold with the same numbers the card
// reports, not merely point the same way. This is what catches wrong wiring or a wrong param
// value that per-step agreement (which feeds its own inputs) cannot see.
func TestDeclarativeAnswersTheSameClaims(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the full claim ensemble twice")
	}
	bespoke := ObservedBehaviour()
	declarative := observedBehaviour(declarativeBuildStub)

	if len(declarative) != len(bespoke) {
		t.Fatalf("got %d claims, want %d", len(declarative), len(bespoke))
	}
	maxDev := 0.0
	for i, claim := range declarative {
		reference := bespoke[i]
		if claim.ID != reference.ID {
			t.Fatalf("claim %d: got ID %q, want %q", i, claim.ID, reference.ID)
		}
		if err := cardgen.Verify(claim); err != nil {
			t.Errorf("claim %q does not hold for the declarative model: %v", claim.ID, err)
			continue
		}
		if len(claim.Observations) != len(reference.Observations) {
			t.Fatalf("claim %q: got %d observations, want %d",
				claim.ID, len(claim.Observations), len(reference.Observations))
		}
		for k, obs := range claim.Observations {
			maxDev = math.Max(maxDev, assertClose(t, obs.Value,
				reference.Observations[k].Value, claim.ID+" / "+obs.Label))
		}
	}
	t.Logf("max deviation across every observation on every claim: %g", maxDev)
}
