package homark

import (
	"sync"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// runStubOverride runs the stub with an override hook applied to the generated
// config before the run, so a behaviour test can vary any partition's params
// (approvals, thresholds, coefficients, rates) without bloating BuildStub's
// signature — BuildStub still exposes only the one headline driver.
func runStubOverride(
	t *testing.T,
	approvalRate float64,
	numSteps int,
	seed uint64,
	override func(*simulator.ConfigGenerator),
) *simulator.StateTimeStorage {
	t.Helper()
	gen := BuildStub(approvalRate, numSteps, seed)
	if override != nil {
		override(gen)
	}
	settings, implementations := gen.GenerateConfigs()
	store := simulator.NewStateTimeStorage()
	implementations.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	coordinator := simulator.NewPartitionCoordinator(settings, implementations)
	var wg sync.WaitGroup
	for !coordinator.ReadyToTerminate() {
		coordinator.Step(&wg)
	}
	return store
}

// setParam overwrites a single scalar param on a named partition.
func setParam(gen *simulator.ConfigGenerator, partition, key string, value float64) {
	gen.GetPartition(partition).Params.Map[key] = []float64{value}
}

// meanFinalAff ensemble-averages the final price-to-earnings ratio over nMembers
// seeds under a fixed approval rate and override, damping single-run noise.
func meanFinalAff(
	t *testing.T,
	approvalRate float64,
	numSteps, nMembers int,
	override func(*simulator.ConfigGenerator),
) float64 {
	t.Helper()
	var sum float64
	for m := 0; m < nMembers; m++ {
		store := runStubOverride(t, approvalRate, numSteps, uint64(3000+m), override)
		sum += finalAffordability(store)
	}
	return sum / float64(nMembers)
}

// meanPipelineStock ensemble-averages the pipeline stock over every step and
// nMembers seeds — the cleanest summary of how full the supply pipeline runs.
func meanPipelineStock(
	t *testing.T,
	approvalRate float64,
	numSteps, nMembers int,
	override func(*simulator.ConfigGenerator),
) float64 {
	t.Helper()
	var sum float64
	var count int
	for m := 0; m < nMembers; m++ {
		store := runStubOverride(t, approvalRate, numSteps, uint64(3000+m), override)
		for _, row := range store.GetValues("pipeline") {
			sum += row[0]
			count++
		}
	}
	return sum / float64(count)
}

// TestHomarkExpectedBehaviour is the expected-behaviour suite: each subtest name
// states, in plain language, a response the model is claimed to produce, and the
// body checks it. Together they specify how the model behaves for a downstream
// decision-maker (actionable levers) and why it should be trusted off-sample
// (structural drivers). Affordability is a price-to-earnings ratio, so a *lower*
// value is *better* affordability.
func TestHomarkExpectedBehaviour(t *testing.T) {
	const steps, nMembers = DefaultNumSteps, 8

	// ----- Decision-path responses (actionable planning levers a downstream controls) -----

	// The core supply lever: raising planning approvals builds a larger market-facing
	// committed pipeline, whose anticipated supply dampens price growth, so the
	// price-to-earnings ratio ends lower. A wrong sign here is a wrong policy
	// recommendation.
	t.Run("higher_approval_rate_improves_affordability", func(t *testing.T) {
		base := meanFinalAff(t, 60.0, steps, nMembers, nil)
		more := meanFinalAff(t, 240.0, steps, nMembers, nil)
		if !(more < base) {
			t.Fatalf("expected more approvals to lower the price-to-earnings ratio: "+
				"base(60)=%.3f more(240)=%.3f", base, more)
		}
	})

	// The tenure/affordable-requirement lever: reducing the market-facing delivery
	// fraction (more of the pipeline diverted to non-market tenures) weakens the
	// supply-dampening channel, so market prices — and the price-to-earnings ratio —
	// end higher. Worsening affordability is the correct sign for tighter market supply.
	t.Run("lower_market_delivery_fraction_worsens_affordability", func(t *testing.T) {
		full := meanFinalAff(t, DefaultApprovalRate, steps, nMembers, func(g *simulator.ConfigGenerator) {
			setParam(g, "price_drift", "market_fraction", 1.0)
		})
		reduced := meanFinalAff(t, DefaultApprovalRate, steps, nMembers, func(g *simulator.ConfigGenerator) {
			setParam(g, "price_drift", "market_fraction", 0.3)
		})
		if !(reduced > full) {
			t.Fatalf("expected a lower market-delivery fraction to raise the price-to-earnings ratio: "+
				"full(1.0)=%.3f reduced(0.3)=%.3f", full, reduced)
		}
	})

	// ----- Structural-driver responses (non-actionable; out-of-sample credibility) -----

	// The mortgage-cost channel: a higher policy rate raises borrowing costs and
	// dampens house-price growth, so the price-to-earnings ratio ends lower (the
	// market cools). Bank rate is set by the central bank, not the local authority —
	// getting this sign right is a credibility check, not a lever.
	t.Run("higher_policy_rate_lowers_price_to_earnings", func(t *testing.T) {
		base := meanFinalAff(t, DefaultApprovalRate, steps, nMembers, nil)
		hiked := meanFinalAff(t, DefaultApprovalRate, steps, nMembers, func(g *simulator.ConfigGenerator) {
			setParam(g, "bank_rate", "mus", 6.0)
		})
		if !(hiked < base) {
			t.Fatalf("expected a higher policy rate to lower the price-to-earnings ratio: "+
				"base(μ=3)=%.3f hiked(μ=6)=%.3f", base, hiked)
		}
	})

	// The demand-pressure channel: switching on the earnings→price coupling lets
	// rising incomes bid prices up faster than they lift the denominator, so the
	// price-to-earnings ratio ends higher. This is the demand side of the
	// demand–supply pressure term in the drift.
	t.Run("stronger_demand_pressure_raises_price_to_earnings", func(t *testing.T) {
		base := meanFinalAff(t, DefaultApprovalRate, steps, nMembers, nil) // demand_beta = 0
		coupled := meanFinalAff(t, DefaultApprovalRate, steps, nMembers, func(g *simulator.ConfigGenerator) {
			setParam(g, "price_drift", "demand_beta", 0.03)
		})
		if !(coupled > base) {
			t.Fatalf("expected demand pressure to raise the price-to-earnings ratio: "+
				"base(β=0)=%.3f coupled(β=0.03)=%.3f", base, coupled)
		}
	})

	// The earnings denominator: faster earnings growth, with the demand coupling off,
	// lifts incomes without pushing prices, so the price-to-earnings ratio falls —
	// affordability improves. The direct affordability-from-incomes mechanism.
	t.Run("higher_earnings_growth_improves_affordability", func(t *testing.T) {
		base := meanFinalAff(t, DefaultApprovalRate, steps, nMembers, nil)
		fast := meanFinalAff(t, DefaultApprovalRate, steps, nMembers, func(g *simulator.ConfigGenerator) {
			setParam(g, "log_earnings", "drift_coefficients", 0.006)
		})
		if !(fast < base) {
			t.Fatalf("expected faster earnings growth to lower the price-to-earnings ratio: "+
				"base(0.0025)=%.3f fast(0.006)=%.3f", base, fast)
		}
	})

	// The pipeline mechanism: a higher completion rate empties the pipeline faster,
	// so the mean stock of units in progress runs lower for the same inflow. This is
	// the throughput invariant of the bespoke StochasticPipelineIteration (in steady
	// state, delivered supply equals the approval inflow regardless of speed; only
	// the standing stock responds).
	t.Run("faster_pipeline_completion_lowers_pipeline_stock", func(t *testing.T) {
		base := meanPipelineStock(t, DefaultApprovalRate, steps, nMembers, nil)
		faster := meanPipelineStock(t, DefaultApprovalRate, steps, nMembers, func(g *simulator.ConfigGenerator) {
			setParam(g, "pipeline", "completion_rate", 0.30)
		})
		if !(faster < base) {
			t.Fatalf("expected a faster completion rate to lower the mean pipeline stock: "+
				"base(0.15)=%.1f faster(0.30)=%.1f", base, faster)
		}
	})
}
