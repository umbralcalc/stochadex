package homark

// Does declarative.yaml — this model written as data, with no Go — reproduce the model
// stub.go builds in code?
//
// Two independent checks, because they fail in different ways. The step-for-step tests
// compare single Iterate calls over randomised inputs, which catches a mis-stated formula
// exactly. The suite test re-runs the model's own claim computations against the
// declarative build, which catches a model that is subtly different in a way per-step
// agreement would not: wrong wiring, wrong param values, wrong state layout.
//
// Every iteration here draws from a stream seeded exactly as the declarative one is:
// rng.New(seed) is rand.New(rand.NewPCG(seed, seed)), the generator the bespoke iterations
// build directly, and the pipeline's binomial reaches the same PCG either way — distuv wraps
// whatever source it is handed in a rand.Rand, and that wrapper only forwards Uint64. Both
// builds then take the same number of draws per step in the same order, so the two stay in
// lockstep and equivalence is decidable directly rather than only in distribution.
// Agreement is asserted to a tight tolerance rather than bit-for-bit: compiled Go is free to
// contract a + b*c into a fused multiply-add, which rounds differently from the evaluator's
// separate operations.

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

// partitionNames is the model's partition order. It is load-bearing twice over: the Go
// price_drift function reads its inputs by the index constants in stub.go, and the
// declarative upstream aliases resolve through these names, so the two only agree while the
// order does.
var partitionNames = []string{
	"bank_rate", "pipeline", "price_drift", "log_earnings", "log_price", "affordability",
}

// declarativeBuildStub assembles the model from declarative.yaml, matching BuildStub's
// signature so it can be dropped into the behaviour helpers. The YAML holds the model; the
// run knobs BuildStub takes as arguments are injected here, exactly as BuildStub injects
// them into its Go partitions.
func declarativeBuildStub(
	approvalRate float64,
	numSteps int,
	seed uint64,
) *simulator.ConfigGenerator {
	config := api.LoadApiRunConfigFromYaml("declarative.yaml")
	config.Main.Simulation = simulator.SimulationConfig{
		OutputCondition: &simulator.EveryStepOutputCondition{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: numSteps,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: stepSizeMonths},
		InitTimeValue:    0.0,
	}
	gen := config.Main.GetConfigGenerator()
	gen.GetPartition("pipeline").Params.Map["approval_rate"] = []float64{approvalRate}
	gen.GetPartition("bank_rate").Seed = seed
	gen.GetPartition("pipeline").Seed = seed + 1
	gen.GetPartition("log_earnings").Seed = seed + 2
	gen.GetPartition("log_price").Seed = seed + 3
	return gen
}

// declarativeIteration returns the expression iteration the YAML supplies for a partition,
// alongside the params it declares.
func declarativeIteration(
	t *testing.T,
	partition string,
) (*general.ExpressionIteration, map[string][]float64) {
	t.Helper()
	config := declarativeBuildStub(DefaultApprovalRate, 10, 0).GetPartition(partition)
	iteration, ok := config.Iteration.(*general.ExpressionIteration)
	if !ok {
		t.Fatalf("%s is not expression-backed: got %T", partition, config.Iteration)
	}
	return iteration, config.Params.Map
}

// modelSettings names every partition in the model's order at a fixed seed, which is what
// lets the declarative price_drift and affordability resolve their upstream aliases to the
// indices the Go build hardcodes.
func modelSettings(seed uint64) *simulator.Settings {
	iterations := make([]simulator.IterationSettings, len(partitionNames))
	for i, name := range partitionNames {
		iterations[i] = simulator.IterationSettings{
			Name:   name,
			Seed:   seed,
			Params: simulator.NewParams(map[string][]float64{}),
		}
	}
	return &simulator.Settings{Iterations: iterations}
}

