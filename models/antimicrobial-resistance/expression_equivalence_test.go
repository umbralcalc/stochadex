package amr

// Does declarative.yaml — this model written as data, with no Go — reproduce the model
// stub.go builds in code?
//
// # Why this model gets a weaker oracle than anglersim's twin
//
// anglersim's declarative twin is checked step-for-step to ~1e-16, because its bespoke
// iterations happen to draw from math/rand/v2's rand.New(rand.NewPCG(seed, seed)) — exactly
// the generator the expression evaluator builds via rng.New(seed). Same stream, same draw
// order, so equivalence is decidable value-by-value. That was a bonus of a coincidence, not
// the standard.
//
// This model cannot have that, for two independent reasons:
//
//  1. colonisation.go and infection.go sample from math/rand v1
//     (rand.New(rand.NewSource(int64(seed)))), a different generator family from pkg/rng's
//     v2 PCG. The streams diverge from the first draw and cannot be aligned: rng.NewFromSource
//     takes a math/rand/v2 Source, a v1 Source is not one, and ExpressionIteration seeds its
//     sampler internally from the partition seed with no injection point regardless.
//  2. infection.go's poissonSample is a hand-rolled sampler — Knuth's product-of-uniforms
//     loop below lambda=30, a rounded normal approximation above it — where the evaluator's
//     poisson() calls the engine's rng.Poisson (exponential-interarrival below lambda=10,
//     Hoermann PTRS above). Different algorithms consuming different numbers of draws. The
//     Knuth loop is not expressible in the DSL in any case: it is unbounded, and the DSL has
//     no loops or recursion by design. Above lambda=30 the bespoke sampler is not even
//     exactly Poisson — the normal approximation matches only the first two moments.
//
// So the declarative twin is a deliberately different sampler for the same generative
// process. Neither of those facts is a defect in the DSL, and neither is worth changing the
// model to remove: AMR's iterations are lifted verbatim from the downstream repo, and
// card.md documents the lambda=30 branch as part of the validity regime. (Both are recorded
// as standardisation signals — v1 math/rand where the engine has settled on pkg/rng, and a
// hand-rolled sampler where the engine now ships one — but that is a decision about the
// bespoke code, not about this test.)
//
// # What is checked instead
//
//   - The drift, clamp and renormalisation logic is still checked EXACTLY, by switching the
//     noise off: at noise_scale = 0 both sides multiply their draw by zero, so every
//     deterministic branch is comparable value-by-value despite the streams differing.
//   - The stochastic parts are compared by their moments at fixed inputs, over many samples.
//     Those tolerances are SAMPLING bounds, not rounding bounds — see mcTolerance.
//   - The whole claim suite is re-run against the declarative build and every claim must
//     still hold. It asserts the claims, NOT that the numbers match the bespoke ones: they
//     will not, and a near-equality assertion with a tolerance fat enough to pass would be
//     an assertion about nothing. The catalogue's claims are ensemble-level and
//     threshold-based precisely so they survive a different draw stream while still
//     discriminating a wrong model.

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

// exactTolerance is well above FMA rounding — compiled Go is free to contract a + b*c into a
// fused multiply-add, which rounds differently from the evaluator's separate operations —
// and well below any difference a real modelling discrepancy would produce. It applies only
// where the comparison is genuinely exact, i.e. with the noise switched off.
const exactTolerance = 1e-12

// mcSamples is the sample count behind every moment comparison, and mcSigmas the width of
// the bound in standard errors. Six sigma puts the false-failure rate of each assertion near
// 2e-9, which keeps a distributional test from being a flaky one.
const (
	mcSamples = 200000
	mcSigmas  = 6.0
)

// declarativeBuildStub assembles the model from declarative.yaml, matching BuildStub's
// signature so it can be dropped into the behaviour helpers. The YAML holds the model; the
// run knobs BuildStub takes as arguments are injected here, exactly as BuildStub injects
// them into its Go partitions. Seeds come from the YAML and match stub.go's, though the
// claim suite overrides them per ensemble member via SetGlobalSeed.
func declarativeBuildStub(
	prescribingRate float64,
	numSteps int,
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
	gen.GetPartition("colonisation").Params.Map["prescribing_rate"] =
		[]float64{prescribingRate}
	return gen
}

