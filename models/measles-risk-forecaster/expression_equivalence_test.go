package measles

// Does declarative.yaml — this model written as data, with no Go — reproduce the model
// stub.go builds in code?
//
// Two independent checks, because they fail in different ways. The step-for-step tests
// compare single Iterate calls over randomised inputs, which catches a mis-stated formula
// exactly. The suite test re-runs the model's own claim computations against the
// declarative build, which catches a model that is subtly different in a way per-step
// agreement would not: wrong wiring, wrong param values, wrong state layout.
//
// # Where the two streams part company, and why
//
// national_importation and the outbreaks seeding generation are exactly comparable:
// rng.New(seed) and rand.New(rand.NewPCG(seed, seed)) are the same generator, pkg/rng's
// draws are bit-identical to the distuv ones the bespoke code uses, and the declarative
// spec takes the same draws in the same order, so the two stay in lockstep and agreement
// is asserted to a tight tolerance.
//
// The outbreaks BRANCHING generations cannot be aligned, and this is a property of the
// DSL, not of how the spec is written. JointOutbreakIteration walks the areas in order and,
// for each ACTIVE one, takes a Gamma then a Poisson — so its stream is per-area
// interleaved, and skips inactive areas entirely. An expression evaluates elementwise over
// the whole vector: gamma(shape, rate) takes all 30 Gammas before poisson(...) takes any
// Poisson, and a vector-conditioned where must evaluate both branches, so the masked-out
// areas draw too. Neither the interleaving nor the skipping is expressible without a loop
// construct the DSL deliberately does not have. The two models are therefore equal in
// distribution but not path-equal, and the branching comparison below is distributional:
// the deterministic structure (which areas are frozen, the depletion cap, the cumulative
// accrual) is still checked exactly, per replicate, and only the drawn magnitudes are
// compared through their sampling distribution.
//
// That parting is inherited by the suite test: the claim ensembles are means over 12–50
// realisations of a heavy-tailed branching process, so the declarative build answers each
// claim correctly but not with the bespoke build's exact numbers. The suite test therefore
// asserts what is true — every claim still verifies — and reports the ensemble spread
// rather than pretending to a tolerance the streams cannot support.

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
// run knobs BuildStub takes as arguments are injected here, exactly as BuildStub injects
// them into its Go partitions — including the per-UTLA surface, which BuildStub likewise
// derives from its coverage argument rather than storing.
func declarativeBuildStub(
	mmr2Coverage float64,
	maxGenerations int,
	seed uint64,
) *simulator.ConfigGenerator {
	config := api.LoadApiRunConfigFromYaml("declarative.yaml")
	config.Main.Simulation = simulator.SimulationConfig{
		OutputCondition: &simulator.EveryStepOutputCondition{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: maxGenerations,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:    0.0,
	}
	gen := config.Main.GetConfigGenerator()
	susceptibility, receptivity, pool := BuildUTLASurface(mmr2Coverage)
	outbreaks := gen.GetPartition("outbreaks")
	outbreaks.Params.Map["susceptibility"] = susceptibility
	outbreaks.Params.Map["receptivity"] = receptivity
	outbreaks.Params.Map["susceptible_pool"] = pool
	outbreaks.Seed = seed + 7919
	gen.GetPartition("national_importation").Seed = seed
	return gen
}

// declarativeIteration returns the expression iteration the YAML supplies for a partition,
// alongside the params it declares.
func declarativeIteration(
	t *testing.T,
	partition string,
) (*general.ExpressionIteration, map[string][]float64) {
	t.Helper()
	config := declarativeBuildStub(DefaultMMR2Coverage, 10, 0).GetPartition(partition)
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

// stepsAt builds a timesteps history reporting the given step number, which is the only
// part of it either iteration reads.
func stepsAt(step int) *simulator.CumulativeTimestepsHistory {
	return &simulator.CumulativeTimestepsHistory{
		Values:            mat.NewVecDense(1, []float64{float64(step)}),
		NextIncrement:     1.0,
		CurrentStepNumber: step,
	}
}

// historyOf wraps one state row as a single-partition history, fresh each call so neither
// iteration can be handed the other's buffer.
func historyOf(state []float64) []*simulator.StateHistory {
	return []*simulator.StateHistory{{
		Values:            mat.NewDense(1, len(state), append([]float64{}, state...)),
		StateWidth:        len(state),
		StateHistoryDepth: 1,
	}}
}

func TestDeclarativeNationalImportationMatchesBespoke(t *testing.T) {
	bespoke := &NationalImportationIteration{}
	declarative, _ := declarativeIteration(t, "national_importation")
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{{Name: "national_importation", Seed: 5}},
	}
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	rng := rand.New(rand.NewPCG(31, 32))
	branches := map[string]int{}
	maxDev := 0.0

	const cases = 20000
	for i := 0; i < cases; i++ {
		// Sweep the band floor across zero so the log-guard fires, and the step number
		// across 1 so both the draw and the hold are compared. The hold must take no draw
		// on either side, or every later case would disagree — which is exactly what makes
		// this a test of the lazy where and not just of the log-uniform formula.
		low := 20.0 + rng.Float64()*100
		if i%7 == 0 {
			low = -rng.Float64()
		}
		high := 150.0 + rng.Float64()*200
		step := 1
		if i%2 == 0 {
			step = 2 + rng.IntN(20)
		}
		p := simulator.NewParams(map[string][]float64{
			"seed_low":  {low},
			"seed_high": {high},
		})
		state := []float64{rng.Float64() * 200}

		want := bespoke.Iterate(&p, 0, historyOf(state), stepsAt(step))
		got := declarative.Iterate(&p, 0, historyOf(state), stepsAt(step))

		if step > 1 {
			branches["held"]++
		} else {
			branches["drawn"]++
		}
		if low > 0 {
			branches["positive_band_floor"]++
		} else {
			branches["clamped_band_floor"]++
		}
		maxDev = math.Max(maxDev, assertClose(t, got[0], want[0], "national seed total"))
	}
	for _, b := range []string{"held", "drawn", "positive_band_floor", "clamped_band_floor"} {
		if branches[b] == 0 {
			t.Errorf("branch %q never exercised; the comparison is weaker than it looks", b)
		}
	}
	t.Logf("branches exercised: %v", branches)
	t.Logf("max deviation: %g", maxDev)
}

func TestDeclarativeOutbreakSeedingMatchesBespoke(t *testing.T) {
	// Generation 0 is the one outbreaks step whose draws do align: one Poisson per area, in
	// area order, on both sides.
	bespoke := &JointOutbreakIteration{}
	declarative, params := declarativeIteration(t, "outbreaks")
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{{Name: "outbreaks", Seed: 13}},
	}
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	n := len(params["susceptibility"])
	rng := rand.New(rand.NewPCG(41, 42))
	branches := map[string]int{}
	maxDev := 0.0

	const cases = 2000
	for i := 0; i < cases; i++ {
		// The band spans both of pkg/rng's Poisson algorithms — the direct
		// exponential-interarrival method below lambda 10 and PTRS above it — so a
		// disagreement in either branch of the sampler would show up here too.
		total := 1.0 + rng.Float64()*600
		p := simulator.NewParams(map[string][]float64{
			"susceptibility":      params["susceptibility"],
			"receptivity":         params["receptivity"],
			"susceptible_pool":    params["susceptible_pool"],
			"r0":                  {DefaultR0},
			"dispersion":          {DefaultDispersion},
			"national_seed_total": {total},
		})
		state := make([]float64, 2*n)

		want := bespoke.Iterate(&p, 0, historyOf(state), stepsAt(1))
		got := declarative.Iterate(&p, 0, historyOf(state), stepsAt(1))

		for k := 0; k < n; k++ {
			lambda := total * params["receptivity"][k]
			if lambda < 10 {
				branches["small_lambda_lane"]++
			} else {
				branches["large_lambda_lane"]++
			}
			if want[k] == 0 {
				branches["zero_seed_lane"]++
			} else {
				branches["seeded_lane"]++
			}
		}
		for k := range want {
			maxDev = math.Max(maxDev, assertClose(t, got[k], want[k], "seeding"))
		}
		// Generation 0 must put the seed count into both blocks from a single draw; a spec
		// that drew twice would pass the elementwise check above and fail this.
		for k := 0; k < n; k++ {
			if got[k] != got[n+k] {
				t.Fatalf("case %d area %d: cumulative %v does not equal infectious %v",
					i, k, got[n+k], got[k])
			}
		}
	}
	for _, b := range []string{
		"small_lambda_lane", "large_lambda_lane", "zero_seed_lane", "seeded_lane",
	} {
		if branches[b] == 0 {
			t.Errorf("branch %q never exercised; the comparison is weaker than it looks", b)
		}
	}
	t.Logf("branches exercised: %v", branches)
	t.Logf("max deviation: %g", maxDev)
}

