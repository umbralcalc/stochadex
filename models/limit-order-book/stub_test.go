package lob

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// The run and ensemble helpers live in behaviour.go so they can be shared with the card
// generator; the tests below exercise them.

func TestLimitOrderBookStub(t *testing.T) {
	// Standard convention: the stub must pass the test harness (NaN, state-width,
	// params-mutation, history-integrity and statefulness-residue checks).
	t.Run("harness", func(t *testing.T) {
		settings, implementations := BuildStub(DefaultDampingGamma, 200, 42).GenerateConfigs()
		if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
			t.Fatalf("harness failed: %v", err)
		}
	})

	// Structural / physical invariants of the generative core, every step.
	t.Run("invariants", func(t *testing.T) {
		store := runOverride(BuildStub, 400, 42, nil)

		flows := store.GetValues("flows")
		if len(flows) == 0 {
			t.Fatal("no flows output")
		}
		for i, row := range flows {
			// Arrivals, cancellations, order legs and the clip count are all non-negative.
			for j, v := range row {
				if v < 0 {
					t.Fatalf("step %d: negative flow component %d = %v", i, j, v)
				}
			}
			// The cancellation-clip count cannot exceed the number of level-sides.
			if clip := row[4*Levels+2]; clip > 2*Levels {
				t.Fatalf("step %d: clip_binds %v exceeds %d", i, clip, 2*Levels)
			}
		}

		book := store.GetValues("book")
		for i, row := range book {
			// Resting depth stays non-negative on every level of both sides.
			for j := 0; j < 2*Levels; j++ {
				if row[j] < 0 {
					t.Fatalf("step %d: negative resting depth at slot %d = %v", i, j, row[j])
				}
			}
			// Swept volume is non-negative.
			if row[2*Levels] < 0 {
				t.Fatalf("step %d: negative swept volume %v", i, row[2*Levels])
			}
		}

		obs := store.GetValues("observables")
		for i, row := range obs {
			nLimit, nCancel, nMarket, depth, spread, clip :=
				row[0], row[1], row[2], row[3], row[4], row[5]
			if nLimit < 0 || nCancel < 0 || nMarket < 0 || depth < 0 || clip < 0 {
				t.Fatalf("step %d: negative observable in %v", i, row)
			}
			// The spread is strictly positive: a live two-sided book gives ≥2 ticks,
			// and a one-sided book reports the positive empty-book sentinel.
			if spread <= 0 {
				t.Fatalf("step %d: non-positive spread %v", i, spread)
			}
			if spread > DefaultEmptySpread {
				t.Fatalf("step %d: spread %v exceeds the one-sided sentinel", i, spread)
			}
		}
	})

	// Headline generative claim (correct direction of parameter response): stronger
	// activity-dependent damping drives the depth/arrival correlation more negative. This
	// is the reason the model exists — the stability brake the downstream calibrates — and
	// a stub that merely "runs" would not catch an inverted damping response. (The full set
	// of response claims, with their observed numbers, is in behaviour_test.go via
	// ObservedBehaviour.)
	t.Run("stronger damping lowers the depth-arrival correlation", func(t *testing.T) {
		weak := corrDepthArrivalsEnsemble(BuildStub, 0.0, 7000)
		strong := corrDepthArrivalsEnsemble(BuildStub, 0.9, 7000)
		if !(strong < weak) {
			t.Fatalf("expected stronger damping to lower corr(depth, arrivals): "+
				"γ=0.0 → %.4f, γ=0.9 → %.4f", weak, strong)
		}
	})
}
