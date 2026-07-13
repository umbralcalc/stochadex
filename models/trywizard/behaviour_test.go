package rugby

import (
	"testing"

	"github.com/umbralcalc/stochadex/models/cardgen"
)

// TestRugbyExpectedBehaviour is the expected-behaviour suite. Each subtest is named
// by a claim ID from ObservedBehaviour and verifies that claim's assertion (its
// stated direction) with cardgen.Verify — covering both the actionable substitution
// levers a downstream decision-maker controls (substitution timing and fullness for
// either side) and the structural drivers the world sets (try intercept, conversion
// probability, substitution effect size, the home-advantage edge, and the card rate).
//
// ObservedBehaviour is the single source of BOTH these assertions and the numbers
// rendered on the card, so the card cannot show a value the test did not observe.
func TestRugbyExpectedBehaviour(t *testing.T) {
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
