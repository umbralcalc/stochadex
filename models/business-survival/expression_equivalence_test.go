package bizsurvival

// Does declarative.yaml — this model written as data, with no Go — reproduce the model
// stub.go builds in code?
//
// This is the entry that had no twin at all until the DSL grew each, slice and concat. The
// register is a flattened sector × age grid whose next row reads the row below it, which is
// exactly what an elementwise evaluator cannot say. So the first question this file answers
// is not "is the twin right" but "is there one", and the branch counts below are the
// evidence: the index shift, the absorbing top bucket and the per-sector blocks are all
// reached, on both sides, and agree.
//
// # Which oracle, and why this one
//
// The strongest available: EXACT, value-by-value, on both the mean-field path and the
// stochastic one. That was not the expected outcome and it is worth saying why it holds,
// because the reasoning that says it should not is nearly right.
//
// SingleLAPopulationIteration has its own poissonSample and binomialSample, which reads like
// antimicrobial-resistance's blocker — a hand-rolled sampler the engine implements
// differently, whose stream cannot be aligned. It is not the same situation. Those two
// methods are thin wrappers: they set Lambda / N / P on a distuv distribution and call its
// Rand, and the guards around them (lambda <= 0, n <= 0, p <= 0, p >= 1) return early
// without drawing. pkg/rng is a deliberate bit-identical reimplementation of distuv's own
// algorithms — see its package doc, which is explicit that a Sampler consumes the source in
// the same order as the distuv distribution it replaces — and both sides seed the same
// generator, since rng.New(seed) is rand.New(rand.NewPCG(seed, seed)) and Configure builds
// exactly that. The DSL's binomial() goes further and calls distuv.Binomial itself.
//
// So the two sides share one stream, and the twin holds it in step by reproducing each early
// return as a scalar guard inside each's lane, where where is lazy and a guarded branch
// draws nothing. That is what makes lambda = 0 and an empty bucket cost zero draws on both
// sides rather than one on one side, and it is the whole reason the streams stay aligned
// past the first guard. TestDeclarativeStochasticMatchesBespokeExactly is what proves it,
// over ten compounding steps per case and across every sampler branch.
//
// Two layers, because they fail in different ways. The step tests compare single Iterate
// calls over randomised inputs, which catches a mis-stated formula exactly. The suite test
// re-runs the model's own claim computations against the declarative build, which catches
// what per-step agreement cannot: wrong wiring, wrong param values, wrong state layout.
//
// Agreement is asserted to a tight tolerance rather than bit-for-bit: compiled Go is free to
// contract a + b*c into a fused multiply-add, which rounds differently from the evaluator's
// separate operations. That residue is the FMA, not the model.
//
// # The one difference that is real, and is not a defect
//
// The Go caches its 60 monthly hazards, and its deterministic flag, in Configure; the twin
// derives both per step from the same params. Every assembly sets those params before
// Configure, so the two agree everywhere either is used — but a mid-run change to
// survival_fracs would be ignored by the Go and honoured by the twin. That is a property of
// the implementation, not of the demography, and the twin states the demography. Recorded,
// not patched: changing the model to make a twin agree is forbidden, and here it is the
// model that is the odd one out.

