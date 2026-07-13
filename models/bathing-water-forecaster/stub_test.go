package bathingwater

import (
	"math"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestBathingWaterStub(t *testing.T) {
	// Standard convention: the stub must pass the test harness (NaN, state-width,
	// params-mutation, history-integrity and statefulness-residue checks).
	t.Run("harness", func(t *testing.T) {
		settings, implementations := BuildStub(DefaultAnomalyVolatility, 60, 42).GenerateConfigs()
		if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
			t.Fatalf("harness failed: %v", err)
		}
	})

	// Structural / physical invariants of the generative core.
	t.Run("invariants", func(t *testing.T) {
		store := runStub(DefaultAnomalyVolatility, 400, 42)

		anomaly := store.GetValues("anomaly")
		if len(anomaly) == 0 {
			t.Fatal("no anomaly output")
		}
		for i, row := range anomaly {
			if math.IsNaN(row[0]) || math.IsInf(row[0], 0) {
				t.Fatalf("step %d: non-finite anomaly %v", i, row[0])
			}
		}

		for _, s := range DefaultSites {
			rows := store.GetValues(s.name)
			for i, row := range rows {
				mu, p := row[0], row[1]
				// Latent log-concentration stays finite.
				if math.IsNaN(mu) || math.IsInf(mu, 0) {
					t.Fatalf("%s step %d: non-finite mu %v", s.name, i, mu)
				}
				// Exceedance probability is a genuine probability.
				if p < 0 || p > 1 {
					t.Fatalf("%s step %d: p_exceed %v outside [0,1]", s.name, i, p)
				}
			}
		}
	})

	// Headline generative claim (correct direction of parameter response): a more
	// volatile shared regional anomaly (higher OU volatility) raises the mean
	// exceedance probability at a site that normally sits below the threshold —
	// bigger wet-week swings push the rare tail up more often than the (bounded)
	// dry side pulls it down. This is the mechanism the whole project rests on; a
	// stub that merely "runs" would not catch an inverted response. Ensemble-averaged.
	t.Run("more volatile anomaly raises mean exceedance", func(t *testing.T) {
		const numSteps, nMembers = 400, 16

		calm := meanPExceedEnsemble(0.3, numSteps, nMembers, "site_0")
		stormy := meanPExceedEnsemble(0.8, numSteps, nMembers, "site_0")

		if !(stormy > calm) {
			t.Fatalf("expected a more volatile anomaly to raise mean exceedance: "+
				"calm(0.3)=%.4f stormy(0.8)=%.4f", calm, stormy)
		}
	})
}
