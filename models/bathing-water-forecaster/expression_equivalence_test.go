package bathingwater

// Does declarative.yaml — this model written as data, with no Go — reproduce the model
// stub.go builds in code?
//
// Two independent checks, because they fail in different ways. The step-for-step tests
// compare single Iterate calls over randomised inputs, which catches a mis-stated formula
// exactly. The suite test re-runs the model's own claim computations against the
// declarative build, which catches a model that is subtly different in a way per-step
// agreement would not: wrong wiring, wrong param values, wrong state layout.
//
// The anomaly is the only partition that draws: it takes one normal per step from a stream
// seeded exactly as the declarative one is (rng.New(seed) and rand.New(rand.NewPCG(seed,
// seed)) are the same generator), so the two stay in lockstep and equivalence is decidable
// directly rather than only in distribution. The sites draw nothing at all — every
// stochastic, cross-site-correlated movement reaches them through the anomaly — so their
// agreement is a pure question of formula.
//
// Agreement is asserted to a tight tolerance rather than bit-for-bit: compiled Go is free
// to contract a + b*c into a fused multiply-add, which rounds differently from the
// evaluator's separate operations.

import (
	"math"
	"math/rand/v2"
	"testing"

	"gonum.org/v1/gonum/mat"

	"github.com/umbralcalc/stochadex/models/cardgen"
	"github.com/umbralcalc/stochadex/pkg/api"
	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// tolerance is well above FMA rounding and well below any difference a real modelling
// discrepancy would produce.
const tolerance = 1e-12

// declarativeBuildStub assembles the model from declarative.yaml, matching BuildStub's
// signature so it can be dropped into the behaviour helpers. The YAML holds the model; the
// run knobs BuildStub takes as arguments are injected here, exactly as BuildStub injects
// them into its Go partitions. The sites need no seed injected because BuildStub pins them
// to 0 and they draw nothing.
func declarativeBuildStub(
	anomalyVolatility float64,
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
	gen.GetPartition("anomaly").Params.Map["sigmas"] = []float64{anomalyVolatility}
	gen.GetPartition("anomaly").Seed = seed
	return gen
}

// declarativeIteration returns the expression iteration the YAML supplies for a partition,
// alongside the params it declares.
func declarativeIteration(
	t *testing.T,
	partition string,
) (*general.ExpressionIteration, map[string][]float64) {
	t.Helper()
	config := declarativeBuildStub(DefaultAnomalyVolatility, 10, 0).GetPartition(partition)
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

func TestDeclarativeAnomalyMatchesBespoke(t *testing.T) {
	bespoke := &continuous.OrnsteinUhlenbeckIteration{}
	declarative, _ := declarativeIteration(t, "anomaly")
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{{Name: "anomaly", Seed: 5}},
	}
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	rng := rand.New(rand.NewPCG(31, 32))
	// The Euler-Maruyama step is branch-free, so what needs covering is its regimes rather
	// than any if: the drift must be seen pulling from both sides of the mean (a sign error
	// in mus - z hides entirely when z is always below it), and the two streams must be
	// seen staying aligned across a varying volatility.
	regimes := map[string]int{}
	maxDev := 0.0

	const cases = 20000
	for i := 0; i < cases; i++ {
		z := rng.Float64()*8 - 4
		mu := rng.Float64()*2 - 1
		p := simulator.NewParams(map[string][]float64{
			"thetas": {rng.Float64()},
			"mus":    {mu},
			"sigmas": {rng.Float64()},
		})
		mk := func() []*simulator.StateHistory {
			return []*simulator.StateHistory{{
				Values:            mat.NewDense(1, 1, []float64{z}),
				StateWidth:        1,
				StateHistoryDepth: 1,
			}}
		}
		// dt is swept away from 1 even though the model runs at a constant stepsize of 1,
		// because at dt=1 both the drift's dt factor and the diffusion's sqrt(dt) are
		// no-ops: a twin that dropped either would agree anyway, and the test would be
		// asserting less than it appears to.
		dt := 0.25 + rng.Float64()*1.75
		ts := &simulator.CumulativeTimestepsHistory{
			Values:            mat.NewVecDense(1, []float64{float64(i)}),
			NextIncrement:     dt,
			CurrentStepNumber: i + 1,
		}
		if dt > 1 {
			regimes["dt_above_1"]++
		} else {
			regimes["dt_below_1"]++
		}

		want := bespoke.Iterate(&p, 0, mk(), ts)
		got := declarative.Iterate(&p, 0, mk(), ts)

		if z > mu {
			regimes["reverting_down"]++
		} else {
			regimes["reverting_up"]++
		}
		maxDev = math.Max(maxDev, assertClose(t, got[0], want[0], "anomaly"))
	}
	for _, r := range []string{
		"reverting_down", "reverting_up", "dt_above_1", "dt_below_1",
	} {
		if regimes[r] == 0 {
			t.Errorf("regime %q never exercised; the comparison is weaker than it looks", r)
		}
	}
	t.Logf("regimes exercised: %v", regimes)
	t.Logf("max deviation: %g", maxDev)
}

func TestDeclarativeConcentrationMatchesBespoke(t *testing.T) {
	bespoke := &BathingConcentrationIteration{}
	declarative, _ := declarativeIteration(t, "site_0")
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{{Name: "site_0", Seed: 5}},
	}
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	rng := rand.New(rand.NewPCG(41, 42))
	// BathingConcentrationIteration has no if in it, so the branches that must be covered
	// are the ones inside the transcendental functions the two sides each call: math.Erfc
	// switches algorithm by magnitude, and saturates to 0 and to 2 in the tails. The
	// exceedance tails are also where this model is actually read — a site sits below the
	// threshold nearly always — so agreeing only in the comfortable middle would be a weak
	// result. The ranges below are deliberately wider than the default scenario (which
	// pins z_score near -2) to reach both saturated tails.
	branches := map[string]int{}
	maxDev := 0.0

	const cases = 20000
	for i := 0; i < cases; i++ {
		// A wide sample_scale range is what spreads z_score across roughly [-50, 50]: at
		// the small end the tails saturate, at the large end the mid-range is dense.
		sigma := 0.1 + rng.Float64()*2.0
		p := simulator.NewParams(map[string][]float64{
			"baseline":           {2 + rng.Float64()*6},
			"seasonal_amplitude": {rng.Float64() * 2},
			"seasonal_phase":     {rng.Float64() * 2 * math.Pi},
			"period":             {30 + rng.Float64()*370},
			"anomaly_loading":    {rng.Float64()*2 - 0.5},
			"sample_scale":       {sigma},
			"log_threshold":      {5 + rng.Float64()*2},
			"anomaly":            {rng.Float64()*8 - 4},
		})
		mk := func() []*simulator.StateHistory {
			return []*simulator.StateHistory{{
				Values:            mat.NewDense(1, 2, []float64{0, 0}),
				StateWidth:        2,
				StateHistoryDepth: 1,
			}}
		}
		// Cumulative time is swept over many periods and is what drives the seasonal term,
		// so sin is compared across its whole cycle rather than near one phase.
		ts := &simulator.CumulativeTimestepsHistory{
			Values:            mat.NewVecDense(1, []float64{rng.Float64() * 2000}),
			NextIncrement:     1.0,
			CurrentStepNumber: i + 1,
		}

		want := bespoke.Iterate(&p, 0, mk(), ts)
		got := declarative.Iterate(&p, 0, mk(), ts)

		switch {
		case want[1] < 1e-6:
			branches["p_lower_tail"]++
		case want[1] > 1-1e-6:
			branches["p_upper_tail"]++
		default:
			branches["p_mid_range"]++
		}
		// The seasonal term must be seen on both sides of zero, or a sign or phase error in
		// the sin argument would survive.
		season := want[0] - p.Map["baseline"][0] -
			p.Map["anomaly_loading"][0]*p.Map["anomaly"][0]
		if season > 0 {
			branches["season_positive"]++
		} else {
			branches["season_negative"]++
		}

		maxDev = math.Max(maxDev, assertClose(t, got[0], want[0], "mu"))
		maxDev = math.Max(maxDev, assertClose(t, got[1], want[1], "p_exceed"))
	}
	for _, b := range []string{
		"p_lower_tail", "p_upper_tail", "p_mid_range",
		"season_positive", "season_negative",
	} {
		if branches[b] == 0 {
			t.Errorf("branch %q never exercised; the comparison is weaker than it looks", b)
		}
	}
	t.Logf("branches exercised: %v", branches)
	t.Logf("max deviation: %g", maxDev)
}

func TestDeclarativeBathingWaterAnswersTheSameClaims(t *testing.T) {
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
