package anglersim

import (
	"math"
	"sync"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// runStubOverride runs the stub with an override hook applied to the generated
// config before the run, so a behaviour test can vary any partition's params
// without bloating BuildStub's signature — BuildStub still exposes only the one
// headline driver (the warming trend).
func runStubOverride(
	t *testing.T,
	numSteps int,
	seed uint64,
	override func(*simulator.ConfigGenerator),
) *simulator.StateTimeStorage {
	t.Helper()
	gen := BuildStub(DefaultWarmingTrend, numSteps, seed)
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

// meanFinalDensityOverride returns the ensemble-mean final log-density under an
// override, averaging over both the final-window and an ensemble of seeds.
func meanFinalDensityOverride(
	t *testing.T,
	numSteps, window, nMembers int,
	override func(*simulator.ConfigGenerator),
) float64 {
	t.Helper()
	sum := 0.0
	for m := 0; m < nMembers; m++ {
		store := runStubOverride(t, numSteps, uint64(4000+m), override)
		sum += meanFinalLogDensity(store.GetValues("population"), window)
	}
	return sum / float64(nMembers)
}

// stdFinalDensityOverride returns the across-ensemble standard deviation of the
// final log-density under an override — the spread of outcomes, used to test the
// process-noise claim.
func stdFinalDensityOverride(
	t *testing.T,
	numSteps, nMembers int,
	override func(*simulator.ConfigGenerator),
) float64 {
	t.Helper()
	finals := make([]float64, nMembers)
	mean := 0.0
	for m := 0; m < nMembers; m++ {
		store := runStubOverride(t, numSteps, uint64(7000+m), override)
		rows := store.GetValues("population")
		finals[m] = rows[len(rows)-1][0]
		mean += finals[m]
	}
	mean /= float64(nMembers)
	varSum := 0.0
	for _, v := range finals {
		d := v - mean
		varSum += d * d
	}
	return math.Sqrt(varSum / float64(nMembers))
}

// setCovariateBaseline overwrites one entry of the covariate baseline vector.
func setCovariateBaseline(gen *simulator.ConfigGenerator, index int, value float64) {
	b := gen.GetPartition("covariates").Params.Map["baseline_levels"]
	c := make([]float64, len(b))
	copy(c, b)
	c[index] = value
	gen.GetPartition("covariates").Params.Map["baseline_levels"] = c
}

// setPopulationParam overwrites a scalar param on the population partition.
func setPopulationParam(gen *simulator.ConfigGenerator, key string, value float64) {
	gen.GetPartition("population").Params.Map[key] = []float64{value}
}

// TestAnglersimExpectedBehaviour is the expected-behaviour suite: each subtest
// name is a plain-language response claim, and the body varies one input and
// asserts the output moves as the name says. It covers both the actionable
// habitat/water-management levers a downstream decision-maker controls (flow and
// dissolved-oxygen management, mapping to the downstream abstraction / drought /
// water-quality scenarios) and the structural drivers the world sets (climate
// warming, growth, density dependence, process noise, depensation) whose correct
// signs earn the model out-of-sample credibility.
func TestAnglersimExpectedBehaviour(t *testing.T) {
	// --- Decision-path responses (actionable habitat / water-management levers) ---

	// Reduced water abstraction raises mean river flow; flow helps trout
	// (beta_flow > 0), so density rises.
	t.Run("reduced_abstraction_higher_flow_raises_density", func(t *testing.T) {
		const steps, window, nMembers = 60, 20, 12
		base := meanFinalDensityOverride(t, steps, window, nMembers, nil)
		higherFlow := meanFinalDensityOverride(t, steps, window, nMembers, func(g *simulator.ConfigGenerator) {
			setCovariateBaseline(g, 0, 2.0*DefaultBaselineFlow)
		})
		if !(higherFlow > base) {
			t.Fatalf("expected higher river flow to raise density: base=%.4f higherFlow=%.4f", base, higherFlow)
		}
	})

	// Drought cuts river flow; less flow lowers trout density.
	t.Run("drought_lower_flow_reduces_density", func(t *testing.T) {
		const steps, window, nMembers = 60, 20, 12
		base := meanFinalDensityOverride(t, steps, window, nMembers, nil)
		drought := meanFinalDensityOverride(t, steps, window, nMembers, func(g *simulator.ConfigGenerator) {
			setCovariateBaseline(g, 0, 0.25*DefaultBaselineFlow)
		})
		if !(drought < base) {
			t.Fatalf("expected drought (lower flow) to reduce density: base=%.4f drought=%.4f", base, drought)
		}
	})

	// Pollution reduction raises dissolved oxygen; oxygen helps trout
	// (beta_do > 0), so density rises.
	t.Run("water_quality_improvement_higher_dissolved_oxygen_raises_density", func(t *testing.T) {
		const steps, window, nMembers = 60, 20, 12
		base := meanFinalDensityOverride(t, steps, window, nMembers, nil)
		cleaner := meanFinalDensityOverride(t, steps, window, nMembers, func(g *simulator.ConfigGenerator) {
			setCovariateBaseline(g, 2, DefaultBaselineDO+3.0)
		})
		if !(cleaner > base) {
			t.Fatalf("expected higher dissolved oxygen to raise density: base=%.4f cleaner=%.4f", base, cleaner)
		}
	})

	// --- Structural-driver responses (non-actionable levers the world sets) ---

	// Climate warming raises water temperature, which hurts trout
	// (beta_temp < 0), so density falls. (Also the headline claim.)
	t.Run("climate_warming_reduces_density", func(t *testing.T) {
		const steps, window, nMembers = 80, 20, 16
		base := meanFinalDensityOverride(t, steps, window, nMembers, nil)
		warmed := meanFinalDensityOverride(t, steps, window, nMembers, func(g *simulator.ConfigGenerator) {
			g.GetPartition("covariates").Params.Map["warming_trend"] = []float64{0.08}
		})
		if !(warmed < base) {
			t.Fatalf("expected warming to reduce density: base=%.4f warmed=%.4f", base, warmed)
		}
	})

	// A higher intrinsic growth rate raises the Ricker equilibrium density.
	t.Run("higher_growth_rate_raises_density", func(t *testing.T) {
		const steps, window, nMembers = 60, 20, 12
		base := meanFinalDensityOverride(t, steps, window, nMembers, nil)
		faster := meanFinalDensityOverride(t, steps, window, nMembers, func(g *simulator.ConfigGenerator) {
			setPopulationParam(g, "growth_rate", 1.0)
		})
		if !(faster > base) {
			t.Fatalf("expected higher growth rate to raise density: base=%.4f faster=%.4f", base, faster)
		}
	})

	// Stronger density-dependent mortality lowers the equilibrium density
	// (N* = (r0 + env)/alpha falls as alpha rises).
	t.Run("stronger_density_dependence_reduces_density", func(t *testing.T) {
		const steps, window, nMembers = 60, 20, 12
		base := meanFinalDensityOverride(t, steps, window, nMembers, nil)
		crowded := meanFinalDensityOverride(t, steps, window, nMembers, func(g *simulator.ConfigGenerator) {
			setPopulationParam(g, "density_dependence", 2.0)
		})
		if !(crowded < base) {
			t.Fatalf("expected stronger density dependence to reduce density: base=%.4f crowded=%.4f", base, crowded)
		}
	})

	// Higher process noise widens the distribution of outcomes (larger spread of
	// final densities across realisations), even where the mean is little changed.
	t.Run("higher_process_noise_widens_density_distribution", func(t *testing.T) {
		const steps, nMembers = 60, 40
		lowNoise := stdFinalDensityOverride(t, steps, nMembers, func(g *simulator.ConfigGenerator) {
			setPopulationParam(g, "process_noise_sd", 0.05)
		})
		highNoise := stdFinalDensityOverride(t, steps, nMembers, func(g *simulator.ConfigGenerator) {
			setPopulationParam(g, "process_noise_sd", 0.6)
		})
		if !(highNoise > lowNoise) {
			t.Fatalf("expected higher process noise to widen the density spread: "+
				"lowNoise std=%.4f highNoise std=%.4f", lowNoise, highNoise)
		}
	})

	// The Allee effect (depensation) suppresses the growth term at low density, so
	// a population starting far below equilibrium recovers more slowly than under
	// the standard Ricker (gamma = 0). Measured over a short horizon from a low
	// seed density, with near-zero noise so the mechanism is isolated.
	t.Run("allee_effect_slows_recovery_from_low_density", func(t *testing.T) {
		const steps, window, nMembers = 8, 1, 8
		lowStart := func(g *simulator.ConfigGenerator) {
			setPopulationParam(g, "process_noise_sd", 0.001)
			g.GetPartition("population").InitStateValues = []float64{-8.0}
		}
		noAllee := meanFinalDensityOverride(t, steps, window, nMembers, func(g *simulator.ConfigGenerator) {
			lowStart(g)
			setPopulationParam(g, "allee_effect", 0.0)
		})
		withAllee := meanFinalDensityOverride(t, steps, window, nMembers, func(g *simulator.ConfigGenerator) {
			lowStart(g)
			setPopulationParam(g, "allee_effect", 30.0)
		})
		if !(withAllee < noAllee) {
			t.Fatalf("expected the Allee effect to slow recovery from low density: "+
				"noAllee=%.4f withAllee=%.4f", noAllee, withAllee)
		}
	})
}
