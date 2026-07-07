package rugby

import (
	"sync"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// runStub runs the stub to completion and returns the recorded state history for
// every partition.
func runStub(t *testing.T, homeSubMinute, numSteps int, seed uint64) *simulator.StateTimeStorage {
	t.Helper()
	settings, implementations := BuildStub(homeSubMinute, numSteps, seed).GenerateConfigs()
	store := simulator.NewStateTimeStorage()
	implementations.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	coordinator := simulator.NewPartitionCoordinator(settings, implementations)
	var wg sync.WaitGroup
	for !coordinator.ReadyToTerminate() {
		coordinator.Step(&wg)
	}
	return store
}

// finalRow returns the last recorded state row of the named partition.
func finalRow(store *simulator.StateTimeStorage, partition string) []float64 {
	rows := store.GetValues(partition)
	return rows[len(rows)-1]
}

// meanHomeTries ensemble-averages the final home-try count over nMembers seeds
// for a given home substitution minute.
func meanHomeTries(t *testing.T, homeSubMinute, numSteps, nMembers int) float64 {
	t.Helper()
	var sum float64
	for m := 0; m < nMembers; m++ {
		store := runStub(t, homeSubMinute, numSteps, uint64(4000+m))
		sum += finalRow(store, "score_events")[0] // [home_try, away_try, home_pen, away_pen]
	}
	return sum / float64(nMembers)
}

func TestRugbyStub(t *testing.T) {
	// Standard convention: the stub must pass the test harness (NaN, state-width,
	// params-mutation, history-integrity and statefulness-residue checks).
	t.Run("harness", func(t *testing.T) {
		settings, implementations := BuildStub(DefaultHomeSubMinute, DefaultNumSteps, 7001).GenerateConfigs()
		if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
			t.Fatalf("harness failed: %v", err)
		}
	})

	// Structural invariants of the generative core: cumulative event counts never
	// decrease, scores are non-negative, conversions never exceed tries, and the
	// half indicator flips from first to second half at minute 40.
	t.Run("invariants", func(t *testing.T) {
		store := runStub(t, DefaultHomeSubMinute, DefaultNumSteps, 7001)

		// Cumulative counting processes are non-decreasing.
		for _, partition := range []string{"score_events", "card_events", "conversion_events"} {
			rows := store.GetValues(partition)
			if len(rows) == 0 {
				t.Fatalf("no %s output", partition)
			}
			for i := 1; i < len(rows); i++ {
				for j := range rows[i] {
					if rows[i][j] < rows[i-1][j]-1e-9 {
						t.Fatalf("%s component %d decreased at step %d: %v → %v",
							partition, j, i, rows[i-1][j], rows[i][j])
					}
				}
			}
		}

		// Scores non-negative; half indicator is 0 in the first half and 1 in the
		// second (match_state records the half derived from cumulative match time).
		matchStates := store.GetValues("match_state")
		for i, s := range matchStates {
			if s[StateIdxHomeScore] < 0 || s[StateIdxAwayScore] < 0 {
				t.Fatalf("step %d: negative score home=%v away=%v", i, s[StateIdxHomeScore], s[StateIdxAwayScore])
			}
			if s[StateIdxHalf] != 0 && s[StateIdxHalf] != 1 {
				t.Fatalf("step %d: half indicator not binary: %v", i, s[StateIdxHalf])
			}
		}
		if last := matchStates[len(matchStates)-1]; last[StateIdxHalf] != 1 {
			t.Fatalf("expected second half at the final step, got half=%v", last[StateIdxHalf])
		}

		// Conversions never exceed the tries that triggered them.
		tries := finalRow(store, "score_events")
		conv := finalRow(store, "conversion_events")
		if conv[0] > tries[0] || conv[1] > tries[1] {
			t.Fatalf("conversions exceed tries: home %v/%v away %v/%v",
				conv[0], tries[0], conv[1], tries[1])
		}
	})

	// Headline generative claim (correct direction of parameter response): an
	// earlier home substitution leaves the "fresh legs" covariate switched on for
	// more of the match, raising the home side's try count. This is the scientific
	// reason the model exists (quantifying the effect of substitution timing) — a
	// stub that merely "runs" would not catch a sign error in the covariate term.
	// Averaged over a 24-member ensemble so the claim is about the distribution.
	t.Run("earlier home substitution raises home tries", func(t *testing.T) {
		const nMembers = 24
		const early, late = 20, 70

		earlyTries := meanHomeTries(t, early, DefaultNumSteps, nMembers)
		lateTries := meanHomeTries(t, late, DefaultNumSteps, nMembers)
		if !(earlyTries > lateTries) {
			t.Fatalf("expected earlier home substitution to raise home tries: "+
				"early(min=%d)=%.3f late(min=%d)=%.3f", early, earlyTries, late, lateTries)
		}
	})
}