// declarativeIteration returns the expression iteration the YAML supplies for a partition,
// alongside the params it declares.
func declarativeIteration(
	t *testing.T,
	partition string,
) (*general.ExpressionIteration, map[string][]float64) {
	t.Helper()
	config := declarativeBuildStub(BaselinePrescribingRate, 10).GetPartition(partition)
	iteration, ok := config.Iteration.(*general.ExpressionIteration)
	if !ok {
		t.Fatalf("%s is not expression-backed: got %T", partition, config.Iteration)
	}
	return iteration, config.Params.Map
}

// assertClose fails when two values differ by more than tol, comparing relatively once the
// magnitude exceeds 1. It returns the deviation so callers can report the worst one.
func assertClose(t *testing.T, got, want, tol float64, context string) float64 {
	t.Helper()
	d := math.Abs(got - want)
	if s := math.Abs(want); s > 1 {
		d /= s
	}
	if d > tol {
		t.Fatalf("%s: declarative=%v bespoke=%v deviation=%g tolerance=%g",
			context, got, want, d, tol)
	}
	return d
}

// timesteps builds a one-step timesteps history with the given increment.
func timesteps(step int, dt float64) *simulator.CumulativeTimestepsHistory {
	return &simulator.CumulativeTimestepsHistory{
		Values:            mat.NewVecDense(1, []float64{float64(step)}),
		NextIncrement:     dt,
		CurrentStepNumber: step + 1,
	}
}

// history wraps a single state row as a depth-1 history.
func history(values []float64) *simulator.StateHistory {
	return &simulator.StateHistory{
		Values:            mat.NewDense(1, len(values), append([]float64{}, values...)),
		StateWidth:        len(values),
		StateHistoryDepth: 1,
	}
}

// moments returns the sample mean and standard deviation of xs.
func moments(xs []float64) (float64, float64) {
	mean := 0.0
	for _, x := range xs {
		mean += x
	}
	mean /= float64(len(xs))
	varSum := 0.0
	for _, x := range xs {
		d := x - mean
		varSum += d * d
	}
	return mean, math.Sqrt(varSum / float64(len(xs)))
}

// mcTolerance returns the bound for comparing a statistic between the two builds. This is a
// SAMPLING bound, not a rounding bound: the two draw from different generators, so their
// estimates of the same moment differ by Monte Carlo error alone, and the only honest
// tolerance is one derived from that error. For a difference of two independent sample means
// the standard error is sd*sqrt(2/n); for a difference of two independent sample standard
// deviations it is about sd/sqrt(n). Both are widened to mcSigmas standard errors.
//
// A lane can be deterministic — a strain at zero kills its own noise — which makes the
// sampling bound zero. The bound is floored at a few parts per billion of the value's scale
// so such a lane absorbs the float accumulation of summing n identical samples instead of
// knife-edging on it; that floor is still orders of magnitude below any modelling difference.
func mcTolerance(sd, scale float64, n int, forMean bool) float64 {
	bound := mcSigmas * sd / math.Sqrt(float64(n))
	if forMean {
		bound = mcSigmas * sd * math.Sqrt(2/float64(n))
	}
	return math.Max(bound, 1e-9*math.Abs(scale))
}

// drawSamples calls Iterate n times at a fixed input, returning one slice of samples per
// state element. The output slice is copied on every call: ExpressionIteration returns a
// reused buffer.
func drawSamples(
	iteration simulator.Iteration,
	n, partitionIndex, width int,
	params *simulator.Params,
	histories []*simulator.StateHistory,
) [][]float64 {
	out := make([][]float64, width)
	for k := range out {
		out[k] = make([]float64, n)
	}
	for i := 0; i < n; i++ {
		v := iteration.Iterate(params, partitionIndex, histories, timesteps(i, 1.0))
		for k := 0; k < width; k++ {
			out[k][i] = v[k]
		}
	}
	return out
}

