package solarfleet

import (
	"math"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// TestSolarFleetStub is the engine-CI test, three tiers weakest-to-strongest:
// the harness, physical invariants, and the headline direction-of-response.
func TestSolarFleetStub(t *testing.T) {
	t.Run("harness", func(t *testing.T) {
		settings, implementations := BuildStub(DefaultCloudVolatility, 200, 42).GenerateConfigs()
		if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
			t.Error(err)
		}
	})

	t.Run("invariants", func(t *testing.T) {
		store := runStub(BuildStub, DefaultCloudVolatility, 400, 42, nil)
		sites := store.GetValues("sites")
		fleet := store.GetValues("fleet")
		s := len(sites[0]) / 2
		for step := range sites {
			total := 0.0
			for i := 0; i < s; i++ {
				gen := sites[step][s+i]
				if gen < 0 || math.IsNaN(gen) {
					t.Fatalf("step %d site %d: generation %v is negative or NaN", step, i, gen)
				}
				total += gen
			}
			// The fleet aggregate is exactly the sum of site generation.
			if math.Abs(fleet[step][0]-total) > 1e-9 {
				t.Fatalf("step %d: fleet %v != sum of sites %v", step, fleet[step][0], total)
			}
		}
	})

	t.Run("higher_cloud_volatility_raises_fleet_variability", func(t *testing.T) {
		lo := fleetIndexVariabilityEnsemble(BuildStub, 0.4, 100, nil)
		hi := fleetIndexVariabilityEnsemble(BuildStub, 1.0, 100, nil)
		if !(hi > lo) {
			t.Fatalf("expected higher volatility to raise variability: lo(sigma=0.4)=%v hi(sigma=1.0)=%v", lo, hi)
		}
	})
}
