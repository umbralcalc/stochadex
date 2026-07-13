package amr

import (
	"testing"

	"github.com/umbralcalc/stochadex/models/cardgen"
)

// TestAMRExpectedBehaviour is the expected-behaviour suite. Each subtest is named
// by a claim ID from ObservedBehaviour and verifies that claim's assertion —
// together they specify how the stewardship lever acts (decision path: prescribing,
// and the causal claim that it acts only through selection) and why the model should
// transfer off-sample (structural drivers).
//
// ObservedBehaviour is the single source of BOTH these assertions and the numbers
// rendered on the card, so the card cannot show a value the test did not observe.
func TestAMRExpectedBehaviour(t *testing.T) {
	claims := ObservedBehaviour()
	if len(claims) == 0 {
		t.Fatal("ObservedBehaviour returned no claims")
	}
	for _, c := range claims {
		t.Run(c.ID, func(t *testing.T) {
			if err := cardgen.Verify(c); err != nil {
				t.Fatal(err)
			}
		})
	}
}