// colonisationSettings configures both builds for the colonisation partition. Params is left
// empty of prescribing_partition, so ColonisationDynamicsIteration takes its direct
// prescribing_rate path — the one BuildStub actually assembles. (The prescribing_partition
// and learned_params branches are inference/policy hooks the stub never configures, so they
// are outside the model this twin states and outside this comparison.)
func colonisationSettings() *simulator.Settings {
	return &simulator.Settings{
		Iterations: []simulator.IterationSettings{{Name: "colonisation", Seed: 5}},
	}
}

func TestDeclarativeColonisationDriftMatchesBespokeExactly(t *testing.T) {
	// With noise_scale = 0 both sides multiply their draw by zero, so the streams stop
	// mattering and the drift, the uncolonised clamp, the zero clamp and the simplex
	// renormalisation are all comparable value-by-value. This is the strongest check
	// available for this model, and it covers everything except the noise magnitude.
	bespoke := &ColonisationDynamicsIteration{}
	declarative, _ := declarativeIteration(t, "colonisation")
	settings := colonisationSettings()
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	rng := rand.New(rand.NewPCG(31, 32))
	branches := map[string]int{}
	maxDev := 0.0

	const cases = 20000
	for i := 0; i < cases; i++ {
		// Ranged above the simplex and below zero — further than one step of drift can
		// correct — so the clamps and the renormalisation trigger often rather than
		// occasionally. Physical trajectories never leave [0,1], so only a range this wide
		// exercises the code that keeps them there.
		s := rng.Float64()*1.1 - 0.2
		r := rng.Float64()*1.1 - 0.2
		dt := 0.5 + rng.Float64()
		p := simulator.NewParams(map[string][]float64{
			"community_susceptible_prevalence": {rng.Float64() * 0.3},
			"community_resistant_prevalence":   {rng.Float64() * 0.3},
			"turnover_rate":                    {rng.Float64() * 0.2},
			"transmission_rate":                {rng.Float64() * 0.1},
			"selection_coefficient":            {rng.Float64() * 0.3},
			"fitness_cost":                     {rng.Float64() * 0.2},
			"noise_scale":                      {0.0},
			"prescribing_rate":                 {rng.Float64()},
		})
		mk := func() []*simulator.StateHistory {
			return []*simulator.StateHistory{history([]float64{s, r})}
		}
		ts := timesteps(i, dt)

		want := bespoke.Iterate(&p, 0, mk(), ts)
		got := declarative.Iterate(&p, 0, mk(), ts)

		if 1-s-r <= 0 {
			branches["uncolonised_clamped"]++
		} else {
			branches["uncolonised_positive"]++
		}
		if s < 0 || r < 0 {
			branches["sqrt_argument_clamped"]++
		} else {
			branches["sqrt_argument_positive"]++
		}
		if want[0] == 0 || want[1] == 0 {
			branches["output_clamped_at_zero"]++
		}
		if math.Abs(want[0]+want[1]-1) < 1e-12 {
			branches["renormalised"]++
		} else {
			branches["inside_simplex"]++
		}
		for k := range want {
			maxDev = math.Max(maxDev,
				assertClose(t, got[k], want[k], exactTolerance, "colonisation fraction"))
		}
	}
	for _, b := range []string{
		"uncolonised_clamped", "uncolonised_positive",
		"sqrt_argument_clamped", "sqrt_argument_positive",
		"output_clamped_at_zero", "renormalised", "inside_simplex",
	} {
		if branches[b] == 0 {
			t.Errorf("branch %q never exercised; the comparison is weaker than it looks", b)
		}
	}
	t.Logf("branches exercised: %v", branches)
	t.Logf("max deviation: %g", maxDev)
}

