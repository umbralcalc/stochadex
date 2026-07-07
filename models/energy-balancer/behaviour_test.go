package energybalancer

import (
	"sync"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// runStubOverride runs the stub with an override hook applied to the generated
// config before the run, so a behaviour test can vary any partition's params
// (thresholds, sizing, efficiencies, structural coefficients) without bloating
// BuildStub's signature — BuildStub still exposes only the one headline driver.
func runStubOverride(
	t *testing.T,
	renewablePenetration float64,
	numSteps int,
	seed uint64,
	override func(*simulator.ConfigGenerator),
) *simulator.StateTimeStorage {
	t.Helper()
	gen := BuildStub(renewablePenetration, numSteps, seed)
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

// finalValue returns the last recorded value of state component idx of the named
// partition.
func finalValue(store *simulator.StateTimeStorage, partition string, idx int) float64 {
	rows := store.GetValues(partition)
	return rows[len(rows)-1][idx]
}

// meanFinalValue ensemble-averages finalValue over nMembers seeds under a fixed
// penetration and override, damping single-run noise.
func meanFinalValue(
	t *testing.T,
	renewablePenetration float64,
	numSteps, nMembers int,
	partition string,
	idx int,
	override func(*simulator.ConfigGenerator),
) float64 {
	t.Helper()
	var sum float64
	for m := 0; m < nMembers; m++ {
		store := runStubOverride(t, renewablePenetration, numSteps, uint64(3000+m), override)
		sum += finalValue(store, partition, idx)
	}
	return sum / float64(nMembers)
}

// setParam overwrites a single scalar param on a named partition.
func setParam(gen *simulator.ConfigGenerator, partition, key string, value float64) {
	gen.GetPartition(partition).Params.Map[key] = []float64{value}
}

// TestEnergyBalancerExpectedBehaviour is the expected-behaviour suite: each
// subtest name states, in plain language, a response the model is claimed to
// produce, and the body checks it. Together they specify how the model behaves
// for a downstream decision-maker (actionable levers) and why it should be
// trusted off-sample (structural drivers).
func TestEnergyBalancerExpectedBehaviour(t *testing.T) {
	const steps, nMembers, pen = 168, 8, 0.6

	// ----- Decision-path responses (actionable levers a downstream controls) -----

	// Raising the sell bar makes the price battery trade less: fewer settlement
	// periods clear the discharge threshold, so cumulative cycling falls.
	t.Run("higher_discharge_threshold_reduces_price_cycling", func(t *testing.T) {
		base := meanFinalValue(t, pen, steps, nMembers, "price_efc", 0, nil)
		strict := meanFinalValue(t, pen, steps, nMembers, "price_efc", 0, func(g *simulator.ConfigGenerator) {
			setParam(g, "price_dispatch", "price_high", 60.0)
		})
		if !(strict < base) {
			t.Fatalf("expected a higher discharge threshold to reduce price-policy EFC: "+
				"base=%.3f strict(£60)=%.3f", base, strict)
		}
	})

	// A larger battery completes fewer equivalent full cycles for the same market:
	// an EFC is throughput normalised by capacity, so doubling the store roughly
	// halves the cycle count for a given amount of energy moved (the extra headroom
	// from less saturation only partly offsets it). Sizing up is a real downstream
	// lever — larger assets degrade more slowly in cycle terms.
	t.Run("larger_battery_capacity_lowers_cycle_count", func(t *testing.T) {
		base := meanFinalValue(t, pen, steps, nMembers, "price_efc", 0, nil)
		big := meanFinalValue(t, pen, steps, nMembers, "price_efc", 0, func(g *simulator.ConfigGenerator) {
			setParam(g, "price_battery", "energy_capacity_mwh", 400.0)
			setParam(g, "price_efc", "energy_capacity_mwh", 400.0)
		})
		if !(big < base) {
			t.Fatalf("expected doubling capacity to lower the cycle count: "+
				"base=%.3f big(400MWh)=%.3f", base, big)
		}
	})

	// (state, action) → outcome: when the market sits persistently above the
	// discharge threshold, the discharge action fires — the battery ends drained
	// (net seller) and earns positive revenue. A wrong sign here is a wrong trade.
	t.Run("persistently_expensive_market_makes_battery_net_seller", func(t *testing.T) {
		store := runStubOverride(t, pen, steps, 42, func(g *simulator.ConfigGenerator) {
			setParam(g, "residual_demand", "mus", 30000.0) // price = 0.002*30000-10 = £50 > £45
			setParam(g, "residual_demand", "sigmas", 50.0)
			setParam(g, "price_noise", "sigmas", 0.5)
		})
		soc := finalValue(store, "price_battery", 0)
		revenue := finalValue(store, "price_revenue", 0)
		if !(soc < InitialSoCMWh) {
			t.Fatalf("expected a persistently expensive market to drain the battery (net seller): "+
				"final SoC=%.1f, initial=%.1f", soc, InitialSoCMWh)
		}
		if !(revenue > 0) {
			t.Fatalf("expected net selling to earn positive revenue: revenue=£%.0f", revenue)
		}
	})

	// (state, action) → outcome: the mirror image — a persistently cheap market
	// triggers the charge action, so the battery ends full (net buyer) and spends
	// money.
	t.Run("persistently_cheap_market_makes_battery_net_buyer", func(t *testing.T) {
		store := runStubOverride(t, pen, steps, 42, func(g *simulator.ConfigGenerator) {
			setParam(g, "residual_demand", "mus", 15000.0) // price = 0.002*15000-10 = £20 < £25
			setParam(g, "residual_demand", "sigmas", 50.0)
			setParam(g, "price_noise", "sigmas", 0.5)
		})
		soc := finalValue(store, "price_battery", 0)
		revenue := finalValue(store, "price_revenue", 0)
		if !(soc > InitialSoCMWh) {
			t.Fatalf("expected a persistently cheap market to fill the battery (net buyer): "+
				"final SoC=%.1f, initial=%.1f", soc, InitialSoCMWh)
		}
		if !(revenue < 0) {
			t.Fatalf("expected net buying to cost money: revenue=£%.0f", revenue)
		}
	})

	// ----- Structural-driver responses (non-actionable; out-of-sample credibility) -----

	// Physics → economics: a leakier battery keeps less of what it buys, so the
	// same arbitrage pattern earns less. The model was not tuned on efficiency;
	// getting this sign right is a credibility check.
	t.Run("lower_round_trip_efficiency_reduces_revenue", func(t *testing.T) {
		base := meanFinalValue(t, pen, steps, nMembers, "price_revenue", 0, nil)
		leaky := meanFinalValue(t, pen, steps, nMembers, "price_revenue", 0, func(g *simulator.ConfigGenerator) {
			setParam(g, "price_battery", "charge_efficiency", 0.75)
			setParam(g, "price_battery", "discharge_efficiency", 0.75)
		})
		if !(leaky < base) {
			t.Fatalf("expected lower round-trip efficiency to reduce revenue: "+
				"base=£%.0f leaky(0.75)=£%.0f", base, leaky)
		}
	})

	// A second, independent volatility channel: more intra-period price noise (not
	// demand-driven) still carries the price across the thresholds more often, so
	// cycling rises. Distinguishes price noise from the demand-volatility driver.
	t.Run("higher_price_noise_raises_cycling", func(t *testing.T) {
		base := meanFinalValue(t, pen, steps, nMembers, "price_efc", 0, nil)
		noisy := meanFinalValue(t, pen, steps, nMembers, "price_efc", 0, func(g *simulator.ConfigGenerator) {
			setParam(g, "price_noise", "sigmas", 15.0)
		})
		if !(noisy > base) {
			t.Fatalf("expected higher price noise to raise cycling: "+
				"base=%.3f noisy(σ=15)=%.3f", base, noisy)
		}
	})

	// A steeper price response to net load amplifies the same demand swings into
	// bigger price swings, so the price battery cycles more. The intercept is
	// compensated to hold the baseline price at ~£35 (mid-band) — otherwise
	// steepening the slope would also shove the mean price out through a threshold
	// and saturate the battery, the very failure mode the card warns about.
	t.Run("steeper_price_sensitivity_raises_cycling", func(t *testing.T) {
		base := meanFinalValue(t, pen, steps, nMembers, "price_efc", 0, nil)
		steep := meanFinalValue(t, pen, steps, nMembers, "price_efc", 0, func(g *simulator.ConfigGenerator) {
			setParam(g, "price", "demand_slope", 0.004)
			setParam(g, "price", "demand_intercept", 35.0-0.004*DefaultResidualMeanMW) // hold baseline ~£35
		})
		if !(steep > base) {
			t.Fatalf("expected steeper price sensitivity (mean held) to raise cycling: "+
				"base=%.3f steep(slope=0.004)=%.3f", base, steep)
		}
	})

	// The carbon analogue on the carbon-policy chain: a steeper carbon response to
	// net load widens carbon-intensity swings, so the carbon-threshold battery
	// cycles more. Again the intercept is compensated to hold the baseline
	// intensity at ~175 gCO₂/kWh (mid-band).
	t.Run("higher_carbon_sensitivity_raises_carbon_cycling", func(t *testing.T) {
		base := meanFinalValue(t, pen, steps, nMembers, "carbon_efc", 0, nil)
		steep := meanFinalValue(t, pen, steps, nMembers, "carbon_efc", 0, func(g *simulator.ConfigGenerator) {
			setParam(g, "carbon_intensity", "carbon_slope", 0.020)
			setParam(g, "carbon_intensity", "carbon_intercept", 175.0-0.020*DefaultResidualMeanMW) // hold baseline ~175
		})
		if !(steep > base) {
			t.Fatalf("expected steeper carbon sensitivity (mean held) to raise carbon-policy cycling: "+
				"base=%.3f steep(slope=0.020)=%.3f", base, steep)
		}
	})
}
