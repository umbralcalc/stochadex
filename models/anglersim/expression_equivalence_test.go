package anglersim

// Does declarative.yaml — this model written as data, with no Go — reproduce the model
// stub.go builds in code?
//
// Two independent checks, because they fail in different ways. The step-for-step tests
// compare single Iterate calls over randomised inputs, which catches a mis-stated formula
// exactly. The suite test re-runs the model's own claim computations against the
// declarative build, which catches a model that is subtly different in a way per-step
// agreement would not: wrong wiring, wrong param values, wrong state layout.
//
// Both iterations draw from a stream seeded exactly as the declarative one is
// (rng.New(seed) and rand.New(rand.NewPCG(seed, seed)) are the same generator), and both
// take the same number of draws per step, so the two stay in lockstep and equivalence is
// decidable directly rather than only in distribution. Agreement is asserted to a tight
// tolerance rather than bit-for-bit: compiled Go is free to contract a + b*c into a fused
// multiply-add, which rounds differently from the evaluator's separate operations.

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
// them into its Go partitions.
func declarativeBuildStub(
	warmingTrend float64,
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
	gen.GetPartition("covariates").Params.Map["warming_trend"] = []float64{warmingTrend}
	gen.GetPartition("covariates").Seed = seed
	gen.GetPartition("population").Seed = seed + 997
	return gen
}

// declarativeIteration returns the expression iteration the YAML supplies for a partition,
// alongside the params it declares.
func declarativeIteration(
	t *testing.T,
	partition string,
) (*general.ExpressionIteration, map[string][]float64) {
	t.Helper()
	config := declarativeBuildStub(DefaultWarmingTrend, 10, 0).GetPartition(partition)
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

func TestDeclarativeCovariatesMatchBespoke(t *testing.T) {
	bespoke := &ClimateCovariatesIteration{}
	declarative, params := declarativeIteration(t, "covariates")
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{{Name: "covariates", Seed: 5}},
	}
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	rng := rand.New(rand.NewPCG(11, 12))
	clipped, warmed, maxDev := 0, 0, 0.0

	const cases = 20000
	for i := 0; i < cases; i++ {
		// Ranged well below zero — further than mean reversion can pull back in one step —
		// so the non-negativity clip on flow and dissolved oxygen actually triggers often,
		// and with a warming trend so the masked drift is exercised.
		state := []float64{
			rng.Float64()*1.5 - 1.0,
			8 + rng.Float64()*10,
			rng.Float64()*11 - 2.0,
		}
		warming := rng.Float64()*0.2 - 0.1
		p := simulator.NewParams(map[string][]float64{
			"baseline_levels": params["baseline_levels"],
			"reversion_rates": params["reversion_rates"],
			"volatilities":    params["volatilities"],
			"warming_mask":    params["warming_mask"],
			"nonneg_mask":     params["nonneg_mask"],
			"warming_trend":   {warming},
		})
		mk := func() []*simulator.StateHistory {
			return []*simulator.StateHistory{{
				Values:            mat.NewDense(1, 3, append([]float64{}, state...)),
				StateWidth:        3,
				StateHistoryDepth: 1,
			}}
		}
		ts := &simulator.CumulativeTimestepsHistory{
			Values:            mat.NewVecDense(1, []float64{float64(i)}),
			NextIncrement:     1.0,
			CurrentStepNumber: i + 1,
		}

		want := bespoke.Iterate(&p, 0, mk(), ts)
		got := declarative.Iterate(&p, 0, mk(), ts)

		if want[0] == 0 || want[2] == 0 {
			clipped++
		}
		if warming != 0 {
			warmed++
		}
		for k := range want {
			maxDev = math.Max(maxDev, assertClose(t, got[k], want[k], "covariate"))
		}
		// The warming drift must land on temperature and nowhere else, which is the whole
		// job of warming_mask replacing the Go tempIndex constant.
		if want[1] != got[1] && math.Abs(want[1]-got[1]) > tolerance {
			t.Fatalf("case %d: warming drift diverged", i)
		}
	}
	if clipped == 0 {
		t.Error("the non-negativity clip never triggered; the comparison is weak")
	}
	t.Logf("cases where a covariate was clipped at zero: %d/%d", clipped, cases)
	t.Logf("max deviation: %g", maxDev)
}

func TestDeclarativeRickerMatchesBespoke(t *testing.T) {
	bespoke := &RickerIteration{}
	declarative, params := declarativeIteration(t, "population")
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{{Name: "population", Seed: 5}},
	}
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	rng := rand.New(rand.NewPCG(21, 22))
	branches := map[string]int{}
	maxDev := 0.0

	const cases = 20000
	for i := 0; i < cases; i++ {
		logN := rng.Float64()*6 - 5
		// Sweep the two guarded paths — the Allee multiplier and the noise draw — so both
		// sides of each guard are compared, and both stay in step on the draw stream.
		gamma := 0.0
		if i%3 == 0 {
			gamma = rng.Float64() * 40
		}
		sigma := 0.0
		if i%5 != 0 {
			sigma = rng.Float64() * 0.5
		}
		p := simulator.NewParams(map[string][]float64{
			"growth_rate":            {rng.Float64() * 1.5},
			"density_dependence":     {0.2 + rng.Float64()*2},
			"covariate_coefficients": params["covariate_coefficients"],
			"process_noise_sd":       {sigma},
			"allee_effect":           {gamma},
			"covariates": {
				rng.Float64(),
				8 + rng.Float64()*10,
				5 + rng.Float64()*6,
			},
		})
		mk := func() []*simulator.StateHistory {
			return []*simulator.StateHistory{{
				Values:            mat.NewDense(1, 1, []float64{logN}),
				StateWidth:        1,
				StateHistoryDepth: 1,
			}}
		}
		ts := &simulator.CumulativeTimestepsHistory{
			Values:            mat.NewVecDense(1, []float64{float64(i)}),
			NextIncrement:     1.0,
			CurrentStepNumber: i + 1,
		}

		want := bespoke.Iterate(&p, 0, mk(), ts)
		got := declarative.Iterate(&p, 0, mk(), ts)

		if gamma > 0 {
			branches["allee"]++
		} else {
			branches["standard_ricker"]++
		}
		if sigma > 0 {
			branches["noisy"]++
		} else {
			branches["deterministic"]++
		}
		maxDev = math.Max(maxDev, assertClose(t, got[0], want[0], "log density"))
	}
	for _, b := range []string{"allee", "standard_ricker", "noisy", "deterministic"} {
		if branches[b] == 0 {
			t.Errorf("branch %q never exercised; the comparison is weaker than it looks", b)
		}
	}
	t.Logf("branches exercised: %v", branches)
	t.Logf("max deviation: %g", maxDev)
}

func TestDeclarativeAnglersimAnswersTheSameClaims(t *testing.T) {
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
