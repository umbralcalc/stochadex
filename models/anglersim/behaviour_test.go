package anglersim

import (
	"testing"

	"github.com/umbralcalc/stochadex/models/cardgen"
)

// TestAnglersimExpectedBehaviour is the expected-behaviour suite. Each subtest is
// named by a claim ID from ObservedBehaviour and verifies that claim's assertion
// (its stated direction) with cardgen.Verify — covering both the actionable
// habitat/water-management levers a downstream decision-maker controls (flow and
// dissolved-oxygen management) and the structural drivers the world sets (warming,
// growth, density dependence, process noise, depensation).
//
// ObservedBehaviour is the single source of BOTH these assertions and the numbers
// rendered on the card, so the card cannot show a value the test did not observe.
func TestAnglersimExpectedBehaviour(t *testing.T) {
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
