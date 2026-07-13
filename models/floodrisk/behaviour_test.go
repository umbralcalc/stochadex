package floodrisk

import (
	"testing"

	"github.com/umbralcalc/stochadex/models/cardgen"
)

// TestFloodRiskExpectedBehaviour is the expected-behaviour suite. This model is
// PURELY STRUCTURAL: its decision layer (natural flood management interventions)
// lives entirely in the downstream repo, so the stub has no actionable in-stub
// lever — and the suite is therefore comprehensive on the structural drivers of the
// flood peak instead. Each subtest is named by a claim ID from ObservedBehaviour
// and verifies that claim's assertion; getting these signs right is what makes the
// model credible off-sample.
//
// ObservedBehaviour is the single source of BOTH these assertions and the numbers
// rendered on the card, so the card cannot show a value the test did not observe.
func TestFloodRiskExpectedBehaviour(t *testing.T) {
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