func TestDeclarativeColonisationNoiseMatchesInDistribution(t *testing.T) {
	// The noise magnitude is the one part of colonisation the exact test above cannot reach,
	// because the two sides draw from different generators. Compare its moments instead.
	bespoke := &ColonisationDynamicsIteration{}
	declarative, _ := declarativeIteration(t, "colonisation")
	settings := colonisationSettings()
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	scenarios := []struct {
		name       string
		state      []float64
		noiseScale float64
	}{
		// Well inside the simplex: nothing clamps, so the sd is purely the diffusion term
		// and the comparison is about the noise itself.
		{"interior", []float64{0.15, 0.05}, 0.01},
		// A strain at zero: sqrt(0) kills its noise entirely, so the resistant lane must be
		// deterministic on both sides. This is the "extinction is absorbing" property.
		{"resistant_extinct", []float64{0.3, 0.0}, 0.01},
		// Noise large enough that the zero clamp and the renormalisation bite, so the
		// compared distributions are the clamped ones, not clean Gaussians.
		{"clamping", []float64{0.02, 0.96}, 0.2},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			p := simulator.NewParams(map[string][]float64{
				"community_susceptible_prevalence": {DefaultCommunitySusceptiblePrevalence},
				"community_resistant_prevalence":   {DefaultCommunityResistantPrevalence},
				"turnover_rate":                    {DefaultTurnoverRate},
				"transmission_rate":                {DefaultTransmissionRate},
				"selection_coefficient":            {DefaultSelectionCoefficient},
				"fitness_cost":                     {DefaultFitnessCost},
				"noise_scale":                      {sc.noiseScale},
				"prescribing_rate":                 {BaselinePrescribingRate},
			})
			histories := []*simulator.StateHistory{history(sc.state)}
			want := drawSamples(bespoke, mcSamples, 0, 2, &p, histories)
			got := drawSamples(declarative, mcSamples, 0, 2, &p, histories)

			for k, lane := range []string{"susceptible", "resistant"} {
				wantMean, wantSd := moments(want[k])
				gotMean, gotSd := moments(got[k])
				meanTol := mcTolerance(wantSd, wantMean, mcSamples, true)
				sdTol := mcTolerance(wantSd, wantMean, mcSamples, false)
				assertClose(t, gotMean, wantMean, meanTol, lane+" mean")
				assertClose(t, gotSd, wantSd, sdTol, lane+" sd")
				t.Logf("%s: mean %.8f vs %.8f (tol %.2e), sd %.8f vs %.8f (tol %.2e)",
					lane, gotMean, wantMean, meanTol, gotSd, wantSd, sdTol)
			}
		})
	}
}

