package rugby

// Does declarative.yaml — this model written as data — reproduce the model stub.go builds
// in code?
//
// Two independent checks, because they fail in different ways. The step-for-step tests
// compare single Iterate calls over randomised inputs, which catches a mis-stated formula
// exactly. The suite test re-runs the model's own claim computations against the
// declarative build, which catches a model that is subtly different in a way per-step
// agreement would not: wrong wiring, wrong param values, wrong state layout.
//
// Every stochastic partition here draws from a stream seeded exactly as the declarative one
// is (rng.New(seed) and rand.New(rand.NewPCG(seed, seed)) are the same generator), and takes
// the same number of draws in the same order per step, so the two stay in lockstep and
// equivalence is decidable directly rather than only in distribution. Two facts make that
// possible: sampler.Uniform(0, 1) is Float64()*(1-0)+0, i.e. exactly the Float64() the Cox
// process and the conversion iteration draw; and iid(n, ...) evaluates its expression n
// times in order, which is the same n draws in the same order as the Go loops it stands in
// for. Agreement is asserted to a tight tolerance rather than bit-for-bit: compiled Go is
// free to contract a + b*c into a fused multiply-add, which rounds differently from the
// evaluator's separate operations.
//
// Every partition is compared, match_state included: its lag-10 read of the card_events
// history — once the one thing here with no declarative form, since an upstreams alias only
// ever gives row 0 — is now lag(card_hist, 10), so the declarative build carries no Go at
// all.