import (
	"fmt"
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

// stateWidth is the register's flattened size: 6 sectors × 60 monthly age buckets.
const stateWidth = 360

// declarativeBuildStub assembles the model from declarative.yaml, matching BuildStub's
// signature so it can be dropped into the behaviour helpers. The YAML holds the model; the
// run knobs BuildStub takes as arguments are injected here, exactly as BuildStub injects
// them into its Go partition.
func declarativeBuildStub(
	hazardScale float64,
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
	gen.GetPartition("population").Params.Map["policy_death_hazard_scale"] =
		[]float64{hazardScale}
	gen.GetPartition("population").Seed = seed
	return gen
}

// declarativeParams returns the params declarative.yaml declares for the population
// partition, copied so a caller may vary them.
func declarativeParams(t *testing.T) map[string][]float64 {
	t.Helper()
	config := declarativeBuildStub(DefaultPolicyHazardScale, 10, 0).GetPartition("population")
	if _, ok := config.Iteration.(*general.ExpressionIteration); !ok {
		t.Fatalf("population is not expression-backed: got %T", config.Iteration)
	}
	out := map[string][]float64{}
	for k, v := range config.Params.Map {
		out[k] = append([]float64{}, v...)
	}
	return out
}

// declarativeIteration returns the expression iteration the YAML supplies for the population
// partition. Configure is what parses and seeds it, so a fresh one per configuration is not
// needed — the caller reconfigures it.
func declarativeIteration(t *testing.T) *general.ExpressionIteration {
	t.Helper()
	config := declarativeBuildStub(DefaultPolicyHazardScale, 10, 0).GetPartition("population")
	iteration, ok := config.Iteration.(*general.ExpressionIteration)
	if !ok {
		t.Fatalf("population is not expression-backed: got %T", config.Iteration)
	}
	return iteration
}

// settingsFor builds the partition settings both sides configure from. The bespoke reads
// survival_fracs, sector_hazard_scales and deterministic here, in Configure, and checks the
// state width, so the settings must carry the case's params and a full-width init state.
func settingsFor(params map[string][]float64, seed uint64) *simulator.Settings {
	return &simulator.Settings{
		Iterations: []simulator.IterationSettings{{
			Name:              "population",
			Seed:              seed,
			Params:            simulator.NewParams(params),
			StateWidth:        stateWidth,
			InitStateValues:   make([]float64, stateWidth),
			StateHistoryDepth: 1,
		}},
	}
}

// history wraps a single state row as a depth-1 history. Both sides get their own copy, so
// neither can be affected by the other's writes.
func history(values []float64) []*simulator.StateHistory {
	return []*simulator.StateHistory{{
		Values:            mat.NewDense(1, len(values), append([]float64{}, values...)),
		StateWidth:        len(values),
		StateHistoryDepth: 1,
	}}
}

// timesteps builds a one-step timesteps history at cumulative time t with increment dt.
func timesteps(t float64, step int, dt float64) *simulator.CumulativeTimestepsHistory {
	return &simulator.CumulativeTimestepsHistory{
		Values:            mat.NewVecDense(1, []float64{t}),
		NextIncrement:     dt,
		CurrentStepNumber: step + 1,
	}
}

// assertClose fails when two values differ by more than the FMA-scale tolerance, comparing
// relatively once the magnitude exceeds 1. Two NaNs count as agreement: a NaN covariate is a
// case both sides are expected to carry identically, and NaN != NaN would report it as a
// difference.
//
// The context is taken as a format string and its arguments rather than a formatted string,
// so that locating a failure costs nothing on the millions of comparisons that pass.
func assertClose(t *testing.T, got, want float64, format string, args ...any) float64 {
	t.Helper()
	if math.IsNaN(got) && math.IsNaN(want) {
		return 0
	}
	d := math.Abs(got - want)
	if s := math.Abs(want); s > 1 {
		d /= s
	}
	if d > tolerance {
		t.Fatalf("%s: declarative=%v bespoke=%v deviation=%g",
			fmt.Sprintf(format, args...), got, want, d)
	}
	return d
}

// offsetOf mirrors the Go's offset(sec, age) = sec*60 + age, which is the layout the twin's
// each(360, k, ...) lane index has to reproduce.
func offsetOf(sec, age int) int { return sec*NumAges + age }

func TestDeclarativeMeanFieldMatchesBespokeExactly(t *testing.T) {
	// The mean-field oracle. With deterministic set, births = lambda and moved = prev*pSurv
	// and neither side draws at all, so the streams stop mattering and the whole cohort
	// structure is decidable value by value: the index shift, the absorbing top bucket, the
	// hazard lookup, the survival-curve clamps, the covariate indexing, the economic
	// multipliers and every policy fallback. This is where nearly all the risk lives.
	bespoke := &SingleLAPopulationIteration{}
	declarative := declarativeIteration(t)
	base := declarativeParams(t)

	rng := rand.New(rand.NewPCG(41, 42))
	branches := map[string]int{}
	maxDev := 0.0

	const cases = 6000
	for i := 0; i < cases; i++ {
		params := map[string][]float64{}
		for k, v := range base {
			params[k] = append([]float64{}, v...)
		}
		params["deterministic"] = []float64{1.0}

		// The survival curve, ranged well outside the physical (0,1] so the year-ratio clamps
		// fire often rather than never: a non-monotone curve gives a ratio above 1, a negative
		// one gives a ratio at or below 0. Kept away from exactly zero so no ratio is 0/0.
		surv := make([]float64, 5)
		for k := range surv {
			switch i % 7 {
			case 0: // rising: ratio > 1
				surv[k] = 0.3 + float64(k)*0.2
			case 1: // negative entries: ratio <= 0
				surv[k] = -0.05 - rng.Float64()*0.2
			case 2: // flat: ratio == 1, so the hazard is exactly 0
				surv[k] = 0.8
			default:
				surv[k] = 0.95 - float64(k)*0.15 - rng.Float64()*0.02
			}
		}
		params["survival_fracs"] = surv

		// Sector hazard scales large enough that the table clamp at 1 bites, and sometimes
		// zero so the hazard vanishes entirely.
		scales := make([]float64, numSectors)
		for k := range scales {
			if i%11 == 0 {
				scales[k] = 0
			} else {
				scales[k] = rng.Float64() * 3
			}
		}
		params["sector_hazard_scales"] = scales

		births := make([]float64, numSectors)
		for k := range births {
			births[k] = rng.Float64() * 8
		}
		params["base_birth_rates"] = births

		// Covariate series of varying length, with the claimant and GDP series often shorter
		// than the rate series so each one's own end-of-series clamp is exercised separately.
		nRates := 1 + rng.IntN(5)
		rates := make([]float64, nRates)
		for k := range rates {
			rates[k] = rng.Float64() * 4
		}
		if i%13 == 0 {
			rates[nRates-1] = math.NaN() // NaN rate: both multipliers must fall back to 1
		}
		params["covariate_bank_rates"] = rates

		claimants := make([]float64, 1+rng.IntN(3))
		for k := range claimants {
			claimants[k] = 8000 + rng.Float64()*20000
		}
		if i%17 == 0 {
			claimants[0] = -5000 // log of a negative: birthMult is NaN and must fall back
		}
		params["covariate_claimants"] = claimants

		gdp := make([]float64, 1+rng.IntN(3))
		for k := range gdp {
			gdp[k] = rng.Float64()*6 - 3
		}
		params["covariate_gdp_growth"] = gdp
		params["gdp_ref"] = []float64{rng.Float64()*2 - 1}
		params["birth_elasticity_gdp"] = []float64{0}
		if i%3 == 0 {
			params["birth_elasticity_gdp"] = []float64{rng.Float64()*0.4 - 0.2}
		}

		distress := make([]float64, 1+rng.IntN(3))
		for k := range distress {
			distress[k] = rng.Float64()*0.8 - 0.4 // straddles zero: active vs inactive
		}
		params["distress_hazard_boost"] = distress

		params["rate_ref"] = []float64{rng.Float64() * 2}
		params["claimant_ref"] = []float64{8000 + rng.Float64()*10000}
		params["birth_elasticity_rate"] = []float64{rng.Float64()*0.6 - 0.3}
		params["birth_elasticity_claimant"] = []float64{rng.Float64()*0.6 - 0.3}
		params["death_elasticity_rate"] = []float64{rng.Float64()*0.6 - 0.3}

		// Policy scalars ranged below zero, which is the Go's "invalid, use the default"
		// branch — a range a real scenario would never visit, and so the only way to reach it.
		policyScalar := func(key string, hi float64) {
			v := rng.Float64() * hi
			if rng.IntN(6) == 0 {
				v = -rng.Float64()
			}
			params[key] = []float64{v}
		}
		policyScalar("policy_birth_scale", 2)
		policyScalar("policy_death_hazard_scale", 3)
		policyScalar("policy_infant_hazard_scale", 2)
		if i%11 == 5 {
			// Drive the effective hazard past 1 outright rather than waiting for a random draw
			// to do it. The Go clamps twice — once building the hazard table, once after the
			// multipliers — and the outer clamp is only reachable from a range like this.
			params["policy_death_hazard_scale"] = []float64{50}
		}

		// Per-sector multipliers use a different fallback threshold in the Go (non-positive,
		// not negative), so zero must appear as well as negatives.
		perSector := func(key string) {
			v := make([]float64, numSectors)
			for k := range v {
				switch rng.IntN(8) {
				case 0:
					v[k] = 0
				case 1:
					v[k] = -rng.Float64()
				default:
					v[k] = rng.Float64() * 2
				}
			}
			params[key] = v
		}
		perSector("policy_sector_birth_scale")
		perSector("policy_sector_hazard_scale")

		// Cumulative time below zero, inside the rate series, and past its end, so all three
		// covariate-index branches are reached.
		var tv float64
		switch i % 4 {
		case 0:
			tv = -1 - rng.Float64()*3
		case 1:
			tv = rng.Float64() * float64(nRates)
		default:
			tv = float64(nRates) + rng.Float64()*6
		}
		// dt is varied below, at and above 1 even though neither side should read it: at the
		// stepsize of 1 every assembly actually runs at, a stray dt or sqrt(dt) factor in the
		// twin would be invisible.
		dt := []float64{0.5, 1.0, 2.0, 3.5}[i%4]

		state := make([]float64, stateWidth)
		for k := range state {
			state[k] = rng.Float64() * 500
		}

		settings := settingsFor(params, uint64(i%97))
		bespoke.Configure(0, settings)
		declarative.Configure(0, settings)

		p := simulator.NewParams(params)
		ts := timesteps(tv, i, dt)
		want := append([]float64{}, bespoke.Iterate(&p, 0, history(state), ts)...)
		got := append([]float64{}, declarative.Iterate(&p, 0, history(state), ts)...)

		// ---- branch accounting, derived from the inputs rather than declared ----

		switch i % 7 {
		case 0:
			branches["year_ratio_clamped_above_one"]++
		case 1:
			branches["year_ratio_clamped_at_zero"]++
		case 2:
			branches["year_hazard_exactly_zero"]++
		default:
			branches["year_ratio_valid"]++
		}
		if tv < 0 {
			branches["covariate_index_clamped_low"]++
		} else if int(tv) >= nRates {
			branches["covariate_index_clamped_high"]++
		} else {
			branches["covariate_index_inside_series"]++
		}
		if len(claimants) < nRates {
			branches["claimant_series_shorter_than_rates"]++
		}
		if params["birth_elasticity_gdp"][0] != 0 {
			branches["gdp_channel_active"]++
		} else {
			branches["gdp_channel_neutral"]++
		}
		tIdx := int(math.Max(0, math.Min(float64(nRates-1), math.Floor(tv))))
		if b := distress[int(math.Min(float64(tIdx), float64(len(distress)-1)))]; b > 0 {
			branches["distress_boost_active"]++
		} else {
			branches["distress_boost_inactive"]++
		}
		if math.IsNaN(pickSeries(rates, tIdx)) {
			branches["death_mult_nan_fallback"]++
		}
		if pickSeries(claimants, tIdx) < 0 {
			branches["birth_mult_nan_fallback"]++
		}
		for _, key := range []string{
			"policy_birth_scale", "policy_death_hazard_scale", "policy_infant_hazard_scale",
		} {
			if params[key][0] < 0 {
				branches[key+"_fallback"]++
			} else {
				branches[key+"_used"]++
			}
		}
		for _, key := range []string{"policy_sector_birth_scale", "policy_sector_hazard_scale"} {
			for _, v := range params[key] {
				if v <= 0 {
					branches[key+"_fallback"]++
				} else {
					branches[key+"_used"]++
				}
			}
		}
		if params["policy_infant_hazard_scale"][0] != 1 {
			branches["infant_relief_differs_from_baseline"]++
		}
		// The structural lanes: every case evaluates all 360 of them, and the top bucket is
		// genuinely absorbing whenever both it and the bucket below hold businesses.
		branches["lane_age_0_formation"]++
		branches["lane_age_1_infant_hazard"]++
		branches["lane_age_2_to_58_shift"]++
		branches["lane_age_59_absorbing"]++
		for sec := 0; sec < numSectors; sec++ {
			if state[offsetOf(sec, 58)] > 0 && state[offsetOf(sec, 59)] > 0 {
				branches["top_bucket_takes_inflow_and_survivors"]++
			}
		}
		// The two clamps on the effective hazard, read off the outputs: a lane whose survivors
		// are exactly zero had its hazard clamped to 1, and one that keeps its whole bucket
		// had a hazard of exactly 0.
		for sec := 0; sec < numSectors; sec++ {
			k := offsetOf(sec, 30)
			if want[k] == 0 && state[k-1] > 0 {
				branches["effective_hazard_clamped_at_one"]++
			}
			if want[k] == state[k-1] && state[k-1] > 0 {
				branches["effective_hazard_exactly_zero"]++
			}
		}

		for k := range want {
			maxDev = math.Max(maxDev, assertClose(t, got[k], want[k],
				"case %d, bucket %d (sector %d, age %d), t=%v, dt=%v",
				i, k, k/NumAges, k%NumAges, tv, dt))
		}
	}

	for _, b := range []string{
		"year_ratio_valid", "year_ratio_clamped_above_one", "year_ratio_clamped_at_zero",
		"year_hazard_exactly_zero",
		"covariate_index_clamped_low", "covariate_index_clamped_high",
		"covariate_index_inside_series", "claimant_series_shorter_than_rates",
		"gdp_channel_active", "gdp_channel_neutral",
		"distress_boost_active", "distress_boost_inactive",
		"death_mult_nan_fallback", "birth_mult_nan_fallback",
		"policy_birth_scale_fallback", "policy_birth_scale_used",
		"policy_death_hazard_scale_fallback", "policy_death_hazard_scale_used",
		"policy_infant_hazard_scale_fallback", "policy_infant_hazard_scale_used",
		"policy_sector_birth_scale_fallback", "policy_sector_birth_scale_used",
		"policy_sector_hazard_scale_fallback", "policy_sector_hazard_scale_used",
		"infant_relief_differs_from_baseline",
		"lane_age_0_formation", "lane_age_1_infant_hazard", "lane_age_2_to_58_shift",
		"lane_age_59_absorbing", "top_bucket_takes_inflow_and_survivors",
		"effective_hazard_clamped_at_one", "effective_hazard_exactly_zero",
	} {
		if branches[b] == 0 {
			t.Errorf("branch %q never exercised; the comparison is weaker than it looks", b)
		}
	}
	t.Logf("branches exercised: %v", branches)
	t.Logf("max deviation: %g", maxDev)
}

func TestDeclarativeStochasticMatchesBespokeExactly(t *testing.T) {
	// The stochastic path, on the same stream. Every sampler branch has to line up for this
	// to hold — a guard that draws on one side and returns early on the other desynchronises
	// the stream and every later lane diverges, which is why this runs ten compounding steps
	// per case rather than one.
	bespoke := &SingleLAPopulationIteration{}
	declarative := declarativeIteration(t)
	base := declarativeParams(t)

	rng := rand.New(rand.NewPCG(51, 52))
	branches := map[string]int{}
	maxDev := 0.0

	const (
		cases = 400
		steps = 10
	)
	for i := 0; i < cases; i++ {
		params := map[string][]float64{}
		for k, v := range base {
			params[k] = append([]float64{}, v...)
		}
		params["deterministic"] = []float64{0.0}

		// Formation rates swept across rng.Poisson's own lambda = 10 switch from the direct
		// exponential-interarrival method to Hoermann's PTRS, and down to zero, which is
		// poissonSample's early return: it draws nothing there, and the twin must not either.
		lambdas := make([]float64, numSectors)
		for k := range lambdas {
			switch (i + k) % 3 {
			case 0:
				lambdas[k] = rng.Float64() * 9
			case 1:
				lambdas[k] = 10 + rng.Float64()*90
			default:
				lambdas[k] = 0
			}
		}
		params["base_birth_rates"] = lambdas

		// The survival probability is driven to both certainties as well as the interior,
		// because those are binomialSample's other two early returns.
		var regime string
		switch i % 4 {
		case 0:
			regime = "certain_survival" // zero hazard: p >= 1, return n, draw nothing
			params["sector_hazard_scales"] = make([]float64, numSectors)
		case 1:
			regime = "certain_death" // hazard clamped to 1: p <= 0, return 0, draw nothing
			params["policy_death_hazard_scale"] = []float64{500}
			scales := make([]float64, numSectors)
			for k := range scales {
				scales[k] = 50
			}
			params["sector_hazard_scales"] = scales
		default:
			regime = "interior"
			scales := make([]float64, numSectors)
			for k := range scales {
				scales[k] = 0.5 + rng.Float64()*1.5
			}
			params["sector_hazard_scales"] = scales
			params["policy_death_hazard_scale"] = []float64{rng.Float64() * 2}
		}
		params["policy_infant_hazard_scale"] = []float64{rng.Float64() * 2}

		// Half the buckets start empty, which is binomialSample's n <= 0 early return.
		state := make([]float64, stateWidth)
		for k := range state {
			if rng.IntN(2) == 0 {
				state[k] = 0
			} else {
				state[k] = math.Floor(rng.Float64() * 300)
			}
		}

		settings := settingsFor(params, uint64(1000+i))
		bespoke.Configure(0, settings)
		declarative.Configure(0, settings)

		for _, lambda := range lambdas {
			switch {
			case lambda <= 0:
				branches["poisson_zero_lambda_no_draw"]++
			case lambda < 10:
				branches["poisson_direct_method"]++
			default:
				branches["poisson_ptrs"]++
			}
		}
		branches["binomial_"+regime]++
		for _, v := range state {
			if v == 0 {
				branches["binomial_empty_bucket_no_draw"]++
			}
		}

		p := simulator.NewParams(params)
		wantState := append([]float64{}, state...)
		gotState := append([]float64{}, state...)
		for step := 0; step < steps; step++ {
			ts := timesteps(float64(step), step, []float64{0.5, 1.0, 2.0}[step%3])
			want := append([]float64{},
				bespoke.Iterate(&p, 0, history(wantState), ts)...)
			got := append([]float64{},
				declarative.Iterate(&p, 0, history(gotState), ts)...)
			for k := range want {
				maxDev = math.Max(maxDev, assertClose(t, got[k], want[k],
					"case %d, step %d, bucket %d (sector %d, age %d)",
					i, step, k, k/NumAges, k%NumAges))
			}
			// Advance each side on its OWN output, so a divergence cannot be masked by
			// feeding both the same state back in.
			wantState, gotState = want, got
		}
	}

	for _, b := range []string{
		"poisson_zero_lambda_no_draw", "poisson_direct_method", "poisson_ptrs",
		"binomial_interior", "binomial_certain_survival", "binomial_certain_death",
		"binomial_empty_bucket_no_draw",
	} {
		if branches[b] == 0 {
			t.Errorf("branch %q never exercised; the comparison is weaker than it looks", b)
		}
	}
	t.Logf("branches exercised: %v", branches)
	t.Logf("max deviation over %d cases × %d compounding steps: %g", cases, steps, maxDev)
}

func TestDeclarativeBusinessSurvivalAnswersTheSameClaims(t *testing.T) {
	// The oracle is the model's own behaviour suite: every claim recomputed against the
	// declarative build must still hold, and must hold with the same numbers the card
	// reports — not merely point the same way.
	//
	// Asserting the numbers is available here, and so is required: every claim in this suite
	// runs in deterministic mean-field mode, where neither side takes a draw, so the two
	// builds are comparable value-by-value over the full 120-step run and a tolerance wide
	// enough to hide a real difference would be a choice, not a necessity.
	if testing.Short() {
		t.Skip("runs the full claim suite twice")
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
				reference.Observations[k].Value, "%s / %s", claim.ID, obs.Label))
		}
	}
	t.Logf("max deviation across every observation on every claim: %g", maxDev)
}
