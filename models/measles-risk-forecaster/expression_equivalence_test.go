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
// # The oracle: exact, throughout
//
// Every partition here is compared exactly, because both models run on the same draw
// stream. rng.New(seed) and rand.New(rand.NewPCG(seed, seed)) are the same generator,
// pkg/rng's Gamma and Poisson are bit-identical to the distuv ones the bespoke code uses,
// and each spec takes the same draws in the same order as its Go counterpart. Agreement is
// therefore asserted to a tight tolerance rather than to bit-identity: compiled Go
// contracts a + b*c into an FMA, which rounds differently from the evaluator's separate
// operations, and that residue is the FMA rather than the model.
//
// The outbreaks branching generations are what makes this interesting. Go walks the areas
// in order and, for each ACTIVE one, takes a Gamma and then a Poisson — a per-area
// interleaved stream that skips inactive areas entirely. Elementwise evaluation cannot say
// that: gamma(shape, rate) would take all 30 Gammas before poisson(...) took any, and a
// vector-conditioned where must evaluate both branches, so masked-out areas would draw too.
// each(30, i, ...) says both halves at once. Lanes run in order and a lane completes before
// the next begins, which is the interleaving; and inside a lane every value is a scalar, so
// the guard is lazy and a skipped area draws nothing. The two models are path-equal, so
// this file asserts equality of values and not of distributions.
//
// Exactness carries into the suite test: the claim ensembles are means over realisations of
// a heavy-tailed branching process, and on a shared stream they are the SAME realisations,
// so the declarative build reproduces the bespoke build's numbers rather than merely its
// directions.

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

func TestDeclarativeOutbreakBranchingMatchesBespoke(t *testing.T) {
	// The branching generations, compared value for value: each puts both sides on one draw
	// stream (see the file comment), so the drawn magnitudes must agree and not merely their
	// distributions. The structural checks are kept alongside — which areas are frozen, the
	// depletion cap, the cumulative accrual — because they hold each side to the model
	// independently, and so still fail if both sides drifted together.
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
	maxDev := 0.0

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
		// The Gamma shape is cases*dispersion, and pkg/rng switches sampler on it: the
		// Liu–Martin–Syring log-space method below 0.2, a plain exponential at exactly 1, and
		// Marsaglia–Tsang otherwise. Sweeping dispersion across all three puts every branch of
		// the sampler on the stream, so a divergence in any of them would surface here.
		dispersion := 0.2 + rng.Float64()
		switch {
		case i%11 == 0:
			dispersion = 0.01 // shape below 0.2 for the small generations above
		case i%13 == 0:
			dispersion = 1.0 // shape of exactly 1 wherever a lane holds a single case
		}
		p := simulator.NewParams(map[string][]float64{
			"susceptibility":      susceptibility,
			"receptivity":         params["receptivity"],
			"susceptible_pool":    pool,
			"r0":                  {r0},
			"dispersion":          {dispersion},
			"national_seed_total": {50.0},
		})
		step := 2 + rng.IntN(12)

		want := bespoke.Iterate(&p, 0, historyOf(state), stepsAt(step))
		got := declarative.Iterate(&p, 0, historyOf(state), stepsAt(step))
		check(want, state, pool, r0, "bespoke", i)
		check(got, state, pool, r0, "declarative", i)
		// The whole point: same stream, so same values, not merely the same distribution.
		for k := range want {
			maxDev = math.Max(maxDev, assertClose(t, got[k], want[k], "branching"))
		}

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
				if want[k] == math.Floor(remaining) {
					branches["cap_bound"]++
				}
				// Which of pkg/rng's three Gamma samplers this lane's shape selects.
				switch shape := math.Floor(state[k]) * dispersion; {
				case shape < 0.2:
					branches["gamma_small_shape"]++
				case shape == 1:
					branches["gamma_unit_shape"]++
				default:
					branches["gamma_marsaglia_tsang"]++
				}
			}
		}
	}
	for _, b := range []string{
		"zero_r_local", "extinct", "depleted", "sub_unit_pool", "branched", "cap_bound",
		"gamma_small_shape", "gamma_unit_shape", "gamma_marsaglia_tsang",
	} {
		if branches[b] == 0 {
			t.Errorf("branch %q never exercised; the comparison is weaker than it looks", b)
		}
	}
	t.Logf("branches exercised: %v", branches)
	t.Logf("max deviation: %g", maxDev)
}

func TestDeclarativeMeaslesAnswersTheSameClaims(t *testing.T) {
	// The oracle is the model's own behaviour suite: every claim recomputed against the
	// declarative build must still hold, AND must hold with the same numbers. The second
	// half is the stronger half. Verify only checks that each claim moves in its stated
	// direction, and direction is cheap — a claim suite cannot tell two mechanisms apart if
	// both respond the same way. Reproducing the observations exactly says the declarative
	// build is running the same model over the same draws, which is what a wrong param,
	// wrong wiring or wrong state layout would break.
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
			want := reference.Observations[k].Value
			if obs.Label != reference.Observations[k].Label {
				t.Fatalf("claim %q observation %d: got label %q, want %q",
					claim.ID, k, obs.Label, reference.Observations[k].Label)
			}
			maxDev = math.Max(maxDev,
				assertClose(t, obs.Value, want, claim.ID+" / "+obs.Label))
		}
	}
	t.Logf("max deviation across every claim observation: %g", maxDev)
}