func TestDeclarativeInfectionMatchesInDistribution(t *testing.T) {
	// infection is distributional-only: even with a matching generator, the bespoke Knuth
	// sampler and the engine's rng.Poisson are different algorithms. What a moment
	// comparison does check is the part that can actually be wrong in translation — the
	// lambda plumbing: the upstream lag-1 read of colonisation, the field layout, and
	// lambda = infection_probability * population * fraction * dt.
	bespoke := &InfectionProcessIteration{}
	declarative, _ := declarativeIteration(t, "infection")
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{
			{Name: "colonisation", Seed: 5},
			{Name: "infection", Seed: 7, Params: simulator.NewParams(
				map[string][]float64{"colonisation_partition": {0}},
			)},
		},
	}
	bespoke.Configure(1, settings)
	declarative.Configure(1, settings)

	scenarios := []struct {
		name            string
		fractions       []float64
		infectionProb   float64
		population      float64
		expectedLambdas []float64
	}{
		// A strain at zero: lambda = 0, where the bespoke sampler returns without drawing at
		// all and rng.Poisson draws once and returns 0. Both must be identically zero.
		{"zero_lambda", []float64{0.15, 0.0}, 0.01, 500, []float64{0.75, 0.0}},
		// The flagship regime: lambda well under 1.
		{"flagship", []float64{0.15, 0.05}, 0.01, 500, []float64{0.75, 0.25}},
		// Straddles rng.Poisson's own lambda=10 switch from direct to PTRS while the bespoke
		// side is still in Knuth.
		{"ptrs_vs_knuth", []float64{0.024, 0.05}, 0.5, 500, []float64{6.0, 12.5}},
		// Above the bespoke lambda=30 switch, where poissonSample stops being Poisson and
		// becomes a rounded normal. Its normal approximation is constructed to match
		// Poisson's mean and variance, so a two-moment comparison cannot distinguish the two
		// here and does not claim to — it is checking the lambda, not the tail shape.
		{"normal_approximation", []float64{0.16, 0.2}, 0.5, 500, []float64{40.0, 50.0}},
	}

	// Which sampler branch each lane lands in is derived from its lambda rather than
	// declared alongside the scenario, so the counts cannot claim a branch the inputs do not
	// actually reach. The thresholds are the ones in the two implementations: poissonSample
	// returns early at lambda<=0 and switches to its normal approximation at 30; rng.Poisson
	// switches from exponential-interarrival to PTRS at 10.
	branchesFor := func(lambda float64) []string {
		switch {
		case lambda <= 0:
			return []string{"bespoke_no_draw", "declarative_direct"}
		case lambda < 10:
			return []string{"bespoke_knuth", "declarative_direct"}
		case lambda < 30:
			return []string{"bespoke_knuth", "declarative_ptrs"}
		default:
			return []string{"bespoke_normal_approximation", "declarative_ptrs"}
		}
	}

	branches := map[string]int{}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			for _, lambda := range sc.expectedLambdas {
				for _, b := range branchesFor(lambda) {
					branches[b]++
				}
			}
			p := simulator.NewParams(map[string][]float64{
				"infection_probability":  {sc.infectionProb},
				"patient_population":     {sc.population},
				"colonisation_partition": {0},
			})
			histories := []*simulator.StateHistory{
				history(sc.fractions),
				history([]float64{0, 0}),
			}
			want := drawSamples(bespoke, mcSamples, 1, 2, &p, histories)
			got := drawSamples(declarative, mcSamples, 1, 2, &p, histories)

			for k, lane := range []string{"susceptible_bsi", "resistant_bsi"} {
				wantMean, wantSd := moments(want[k])
				gotMean, gotSd := moments(got[k])
				lambda := sc.expectedLambdas[k]
				// Both samplers must be centred on the lambda the wiring implies; this is
				// what pins the upstream read and the rate formula. The reference sd is
				// Poisson's own sqrt(lambda), floored at 1 so the lambda=0 lane (which is
				// deterministically zero on both sides) still gets a usable bound.
				refSd := math.Sqrt(math.Max(lambda, 1))
				meanTol := mcTolerance(refSd, refSd, mcSamples, true)
				sdTol := mcTolerance(refSd, refSd, mcSamples, false)
				assertClose(t, gotMean, lambda, meanTol, lane+" mean vs lambda")
				assertClose(t, wantMean, lambda, meanTol, lane+" bespoke mean vs lambda")
				assertClose(t, gotMean, wantMean, meanTol, lane+" mean")
				assertClose(t, gotSd, wantSd, sdTol, lane+" sd")
				t.Logf("%s: lambda %.4f, mean %.6f vs %.6f, sd %.6f vs %.6f",
					lane, lambda, gotMean, wantMean, gotSd, wantSd)
			}
		})
	}
	for _, b := range []string{
		"bespoke_no_draw", "bespoke_knuth", "bespoke_normal_approximation",
		"declarative_direct", "declarative_ptrs",
	} {
		if branches[b] == 0 {
			t.Errorf("branch %q never exercised; the comparison is weaker than it looks", b)
		}
	}
	t.Logf("sampler branches exercised: %v", branches)
}

func TestDeclarativeAMRAnswersTheSameClaims(t *testing.T) {
	// The oracle is the model's own behaviour suite: every claim recomputed against the
	// declarative build must still hold — same direction, same thresholds.
	//
	// It deliberately does NOT assert the observations match the bespoke numbers. The two
	// builds sample from different generators, so they will not match, and a tolerance wide
	// enough to admit them would assert nothing. What survives a change of draw stream is
	// exactly what the claims are made of: the direction of each response and the magnitude
	// thresholds. A twin with the wrong wiring, the wrong params or the wrong state layout
	// still fails here.
	if testing.Short() {
		t.Skip("runs the full claim ensemble")
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
		if len(claim.Observations) != len(reference.Observations) {
			t.Fatalf("claim %q: got %d observations, want %d",
				claim.ID, len(claim.Observations), len(reference.Observations))
		}
		// Still a true claim when the model is data.
		if err := cardgen.Verify(claim); err != nil {
			t.Errorf("claim %q does not hold for the declarative model: %v", claim.ID, err)
			continue
		}
		for k, obs := range claim.Observations {
			t.Logf("%s / %s: declarative %.4f (bespoke %.4f)",
				claim.ID, obs.Label, obs.Value, reference.Observations[k].Value)
		}
	}
}
