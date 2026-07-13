package homark

import (
	"testing"

	"github.com/umbralcalc/stochadex/models/cardgen"
)

// TestHomarkExpectedBehaviour is the expected-behaviour suite. Each subtest is
// named by a claim ID from ObservedBehaviour and verifies that claim's assertion
// (its stated direction) with cardgen.Verify — covering both the actionable
// planning levers a downstream decision-maker controls (approvals, market-facing
// delivery) and the structural drivers the world sets (policy rates, demand
// pressure, earnings growth, pipeline throughput). Affordability is a
// price-to-earnings ratio, so a *lower* value is *better* affordability.
//
// ObservedBehaviour is the single source of BOTH these assertions and the numbers
// rendered on the card, so the card cannot show a value the test did not observe.
func TestHomarkExpectedBehaviour(t *testing.T) {
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
