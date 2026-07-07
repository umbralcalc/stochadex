package floodrisk

import (
	"sync"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// runStubOverride runs the stub with an override hook applied to the generated
// config before the run, so a behaviour test can vary any partition's params
// without bloating BuildStub's signature — BuildStub still exposes only the one
// headline driver (the rainfall multiplier).
func runStubOverride(
	t *testing.T,
	rainfallMultiplier float64,
	numSteps int,
	seed uint64,
	override func(*simulator.ConfigGenerator),
) *simulator.StateTimeStorage {
	t.Helper()
	gen := BuildStub(rainfallMultiplier, numSteps, seed)
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

// meanPeakFlowOverride averages peak flow across an ensemble of independent
// realisations (varying the rainfall seed) under a fixed override, damping the
// sampling noise in a single run.
func meanPeakFlowOverride(
	t *testing.T,
	rainfallMultiplier float64,
	numSteps, nMembers, spinUp int,
	override func(*simulator.ConfigGenerator),
) float64 {
	t.Helper()
	var sum float64
	for m := 0; m < nMembers; m++ {
		store := runStubOverride(t, rainfallMultiplier, numSteps, uint64(5000+m), override)
		sum += peakFlow(store.GetValues("runoff"), spinUp)
	}
	return sum / float64(nMembers)
}

// setRunoffParam / setRainfallParam overwrite a single scalar param on the runoff
// or rainfall partition.
func setRunoffParam(gen *simulator.ConfigGenerator, key string, value float64) {
	gen.GetPartition("runoff").Params.Map[key] = []float64{value}
}
func setRainfallParam(gen *simulator.ConfigGenerator, key string, value float64) {
	gen.GetPartition("rainfall").Params.Map[key] = []float64{value}
}

// TestFloodRiskExpectedBehaviour is the expected-behaviour suite. This model is
// PURELY STRUCTURAL: its decision layer (natural flood management interventions)
// lives entirely in the downstream repo, so the stub has no actionable in-stub
// lever — and the suite is therefore comprehensive on the structural drivers of
// the flood peak instead. Each subtest name states a claimed response, checked
// here; getting these signs right is what makes the model credible off-sample.
func TestFloodRiskExpectedBehaviour(t *testing.T) {
	const steps, nMembers, spinUp = 730, 8, 60

	// More persistent wet spells (higher wet→wet transition probability) pile up
	// consecutive rain days, saturating the catchment and raising the flood peak.
	t.Run("higher_wet_persistence_raises_flow", func(t *testing.T) {
		base := meanPeakFlowOverride(t, 1.0, steps, nMembers, spinUp, nil)
		persistent := meanPeakFlowOverride(t, 1.0, steps, nMembers, spinUp, func(g *simulator.ConfigGenerator) {
			setRainfallParam(g, "p_wet_given_wet", 0.95)
		})
		if !(persistent > base) {
			t.Fatalf("expected higher wet-day persistence to raise peak flow: "+
				"base=%.2f persistent(0.95)=%.2f m3/s", base, persistent)
		}
	})

	// Higher evapotranspiration removes more water before it can run off, lowering
	// net rainfall and so the flood peak.
	t.Run("higher_evapotranspiration_lowers_flow", func(t *testing.T) {
		base := meanPeakFlowOverride(t, 1.0, steps, nMembers, spinUp, nil)
		thirsty := meanPeakFlowOverride(t, 1.0, steps, nMembers, spinUp, func(g *simulator.ConfigGenerator) {
			setRunoffParam(g, "et_rate", 5.0)
		})
		if !(thirsty < base) {
			t.Fatalf("expected higher evapotranspiration to lower peak flow: "+
				"base=%.2f thirsty(5.0)=%.2f m3/s", base, thirsty)
		}
	})

	// Flow scales with catchment area through the mm→m³/s conversion: a larger
	// catchment collecting the same rainfall depth produces a larger peak flow.
	t.Run("larger_catchment_area_raises_flow", func(t *testing.T) {
		base := meanPeakFlowOverride(t, 1.0, steps, nMembers, spinUp, nil)
		bigger := meanPeakFlowOverride(t, 1.0, steps, nMembers, spinUp, func(g *simulator.ConfigGenerator) {
			setRunoffParam(g, "catchment_area_km2", 2.0*DefaultCatchmentAreaKm2)
		})
		if !(bigger > base) {
			t.Fatalf("expected a larger catchment area to raise peak flow: "+
				"base=%.2f bigger(2x)=%.2f m3/s", base, bigger)
		}
	})

	// The closest thing to an intervention the stub can express (real NFM lives
	// downstream): a greater soil-storage capacity buffers more rainfall before the
	// nonlinear runoff response kicks in, lowering the flood peak. This is the
	// structural basis for why "make room for water" catchment measures work.
	t.Run("higher_field_capacity_lowers_peak_flow", func(t *testing.T) {
		base := meanPeakFlowOverride(t, 1.0, steps, nMembers, spinUp, nil)
		spongy := meanPeakFlowOverride(t, 1.0, steps, nMembers, spinUp, func(g *simulator.ConfigGenerator) {
			setRunoffParam(g, "field_capacity", 700.0)
		})
		if !(spongy < base) {
			t.Fatalf("expected higher soil-storage capacity to lower peak flow: "+
				"base=%.2f spongy(700mm)=%.2f m3/s", base, spongy)
		}
	})
}