func TestDeclarativeOutbreakBranchingStructureMatchesBespoke(t *testing.T) {
	// The branching generations' draw streams cannot be aligned (see the file comment), so
	// this checks everything about them that is NOT a drawn magnitude: which areas are
	// frozen, the depletion cap, and the cumulative accrual. Those are the parts of the
	// spec a mis-statement would break, and they are exactly checkable per replicate.
	bespoke := &JointOutbreakIteration{}
	declarative, params := declarativeIteration(t, "outbreaks")
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{{Name: "outbreaks", Seed: 17}},
	}
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	susceptibility := params["susceptibility"]
	n := len(susceptibility)
	rng := rand.New(rand.NewPCG(51, 52))
	branches := map[string]int{}

	check := func(out, state, pool []float64, r0 float64, side string, i int) {
		for k := 0; k < n; k++ {
			infectious := state[k]
			cumulative := state[n+k]
			remaining := pool[k] - cumulative
			frozen := infectious <= 0 || remaining <= 0 || pool[k] < 1 ||
				r0*susceptibility[k]*(remaining/math.Max(pool[k], 1)) <= 0
			if frozen {
				if out[k] != 0 || out[n+k] != cumulative {
					t.Fatalf("%s case %d area %d: frozen area moved: %v, %v -> %v, %v",
						side, i, k, infectious, cumulative, out[k], out[n+k])
				}
				continue
			}
			if out[k] > math.Floor(remaining) {
				t.Fatalf("%s case %d area %d: %v new cases exceeds the %v remaining",
					side, i, k, out[k], remaining)
			}
			if out[n+k] != cumulative+out[k] {
				t.Fatalf("%s case %d area %d: cumulative %v is not %v + %v",
					side, i, k, out[n+k], cumulative, out[k])
			}
		}
	}

	const cases = 4000
	for i := 0; i < cases; i++ {
		pool := make([]float64, n)
		state := make([]float64, 2*n)
		for k := 0; k < n; k++ {
			switch k % 5 {
			case 0: // extinct: no infectious cases left
				state[k] = 0
				pool[k] = 400
				state[n+k] = math.Floor(rng.Float64() * 50)
			case 1: // depleted: the whole reachable pool has already been infected
				state[k] = math.Floor(1 + rng.Float64()*5)
				pool[k] = 400
				state[n+k] = 400 + math.Floor(rng.Float64()*10)
			case 2: // sub-unit pool: too few susceptibles to reach anyone
				state[k] = math.Floor(1 + rng.Float64()*5)
				pool[k] = rng.Float64() * 0.9
				state[n+k] = 0
			case 3: // a small pool branching hard, so the cap on new cases binds
				pool[k] = 5 + math.Floor(rng.Float64()*3)
				state[k] = 20 + math.Floor(rng.Float64()*20)
				state[n+k] = math.Floor(rng.Float64() * 2)
			default: // freely branching
				pool[k] = 400
				state[k] = math.Floor(1 + rng.Float64()*8)
				state[n+k] = math.Floor(rng.Float64() * 100)
			}
		}
		// Sweep R0 through zero so the zero-R_local guard inside the Go kernel — reachable
		// only through a degenerate scenario — is compared too.
		r0 := 4.0 + rng.Float64()*14
		if i%9 == 0 {
			r0 = 0
		}
		p := simulator.NewParams(map[string][]float64{
			"susceptibility":      susceptibility,
			"receptivity":         params["receptivity"],
			"susceptible_pool":    pool,
			"r0":                  {r0},
			"dispersion":          {0.2 + rng.Float64()},
			"national_seed_total": {50.0},
		})
		step := 2 + rng.IntN(12)

		want := bespoke.Iterate(&p, 0, historyOf(state), stepsAt(step))
		got := declarative.Iterate(&p, 0, historyOf(state), stepsAt(step))
		check(want, state, pool, r0, "bespoke", i)
		check(got, state, pool, r0, "declarative", i)

		// Counted in the order the Go guards test them, so each area lands in the branch
		// that actually decided its update.
		for k := 0; k < n; k++ {
			remaining := pool[k] - state[n+k]
			switch {
			case state[k] <= 0:
				branches["extinct"]++
			case remaining <= 0:
				branches["depleted"]++
			case pool[k] < 1:
				branches["sub_unit_pool"]++
			case r0*susceptibility[k]*(remaining/pool[k]) <= 0:
				branches["zero_r_local"]++
			default:
				branches["branched"]++
				// Counted per side: the two streams differ, so an area whose cap binds on
				// one need not have binding on the other.
				if want[k] == math.Floor(remaining) {
					branches["cap_bound_bespoke"]++
				}
				if got[k] == math.Floor(remaining) {
					branches["cap_bound_declarative"]++
				}
			}
		}
	}
	for _, b := range []string{
		"zero_r_local", "extinct", "depleted", "sub_unit_pool", "branched",
		"cap_bound_bespoke", "cap_bound_declarative",
	} {
		if branches[b] == 0 {
			t.Errorf("branch %q never exercised; the comparison is weaker than it looks", b)
		}
	}
	t.Logf("branches exercised: %v", branches)
}