import (
	"math"
	"math/rand/v2"
	"testing"

	"gonum.org/v1/gonum/mat"

	"github.com/umbralcalc/stochadex/models/cardgen"
	"github.com/umbralcalc/stochadex/pkg/api"
	"github.com/umbralcalc/stochadex/pkg/discrete"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// tolerance is well above FMA rounding and well below any difference a real modelling
// discrepancy would produce.
const tolerance = 1e-12

// subMinutes flattens a strategy into the sub_minutes param: the four home groups followed
// by the four away groups, which is the covariate layout the rate coefficients index.
func subMinutes(strategy *SubstitutionStrategy) []float64 {
	minutes := make([]float64, SubCovWidth)
	for g := 0; g < NumPositionGroups; g++ {
		minutes[g] = float64(strategy.HomeSubs[g])
		minutes[NumPositionGroups+g] = float64(strategy.AwaySubs[g])
	}
	return minutes
}

// declarativeBuildStub assembles the model from declarative.yaml, matching
// buildStubWithStrategy's signature so it can be dropped into the behaviour helpers. The
// YAML holds the model; the run knobs the Go builder takes as arguments are injected here,
// exactly as it injects them into its Go partitions. No iteration is attached: every
// partition, match_state included, comes out of the file.
func declarativeBuildStub(
	strategy *SubstitutionStrategy,
	numSteps int,
	seed uint64,
) *simulator.ConfigGenerator {
	config := api.LoadApiRunConfigFromYaml("declarative.yaml")
	config.Main.Simulation = simulator.SimulationConfig{
		OutputCondition: &simulator.EveryStepOutputCondition{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: numSteps,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: stepSizeMinutes},
		InitTimeValue:    0.0,
	}
	gen := config.Main.GetConfigGenerator()
	gen.GetPartition("sub_covariates").Params.Map["sub_minutes"] = subMinutes(strategy)
	gen.GetPartition("score_events").Seed = seed
	gen.GetPartition("card_events").Seed = seed + 1
	gen.GetPartition("conversion_events").Seed = seed + 2
	return gen
}

// declarativeIteration returns the expression iteration the YAML supplies for a partition,
// alongside the params it declares.
func declarativeIteration(
	t *testing.T,
	partition string,
) (*general.ExpressionIteration, map[string][]float64) {
	t.Helper()
	config := declarativeBuildStub(
		DefaultSubstitutionStrategy(DefaultHomeSubMinute),
		DefaultNumSteps,
		0,
	).GetPartition(partition)
	iteration, ok := config.Iteration.(*general.ExpressionIteration)
	if !ok {
		t.Fatalf("%s is not expression-backed: got %T", partition, config.Iteration)
	}
	return iteration, config.Params.Map
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

// singleIteration is a settings block for comparing one partition in isolation, seeded so
// both sides build the same generator.
func singleIteration(name string, seed uint64) *simulator.Settings {
	return &simulator.Settings{
		Iterations: []simulator.IterationSettings{{Name: name, Seed: seed}},
	}
}

// history wraps one row of state as a partition's history, fresh per call so that neither
// iteration can see the other's writes.
func history(values []float64, depth int) *simulator.StateHistory {
	rows := mat.NewDense(depth, len(values), nil)
	rows.SetRow(0, values)
	return &simulator.StateHistory{
		Values:            rows,
		StateWidth:        len(values),
		StateHistoryDepth: depth,
	}
}

// steps returns a timesteps history positioned at step number i with the given increment.
func steps(i int, increment float64) *simulator.CumulativeTimestepsHistory {
	return &simulator.CumulativeTimestepsHistory{
		Values:            mat.NewVecDense(1, []float64{float64(i)}),
		NextIncrement:     increment,
		CurrentStepNumber: i,
	}
}

func TestDeclarativeSubCovariatesMatchBespoke(t *testing.T) {
	declarative, _ := declarativeIteration(t, "sub_covariates")
	declarative.Configure(0, singleIteration("sub_covariates", 5))

	rng := rand.New(rand.NewPCG(31, 32))
	branches := map[string]int{}
	maxDev := 0.0

	const cases = 400
	for c := 0; c < cases; c++ {
		// Minute 0 means "never substitute this group", which is a different branch from a
		// minute the match simply has not reached yet, so both are drawn here.
		strategy := &SubstitutionStrategy{}
		for g := 0; g < NumPositionGroups; g++ {
			if rng.IntN(4) > 0 {
				strategy.HomeSubs[g] = rng.IntN(DefaultNumSteps)
			}
			if rng.IntN(4) > 0 {
				strategy.AwaySubs[g] = rng.IntN(DefaultNumSteps)
			}
		}
		minutes := subMinutes(strategy)
		p := simulator.NewParams(map[string][]float64{"sub_minutes": minutes})
		bespoke := &general.FromStorageIteration{
			Data: strategy.GenerateCovariates(DefaultNumSteps + 1),
		}
		bespoke.Configure(0, singleIteration("sub_covariates", 5))

		for step := 1; step <= DefaultNumSteps; step++ {
			ts := steps(step, stepSizeMinutes)
			want := bespoke.Iterate(&p, 0,
				[]*simulator.StateHistory{history(make([]float64, SubCovWidth), 1)}, ts)
			got := declarative.Iterate(&p, 0,
				[]*simulator.StateHistory{history(make([]float64, SubCovWidth), 1)}, ts)

			for k := range want {
				switch {
				case minutes[k] == 0:
					branches["never_substituted"]++
				case float64(step) < minutes[k]:
					branches["before_minute"]++
				default:
					branches["after_minute"]++
				}
				maxDev = math.Max(maxDev, assertClose(t, got[k], want[k], "covariate"))
			}
		}
	}
	for _, b := range []string{"never_substituted", "before_minute", "after_minute"} {
		if branches[b] == 0 {
			t.Errorf("branch %q never exercised; the comparison is weaker than it looks", b)
		}
	}
	t.Logf("branches exercised: %v", branches)
	t.Logf("max deviation: %g", maxDev)
}

// rateCase draws a randomised log-linear rate problem: coefficients for width channels, the
// eight substitution covariates, and a baseline that is zero often enough to sweep both
// sides of the per-channel baseline guard.
func rateCase(
	rng *rand.Rand,
	width int,
) (coefficients, covariates, baseline []float64) {
	coefficients = make([]float64, width*CoeffsPerRate)
	for i := range coefficients {
		coefficients[i] = rng.Float64()*6 - 4
	}
	covariates = make([]float64, SubCovWidth)
	for i := range covariates {
		// Binary in the model proper, but continuous values are drawn too: they make a
		// dropped or misplaced coefficient visible, which a 0/1 covariate can hide.
		if rng.IntN(2) == 0 {
			covariates[i] = float64(rng.IntN(2))
		} else {
			covariates[i] = rng.Float64() * 2
		}
	}
	baseline = make([]float64, RateEventWidth)
	for i := range baseline {
		if rng.IntN(2) == 0 {
			baseline[i] = rng.Float64() * 3
		}
	}
	return coefficients, covariates, baseline
}

// assertRatesMatch compares a rate partition against its bespoke rate function, counting
// how often each side of the baseline guard is taken over the channels at baseOffset.
func assertRatesMatch(
	t *testing.T,
	partition string,
	function func(
		params *simulator.Params,
		partitionIndex int,
		stateHistories []*simulator.StateHistory,
		timestepsHistory *simulator.CumulativeTimestepsHistory,
	) []float64,
	width, baseOffset int,
	seed uint64,
) {
	t.Helper()
	bespoke := &general.ValuesFunctionIteration{Function: function}
	declarative, _ := declarativeIteration(t, partition)
	settings := singleIteration(partition, 5)
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	rng := rand.New(rand.NewPCG(seed, seed+1))
	branches := map[string]int{}
	maxDev := 0.0

	const cases = 5000
	for i := 0; i < cases; i++ {
		coefficients, covariates, baseline := rateCase(rng, width)
		p := simulator.NewParams(map[string][]float64{
			"coefficients": coefficients,
			"covariates":   covariates,
			"baseline":     baseline,
		})
		ts := steps(i+1, stepSizeMinutes)
		want := function(&p, 0, []*simulator.StateHistory{history(make([]float64, width), 1)}, ts)
		got := declarative.Iterate(&p, 0,
			[]*simulator.StateHistory{history(make([]float64, width), 1)}, ts)

		for k := 0; k < width; k++ {
			if baseline[baseOffset+k] > 0 {
				branches["baseline_applied"]++
			} else {
				branches["no_baseline"]++
			}
			maxDev = math.Max(maxDev, assertClose(t, got[k], want[k], partition))
		}
	}
	for _, b := range []string{"baseline_applied", "no_baseline"} {
		if branches[b] == 0 {
			t.Errorf("branch %q never exercised; the comparison is weaker than it looks", b)
		}
	}
	t.Logf("%s branches exercised: %v", partition, branches)
	t.Logf("%s max deviation: %g", partition, maxDev)
}

// The intercept-only and no-baseline fallbacks inside computeRates dispatch on the LENGTH of
// the coefficient and baseline vectors, which the declarative model fixes by construction
// (36 or 18 coefficients, a 6-wide baseline from upstream). They are unreachable in this
// stub's wiring and have no declarative counterpart — the DSL cannot introspect a width —
// so they are deliberately not swept here.
func TestDeclarativeScoreRatesMatchBespoke(t *testing.T) {
	assertRatesMatch(t, "score_rates", ScoreEventRateFunction, ScoreRateWidth, 0, 41)
}

func TestDeclarativeCardRatesMatchBespoke(t *testing.T) {
	assertRatesMatch(t, "card_rates", CardEventRateFunction, CardRateWidth, ScoreRateWidth, 51)
}

// assertCoxMatches compares a Cox-process partition against discrete.CoxProcessIteration.
// Both are configured at the same seed and draw exactly one uniform per channel per step, so
// the streams stay aligned across the whole sweep and the thinning decisions must agree
// case for case, not merely on average.
func assertCoxMatches(t *testing.T, partition string, width int, seed uint64) {
	t.Helper()
	bespoke := &discrete.CoxProcessIteration{}
	declarative, _ := declarativeIteration(t, partition)
	settings := singleIteration(partition, 5)
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	rng := rand.New(rand.NewPCG(seed, seed+1))
	branches := map[string]int{}
	maxDev := 0.0

	const cases = 20000
	counts := make([]float64, width)
	for i := 0; i < cases; i++ {
		rates := make([]float64, width)
		for k := range rates {
			// Spanning rates that almost never fire and rates that almost always do, so the
			// thinning threshold is compared across its whole range rather than in the tail
			// the calibrated intercepts happen to sit in.
			rates[k] = math.Exp(rng.Float64()*6 - 5)
		}
		// The increment enters the threshold as 1/dt, so it is varied rather than pinned at
		// the model's one minute.
		increment := []float64{1.0, 0.5, 2.0}[i%3]
		p := simulator.NewParams(map[string][]float64{"rates": rates})
		ts := steps(i+1, increment)

		want := bespoke.Iterate(&p, 0, []*simulator.StateHistory{history(counts, 2)}, ts)
		wantCopy := append([]float64(nil), want...)
		got := declarative.Iterate(&p, 0, []*simulator.StateHistory{history(counts, 2)}, ts)

		for k := 0; k < width; k++ {
			if wantCopy[k] > counts[k] {
				branches["event_fired"]++
			} else {
				branches["no_event"]++
			}
			maxDev = math.Max(maxDev, assertClose(t, got[k], wantCopy[k], partition))
		}
		copy(counts, wantCopy)
	}
	for _, b := range []string{"event_fired", "no_event"} {
		if branches[b] == 0 {
			t.Errorf("branch %q never exercised; the comparison is weaker than it looks", b)
		}
	}
	t.Logf("%s branches exercised: %v", partition, branches)
	t.Logf("%s max deviation: %g", partition, maxDev)
}

func TestDeclarativeScoreEventsMatchBespoke(t *testing.T) {
	assertCoxMatches(t, "score_events", ScoreRateWidth, 61)
}

func TestDeclarativeCardEventsMatchBespoke(t *testing.T) {
	assertCoxMatches(t, "card_events", CardRateWidth, 71)
}

func TestDeclarativeConversionsMatchBespoke(t *testing.T) {
	// Index 1 is score_events: the bespoke iteration finds it through the
	// score_events_partition param, the declarative one through its score_prev upstream
	// alias. Both are the same lag-1 read of the same partition.
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{
			{
				Name: "conversion_events",
				Seed: 5,
				Params: simulator.NewParams(map[string][]float64{
					"score_events_partition": {1},
				}),
			},
			{Name: "score_events", Seed: 0},
		},
	}
	bespoke := &ConversionIteration{}
	declarative, _ := declarativeIteration(t, "conversion_events")
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	rng := rand.New(rand.NewPCG(81, 82))
	branches := map[string]int{}
	maxDev := 0.0

	const cases = 20000
	conv := []float64{0, 0}
	prevTries := []float64{0, 0, 0, 0}
	for i := 0; i < cases; i++ {
		// A step with no tries must draw nothing at all, or every later step is compared
		// against a stream that has slipped — so the zero case is swept heavily.
		newHome, newAway := 0, 0
		if i%3 == 0 {
			newHome = rng.IntN(3)
		}
		if i%4 == 0 {
			newAway = rng.IntN(3)
		}
		tries := []float64{
			prevTries[0] + float64(newHome),
			prevTries[1] + float64(newAway),
			prevTries[2],
			prevTries[3],
		}
		// Probabilities span both extremes so successes and misses both occur.
		probs := []float64{rng.Float64(), rng.Float64()}
		p := simulator.NewParams(map[string][]float64{
			"conversion_probs": probs,
			"try_values":       tries,
		})
		ts := steps(i+1, stepSizeMinutes)
		mk := func() []*simulator.StateHistory {
			return []*simulator.StateHistory{
				history(conv, 1),
				history(prevTries, 2),
			}
		}

		want := bespoke.Iterate(&p, 0, mk(), ts)
		got := declarative.Iterate(&p, 0, mk(), ts)

		homeHits := int(want[0] - conv[0])
		awayHits := int(want[1] - conv[1])
		if newHome+newAway == 0 {
			branches["no_new_tries"]++
		} else {
			branches["new_tries"]++
			if homeHits+awayHits > 0 {
				branches["conversion_made"]++
			}
			if homeHits+awayHits < newHome+newAway {
				branches["conversion_missed"]++
			}
		}
		for k := range want {
			maxDev = math.Max(maxDev, assertClose(t, got[k], want[k], "conversions"))
		}
		conv = append([]float64(nil), want...)
		prevTries = tries
	}
	for _, b := range []string{
		"no_new_tries", "new_tries", "conversion_made", "conversion_missed",
	} {
		if branches[b] == 0 {
			t.Errorf("branch %q never exercised; the comparison is weaker than it looks", b)
		}
	}
	t.Logf("branches exercised: %v", branches)
	t.Logf("max deviation: %g", maxDev)
}

// cardHistory builds card_events' cumulative history: depth rows of two non-decreasing
// counts, newest first, so row r is the total r steps ago. Increments are drawn per row and
// per channel, which is what makes the rows differ — a history that were flat across the
// window would agree with any lag offset at all and so pin nothing.
func cardHistory(rng *rand.Rand, depth int) *simulator.StateHistory {
	rows := mat.NewDense(depth, CardRateWidth, nil)
	total := make([]float64, CardRateWidth)
	for r := depth - 1; r >= 0; r-- {
		for k := range total {
			if rng.IntN(3) == 0 {
				total[k]++
			}
		}
		rows.SetRow(r, total)
	}
	return &simulator.StateHistory{
		Values:            rows,
		StateWidth:        CardRateWidth,
		StateHistoryDepth: depth,
	}
}

// atTime returns a timesteps history at cumulative time now. match_state reads t rather than
// the step number, so the two are decoupled here.
func atTime(now, increment float64, step int) *simulator.CumulativeTimestepsHistory {
	return &simulator.CumulativeTimestepsHistory{
		Values:            mat.NewVecDense(1, []float64{now}),
		NextIncrement:     increment,
		CurrentStepNumber: step,
	}
}

// MatchStateFunction clamps its lookback to historyDepth-1 when a partition keeps fewer rows
// than the ten-minute window. The clamp is unreachable in this stub's wiring — card_events is
// built at YellowCardMinutes+1 rows precisely so the window fits — and it has no declarative
// counterpart, since lag panics rather than silently shortening the read. So the comparison
// below runs at the model's depth only, and the clamp is deliberately not swept.
func TestDeclarativeMatchStateMatchesBespoke(t *testing.T) {
	// Index 1 is card_events: the bespoke function reaches it through the card_partition
	// param and reads Values.At(10, k) by hand, the declarative one through its card_hist
	// upstream alias and lag(card_hist, 10). Both are the same read of the same row. The
	// other three inputs are within-step params_from_upstream on both sides, so they arrive
	// as params either way.
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{
			{Name: "match_state", Seed: 5},
			{Name: "card_events", Seed: 0},
		},
	}
	bespoke := &general.ValuesFunctionIteration{Function: MatchStateFunction}
	declarative, _ := declarativeIteration(t, "match_state")
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	rng := rand.New(rand.NewPCG(91, 92))
	branches := map[string]int{}
	maxDev := 0.0

	const cases = 20000
	for i := 0; i < cases; i++ {
		cards := cardHistory(rng, YellowCardMinutes+1)
		current := cards.Values.RawRowView(0)
		// card_values is card_events' within-step output, so it is this step's total: at or
		// above row 0, which is last step's.
		cardValues := []float64{
			current[0] + float64(rng.IntN(2)),
			current[1] + float64(rng.IntN(2)),
		}
		scoreValues := []float64{
			float64(rng.IntN(6)), float64(rng.IntN(6)),
			float64(rng.IntN(8)), float64(rng.IntN(8)),
		}
		convValues := []float64{
			float64(rng.IntN(int(scoreValues[0]) + 1)),
			float64(rng.IntN(int(scoreValues[1]) + 1)),
		}
		// Time is swept across the half boundary, and lands exactly on it often enough to pin
		// the >= rather than a >. The increment does not enter match_state at all — no output
		// here uses dt — but it is varied anyway so that a twin which had reached for it
		// would show up.
		now := float64(rng.IntN(81))
		if i%7 == 0 {
			now = 40
		}
		increment := []float64{1.0, 0.5, 2.0}[i%3]

		p := simulator.NewParams(map[string][]float64{
			"score_values":      scoreValues,
			"conversion_values": convValues,
			"card_values":       cardValues,
			"card_partition":    {1},
		})
		ts := atTime(now, increment, i+1)
		mk := func() []*simulator.StateHistory {
			return []*simulator.StateHistory{
				history(make([]float64, MatchStateWidth), 1),
				cards,
			}
		}

		want := bespoke.Iterate(&p, 0, mk(), ts)
		wantCopy := append([]float64(nil), want...)
		got := declarative.Iterate(&p, 0, mk(), ts)

		if now >= 40 {
			branches["second_half"]++
		} else {
			branches["first_half"]++
		}
		switch {
		case wantCopy[StateIdxScoreDiff] > 0:
			branches["home_leading"]++
		case wantCopy[StateIdxScoreDiff] < 0:
			branches["away_leading"]++
		default:
			branches["scores_level"]++
		}
		for k, idx := range []int{StateIdxHomeActiveYellow, StateIdxAwayActiveYellow} {
			if wantCopy[idx] > 0 {
				branches["yellow_active"]++
			} else {
				branches["no_active_yellow"]++
			}
			// A card that has left the window: the row ten back is non-zero, so the lag term
			// is load-bearing rather than a subtraction of zero.
			if cards.Values.At(YellowCardMinutes, k) > 0 {
				branches["yellow_aged_out"]++
			}
			// The rows either side of the window differ, so reading one row too few or too
			// many would give a different answer here.
			if cards.Values.At(YellowCardMinutes, k) !=
				cards.Values.At(YellowCardMinutes-1, k) {
				branches["window_edge_pinned"]++
			}
		}
		for k := range wantCopy {
			maxDev = math.Max(maxDev, assertClose(t, got[k], wantCopy[k], "match_state"))
		}
	}
	for _, b := range []string{
		"first_half", "second_half", "home_leading", "away_leading", "scores_level",
		"yellow_active", "no_active_yellow", "yellow_aged_out", "window_edge_pinned",
	} {
		if branches[b] == 0 {
			t.Errorf("branch %q never exercised; the comparison is weaker than it looks", b)
		}
	}
	t.Logf("branches exercised: %v", branches)
	t.Logf("max deviation: %g", maxDev)
}

func TestDeclarativeRugbyAnswersTheSameClaims(t *testing.T) {
	// The oracle is the model's own behaviour suite: every claim recomputed against the
	// declarative build must still hold, and must hold with the same numbers the card
	// reports — not merely point the same way.
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
		// Still a true claim when the model is data.
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
