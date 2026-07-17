package measles

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// The run/ensemble helpers (runStub, totalCases, meanTotalCases, …) live in the
// non-test behaviour.go so the behaviour suite and the card share one definition of
// what each number means; this file uses them for the harness/invariant/headline
// checks.

func TestMeaslesStub(t *testing.T) {
	// Standard convention: the stub must pass the test harness (NaN, state-width,
	// params-mutation, history-integrity and statefulness-residue checks).
	t.Run("harness", func(t *testing.T) {
		settings, implementations := BuildStub(DefaultMMR2Coverage, DefaultMaxGenerations, 42).GenerateConfigs()
		if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
			t.Fatalf("harness failed: %v", err)
		}
	})

	// Structural / physical invariants of the generative core.
	t.Run("invariants", func(t *testing.T) {
		const cov = DefaultMMR2Coverage
		_, _, pool := BuildUTLASurface(cov)
		n := len(pool)
		store := runStub(BuildStub, cov, DefaultMaxGenerations, 42)
		rows := store.GetValues("outbreaks")

		var prevCumulative []float64
		for step, row := range rows {
			for i := 0; i < n; i++ {
				infectious, cumulative := row[i], row[n+i]
				// Non-negativity of both current generation size and running total.
				if infectious < -1e-9 || cumulative < -1e-9 {
					t.Fatalf("step %d UTLA %d: negative state (infectious=%v cumulative=%v)",
						step, i, infectious, cumulative)
				}
				// Susceptible depletion: an outbreak never exceeds its reachable pool.
				if cumulative > pool[i]+1e-6 {
					t.Fatalf("step %d UTLA %d: cumulative %v exceeds susceptible pool %v",
						step, i, cumulative, pool[i])
				}
				// Cumulative cases are monotonically non-decreasing.
				if prevCumulative != nil && cumulative < prevCumulative[i]-1e-9 {
					t.Fatalf("step %d UTLA %d: cumulative decreased %v -> %v",
						step, i, prevCumulative[i], cumulative)
				}
			}
			prevCumulative = append(prevCumulative[:0], row[n:]...)
		}

		// The shared national importation total M is drawn once (at generation 0, the
		// second recorded row — the first is the pre-simulation init state) and held
		// constant across generations, and stays inside its log-uniform band.
		national := store.GetValues("national_importation")
		m0 := national[1][0]
		if m0 < DefaultSeedLow-1e-6 || m0 > DefaultSeedHigh+1e-6 {
			t.Fatalf("national seed total %v outside band [%v, %v]", m0, DefaultSeedLow, DefaultSeedHigh)
		}
		for step := 1; step < len(national); step++ {
			if math.Abs(national[step][0]-m0) > 1e-9 {
				t.Fatalf("step %d: national seed total drifted %v -> %v", step, m0, national[step][0])
			}
		}
	})

	// The lifted branching kernel matches theory: a subcritical process seeded by one
	// case has analytic mean total progeny 1/(1-m). This guards the O(1) Gamma–Poisson
	// generation step (nextGeneration) that both bespoke iterations share.
	t.Run("kernel matches analytic mean total progeny", func(t *testing.T) {
		const m, dispersion, nSims = 0.6, 0.5, 40000
		rng := rand.New(rand.NewPCG(7, 7))
		var total float64
		for s := 0; s < nSims; s++ {
			size, infectious := 1, 1 // index case
			for infectious > 0 && size < 100000 {
				next := nextGeneration(infectious, m, dispersion, rng)
				size += next
				infectious = next
			}
			total += float64(size)
		}
		got := total / nSims
		want := expectedTotalProgeny(m) // 1/(1-0.6) = 2.5
		if math.Abs(got-want) > 0.1 {
			t.Fatalf("mean total progeny %.3f, want %.3f (1/(1-m))", got, want)
		}
	})

	// Headline generative claim (correct direction of parameter response): lower MMR
	// coverage raises effective susceptibility, pushes R_local = R0·s above 1 in more
	// areas, and so raises the total simulated case count. This is the reason the model
	// exists (coverage → transmission risk); a stub that merely "runs" would not catch
	// an inverted coverage response. Ensemble-averaged over shared-importation
	// scenarios so the claim is about the distribution.
	t.Run("lower coverage raises total cases", func(t *testing.T) {
		const gens, nScenarios = DefaultMaxGenerations, 16
		lowCoverage := meanTotalCases(BuildStub, 0.80, gens, nScenarios)
		highCoverage := meanTotalCases(BuildStub, 0.94, gens, nScenarios)
		if !(lowCoverage > highCoverage) {
			t.Fatalf("expected lower coverage to raise total cases: "+
				"low(0.80)=%.0f high(0.94)=%.0f", lowCoverage, highCoverage)
		}
	})
}