// singleHistory builds a one-row state history per partition from the given scalar values,
// so both builds read identical inputs from identical structures.
func singleHistory(values []float64) []*simulator.StateHistory {
	histories := make([]*simulator.StateHistory, len(values))
	for i, v := range values {
		histories[i] = &simulator.StateHistory{
			Values:            mat.NewDense(1, 1, []float64{v}),
			StateWidth:        1,
			StateHistoryDepth: 1,
		}
	}
	return histories
}

// stepTimesteps varies the increment rather than pinning it at the model's own
// stepSizeMonths of 1. At dt = 1 every dt and sqrt(dt) factor is a no-op, so a twin that
// dropped one would agree anyway and this comparison would assert less than it looks like it
// does — verified by mutation: deleting dt from the bank-rate update passes at a fixed dt of
// 1 and fails here. The cycle covers dt below, at and above 1, since sqrt(dt) moves the
// opposite way either side of it.
func stepTimesteps(i int) *simulator.CumulativeTimestepsHistory {
	dt := []float64{stepSizeMonths, 0.25, 2.5, 0.75}[i%4]
	return &simulator.CumulativeTimestepsHistory{
		Values:            mat.NewVecDense(1, []float64{float64(i) * dt}),
		NextIncrement:     dt,
		CurrentStepNumber: i + 1,
	}
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

// requireBranches fails when a branch was never reached: an untriggered branch means the
// step-for-step comparison never covered it, so it is weaker than the case count suggests.
func requireBranches(t *testing.T, branches map[string]int, names ...string) {
	t.Helper()
	for _, name := range names {
		if branches[name] == 0 {
			t.Errorf("branch %q never exercised; the comparison is weaker than it looks", name)
		}
	}
	t.Logf("branches exercised: %v", branches)
}

func TestDeclarativeBankRateMatchesBespoke(t *testing.T) {
	bespoke := &continuous.OrnsteinUhlenbeckIteration{}
	declarative, _ := declarativeIteration(t, "bank_rate")
	settings := modelSettings(5)
	bespoke.Configure(0, settings)
	declarative.Configure(0, settings)

	rng := rand.New(rand.NewPCG(11, 12))
	maxDev := 0.0

	const cases = 20000
	for i := 0; i < cases; i++ {
		p := simulator.NewParams(map[string][]float64{
			"thetas": {rng.Float64() * 0.4},
			"mus":    {rng.Float64() * 8},
			"sigmas": {rng.Float64() * 0.6},
		})
		state := []float64{rng.Float64()*10 - 2}
		ts := stepTimesteps(i)

		want := bespoke.Iterate(&p, 0, singleHistory(state), ts)
		got := declarative.Iterate(&p, 0, singleHistory(state), ts)
		maxDev = math.Max(maxDev, assertClose(t, got[0], want[0], "bank rate"))
	}
	t.Logf("max deviation: %g", maxDev)
}

func TestDeclarativePipelineMatchesBespoke(t *testing.T) {
	bespoke := &StochasticPipelineIteration{}
	declarative, _ := declarativeIteration(t, "pipeline")
	settings := modelSettings(5)
	bespoke.Configure(1, settings)
	declarative.Configure(1, settings)

	rng := rand.New(rand.NewPCG(21, 22))
	branches := map[string]int{}
	maxDev := 0.0

	const cases = 18000
	for i := 0; i < cases; i++ {
		// Nine input regimes, each chosen so the branch it targets is reached for a reason
		// the test can state rather than hope for.
		var stock, completionRate, attritionRate, approvalRate float64
		mode := i % 9
		completionRate = 0.15
		attritionRate = 0.02
		approvalRate = rng.Float64() * 150
		switch mode {
		case 0, 1, 2, 3:
			// A stock of at least 200 units at a 0.15 completion rate cannot complete every
			// unit, so remaining is positive and the attrition draw always follows the
			// completion draw — which is the ordering the two streams have to agree on.
			stock = 200 + rng.Float64()*400
		case 4:
			stock = 200 + rng.Float64()*400
			completionRate = 0
		case 5:
			stock = 200 + rng.Float64()*400
			attritionRate = 0
		case 6:
			// Negative stock: floor is clamped to zero, neither draw happens, and a small
			// inflow leaves the new stock negative and clamped too.
			stock = -rng.Float64() * 40
			approvalRate = rng.Float64() * 5
		case 7:
			stock = rng.Float64()
		case 8:
			// Under 25 units distuv switches from rejection to the direct method; both
			// builds go through the same distuv call, but the regime is worth covering.
			stock = 1 + rng.Float64()*23
		}
		p := simulator.NewParams(map[string][]float64{
			"completion_rate": {completionRate},
			"attrition_rate":  {attritionRate},
			"approval_rate":   {approvalRate},
		})
		ts := stepTimesteps(i)

		want := bespoke.Iterate(&p, 1, singleHistory([]float64{0, stock}), ts)
		got := declarative.Iterate(&p, 1, singleHistory([]float64{0, stock}), ts)

		n := math.Floor(stock)
		if n < 0 {
			branches["n_clamped_negative_stock"]++
			n = 0
		}
		switch {
		case n == 0:
			branches["completions_skipped_empty_pipeline"]++
			branches["attritions_skipped_empty_remaining"]++
		case completionRate == 0:
			branches["completions_skipped_zero_rate"]++
			// No unit completed, so remaining is the whole stock and attrition draws.
			branches["attritions_drawn"]++
		default:
			branches["completions_drawn"]++
			if attritionRate == 0 {
				branches["attritions_skipped_zero_rate"]++
			} else if mode != 8 {
				branches["attritions_drawn"]++
			}
		}
		if want[0] == 0 {
			branches["stock_clamped_at_zero"]++
		}
		maxDev = math.Max(maxDev, assertClose(t, got[0], want[0], "pipeline stock"))
	}
	requireBranches(t, branches,
		"n_clamped_negative_stock",
		"completions_drawn",
		"completions_skipped_zero_rate",
		"completions_skipped_empty_pipeline",
		"attritions_drawn",
		"attritions_skipped_zero_rate",
		"attritions_skipped_empty_remaining",
		"stock_clamped_at_zero",
	)
	t.Logf("max deviation: %g", maxDev)
}

func TestDeclarativePriceDriftMatchesBespoke(t *testing.T) {
	initLogEarnings := math.Log(DefaultInitEarnings)
	bespoke := &general.ValuesFunctionIteration{
		Function: HousingPriceDriftFunction(
			bankRatePartition, pipelinePartition, logEarningsPartition, initLogEarnings,
		),
	}
	declarative, params := declarativeIteration(t, "price_drift")
	settings := modelSettings(5)
	bespoke.Configure(priceDriftPartition, settings)
	declarative.Configure(priceDriftPartition, settings)

	// The Go closure bakes initLogEarnings in; the YAML states it as a param, so the two
	// only agree while they carry the same number.
	if got := params["init_log_earnings"][0]; got != initLogEarnings {
		t.Fatalf("init_log_earnings: yaml=%v, closure=%v", got, initLogEarnings)
	}

	rng := rand.New(rand.NewPCG(31, 32))
	branches := map[string]int{}
	maxDev := 0.0

	const cases = 20000
	for i := 0; i < cases; i++ {
		// A non-positive reference stock is rare in practice but is a live branch, so a
		// fifth of the cases sweep it — including exactly zero, which is the boundary.
		pipelineRef := 200 + rng.Float64()*2000
		if i%5 == 0 {
			pipelineRef = -rng.Float64() * 500
		}
		p := simulator.NewParams(map[string][]float64{
			"drift_base":        {rng.Float64() * 0.01},
			"bank_beta":         {-rng.Float64() * 0.05},
			"pipeline_beta":     {rng.Float64() * 0.005},
			"pipeline_ref":      {pipelineRef},
			"market_fraction":   {rng.Float64()},
			"demand_beta":       {rng.Float64() * 0.05},
			"init_log_earnings": {initLogEarnings},
		})
		state := []float64{
			rng.Float64() * 8,                     // bank_rate
			rng.Float64() * 900,                   // pipeline
			0,                                     // price_drift (its own state, unread)
			initLogEarnings + rng.Float64() - 0.5, // log_earnings
		}
		ts := stepTimesteps(i)

		want := bespoke.Iterate(&p, priceDriftPartition, singleHistory(state), ts)
		got := declarative.Iterate(&p, priceDriftPartition, singleHistory(state), ts)

		if pipelineRef <= 0 {
			branches["pipeline_ref_fallback"]++
		} else {
			branches["pipeline_ref_used"]++
		}
		maxDev = math.Max(maxDev, assertClose(t, got[0], want[0], "price drift"))
	}
	requireBranches(t, branches, "pipeline_ref_fallback", "pipeline_ref_used")
	t.Logf("max deviation: %g", maxDev)
}

func TestDeclarativeDriftDiffusionPartitionsMatchBespoke(t *testing.T) {
	// log_earnings and log_price share an update; log_price differs only in taking its drift
	// from price_drift within-step, which the run-level suite test covers.
	for _, partition := range []string{"log_earnings", "log_price"} {
		t.Run(partition, func(t *testing.T) {
			index := logEarningsPartition
			if partition == "log_price" {
				index = logPricePartition
			}
			bespoke := &continuous.DriftDiffusionIteration{}
			declarative, _ := declarativeIteration(t, partition)
			settings := modelSettings(5)
			bespoke.Configure(index, settings)
			declarative.Configure(index, settings)

			rng := rand.New(rand.NewPCG(41, 42))
			maxDev := 0.0

			const cases = 20000
			for i := 0; i < cases; i++ {
				p := simulator.NewParams(map[string][]float64{
					"drift_coefficients":     {rng.Float64()*0.02 - 0.01},
					"diffusion_coefficients": {rng.Float64() * 0.05},
				})
				state := make([]float64, len(partitionNames))
				state[index] = 8 + rng.Float64()*6
				ts := stepTimesteps(i)

				want := bespoke.Iterate(&p, index, singleHistory(state), ts)
				got := declarative.Iterate(&p, index, singleHistory(state), ts)
				maxDev = math.Max(maxDev, assertClose(t, got[0], want[0], partition))
			}
			t.Logf("max deviation: %g", maxDev)
		})
	}
}

func TestDeclarativeAffordabilityMatchesBespoke(t *testing.T) {
	bespoke := &AffordabilityFromLogsIteration{}
	declarative, _ := declarativeIteration(t, "affordability")
	settings := modelSettings(5)
	// The Go iteration resolves its two inputs from partition-index params; the declarative
	// one resolves the same two by name, which is why the YAML carries no index at all.
	settings.Iterations[affordabilityPartition].Params = simulator.NewParams(
		map[string][]float64{
			"log_price_partition":    {float64(logPricePartition)},
			"log_earnings_partition": {float64(logEarningsPartition)},
		},
	)
	bespoke.Configure(affordabilityPartition, settings)
	declarative.Configure(affordabilityPartition, settings)

	rng := rand.New(rand.NewPCG(51, 52))
	maxDev := 0.0

	const cases = 20000
	for i := 0; i < cases; i++ {
		p := simulator.NewParams(map[string][]float64{})
		state := make([]float64, len(partitionNames))
		state[logEarningsPartition] = math.Log(DefaultInitEarnings) + rng.Float64() - 0.5
		state[logPricePartition] = math.Log(DefaultInitPrice) + rng.Float64()*2 - 1
		ts := stepTimesteps(i)

		want := bespoke.Iterate(&p, affordabilityPartition, singleHistory(state), ts)
		got := declarative.Iterate(&p, affordabilityPartition, singleHistory(state), ts)
		maxDev = math.Max(maxDev, assertClose(t, got[0], want[0], "affordability"))
	}
	t.Logf("max deviation: %g", maxDev)
}

func TestDeclarativeHomarkAnswersTheSameClaims(t *testing.T) {
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
