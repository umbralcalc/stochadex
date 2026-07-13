package homark

import (
	"sync"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// The override run helper (runStubOverride), the param setter (setParam), and the
// metric helpers (finalAffordability, meanFinalAff, meanPipelineStock) live in
// behaviour.go so they can be shared with the card generator; the tests below
// exercise the stub through the local runStub helper.

// runStub runs the stub to completion and returns the recorded state history for
// every partition, keyed by partition name.
func runStub(approvalRate float64, numSteps int, seed uint64) *simulator.StateTimeStorage {
	settings, implementations := BuildStub(approvalRate, numSteps, seed).GenerateConfigs()
	store := simulator.NewStateTimeStorage()
	implementations.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	coordinator := simulator.NewPartitionCoordinator(settings, implementations)
	var wg sync.WaitGroup
	for !coordinator.ReadyToTerminate() {
		coordinator.Step(&wg)
	}
	return store
}

// meanFinalAffordability ensemble-averages the final affordability ratio across
// independent realisations (varying only the seed) to damp single-run noise.
func meanFinalAffordability(approvalRate float64, numSteps, nMembers int) float64 {
	var sum float64
	for m := 0; m < nMembers; m++ {
		sum += finalAffordability(runStub(approvalRate, numSteps, uint64(2000+m)))
	}
	return sum / float64(nMembers)
}

func TestHomarkStub(t *testing.T) {
	// Standard convention: the stub must pass the test harness (NaN, state-width,
	// params-mutation, history-integrity and statefulness-residue checks).
	t.Run("harness", func(t *testing.T) {
		settings, implementations := BuildStub(DefaultApprovalRate, 48, 42).GenerateConfigs()
		if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
			t.Fatalf("harness failed: %v", err)
		}
	})

	// Structural / physical invariants of the generative core.
	t.Run("invariants", func(t *testing.T) {
		store := runStub(DefaultApprovalRate, DefaultNumSteps, 42)

		// Affordability (a ratio of exponentials) and the pipeline stock (a count of
		// units) are non-negative every step.
		for i, row := range store.GetValues("affordability") {
			if row[0] < 0 {
				t.Fatalf("step %d: negative affordability %v", i, row[0])
			}
		}
		for i, row := range store.GetValues("pipeline") {
			if row[0] < 0 {
				t.Fatalf("step %d: negative pipeline stock %v", i, row[0])
			}
		}

		// Pipeline conservation: the stock cannot rise by more than the monthly
		// approval inflow (completions and attrition only ever remove units).
		pipe := store.GetValues("pipeline")
		for i := 1; i < len(pipe); i++ {
			if pipe[i][0] > pipe[i-1][0]+DefaultApprovalRate+1e-9 {
				t.Fatalf("step %d: pipeline stock rose by more than the approval inflow: %v -> %v",
					i, pipe[i-1][0], pipe[i][0])
			}
		}
	})

	// Headline generative claim (correct direction of parameter response): more
	// planning approvals build a larger market-facing committed pipeline, whose
	// anticipated supply dampens the log-price drift, so the price-to-earnings ratio
	// ends lower — i.e. affordability improves. This is the reason the model exists
	// (supply policy → affordability); a stub that merely "runs" would not catch an
	// inverted supply response. Ensemble-averaged so the claim is about the
	// distribution, not one noisy realisation. (The full set of response claims, with
	// their observed numbers, is in behaviour_test.go via ObservedBehaviour.)
	t.Run("more approvals improve affordability", func(t *testing.T) {
		const numSteps, nMembers = DefaultNumSteps, 12
		lowSupply := meanFinalAffordability(40.0, numSteps, nMembers)
		highSupply := meanFinalAffordability(250.0, numSteps, nMembers)
		if !(highSupply < lowSupply) {
			t.Fatalf("expected more approvals to lower the price-to-earnings ratio: "+
				"low(40)=%.3f high(250)=%.3f", lowSupply, highSupply)
		}
	})
}