func TestDeclarativeOutbreakBranchingMatchesInDistribution(t *testing.T) {
	// The magnitudes the structural test above leaves alone, checked the only way the
	// unalignable streams allow: both sides' per-area sample means over a common scenario
	// must sit on the analytic mean of the branching kernel, E[next_i] = I_i * R_local_i,
	// and on each other.
	bespoke := &JointOutbreakIteration{}
	declarative, params := declarativeIteration(t, "outbreaks")
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{{Name: "outbreaks", Seed: 23}},
	}
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	susceptibility := params["susceptibility"]
	pool := params["susceptible_pool"]
	n := len(susceptibility)
	const r0, dispersion = 15.0, DefaultDispersion

	// A scenario chosen so the depletion cap never binds — with an empty pool of 400 and at
	// most 3 infectious at R_local well under 3, hitting 400 in one generation has no
	// meaningful probability — because the cap would bias the mean away from the analytic
	// value this compares against.
	state := make([]float64, 2*n)
	for k := 0; k < n; k++ {
		state[k] = float64(1 + k%3)
	}
	p := simulator.NewParams(map[string][]float64{
		"susceptibility":      susceptibility,
		"receptivity":         params["receptivity"],
		"susceptible_pool":    pool,
		"r0":                  {r0},
		"dispersion":          {dispersion},
		"national_seed_total": {50.0},
	})

	const replicates = 20000
	sums := [2][]float64{make([]float64, n), make([]float64, n)}
	squares := [2][]float64{make([]float64, n), make([]float64, n)}
	for i := 0; i < replicates; i++ {
		for side, out := range [2][]float64{
			bespoke.Iterate(&p, 0, historyOf(state), stepsAt(2)),
			declarative.Iterate(&p, 0, historyOf(state), stepsAt(2)),
		} {
			for k := 0; k < n; k++ {
				sums[side][k] += out[k]
				squares[side][k] += out[k] * out[k]
			}
		}
	}

	worst := 0.0
	for k := 0; k < n; k++ {
		rLocal := r0 * susceptibility[k] * ((pool[k] - state[n+k]) / pool[k])
		analytic := state[k] * rLocal
		var mean, se [2]float64
		for side := 0; side < 2; side++ {
			mean[side] = sums[side][k] / replicates
			variance := squares[side][k]/replicates - mean[side]*mean[side]
			se[side] = math.Sqrt(variance / replicates)
			// Four standard errors: a false failure needs a ~1-in-16,000 deviation, and 60
			// comparisons run here.
			if z := math.Abs(mean[side]-analytic) / se[side]; z > 4 {
				t.Errorf("area %d %s: mean %v is %.1f standard errors from the analytic %v",
					k, [2]string{"bespoke", "declarative"}[side], mean[side], z, analytic)
			}
		}
		z := math.Abs(mean[0]-mean[1]) / math.Sqrt(se[0]*se[0]+se[1]*se[1])
		if z > 4 {
			t.Errorf("area %d: declarative mean %v and bespoke mean %v differ by %.1f "+
				"standard errors", k, mean[1], mean[0], z)
		}
		worst = math.Max(worst, z)
	}
	t.Logf("largest declarative-vs-bespoke separation across the 30 areas: %.2f "+
		"standard errors", worst)
}

func TestDeclarativeMeaslesAnswersTheSameClaims(t *testing.T) {
	// The oracle is the model's own behaviour suite: every claim recomputed against the
	// declarative build must still hold. It cannot hold with the SAME numbers — the
	// branching streams do not align (see the file comment), so each claim's ensemble is a
	// different set of realisations of the same distribution — so the numbers are reported
	// rather than asserted on, and what is asserted is what the claims actually say.
	if testing.Short() {
		t.Skip("runs the full claim ensemble twice")
	}
	bespoke := ObservedBehaviour()
	declarative := observedBehaviour(declarativeBuildStub)

	if len(declarative) != len(bespoke) {
		t.Fatalf("got %d claims, want %d", len(declarative), len(bespoke))
	}
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
			want := reference.Observations[k].Value
			relative := math.Abs(obs.Value-want) / math.Max(math.Abs(want), 1e-12)
			t.Logf("%s / %s: declarative=%.4g bespoke=%.4g relative=%.1f%%",
				claim.ID, obs.Label, obs.Value, want, 100*relative)
			// A stochastic band, not an equivalence: both ensembles are fixed-seed and so
			// this is deterministic, and it is set well above the ensemble noise between
			// two independent realisations (the worst seen is ~21%) but far below what a
			// wrong param, wrong wiring or wrong state layout does to these totals, which
			// is multiples rather than percentages.
			if relative > 0.5 {
				t.Errorf("%s / %s: declarative=%v and bespoke=%v differ by %.0f%%, "+
					"more than ensemble noise explains",
					claim.ID, obs.Label, obs.Value, want, 100*relative)
			}
		}
	}
}
