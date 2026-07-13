package measles

import (
	"testing"

	"github.com/umbralcalc/stochadex/models/cardgen"
)

// TestMeaslesExpectedBehaviour is the expected-behaviour suite. Each subtest is
// named by a claim ID from ObservedBehaviour and verifies that claim's assertion
// (its stated direction) with cardgen.Verify. This model is a transmission-*risk
// surface* — its one actionable public-health lever is vaccine coverage (the
// decision-path claim); everything else is a structural driver the world sets
// (susceptibility gradient, R0, importation pressure, the shared-latent joint tail)
// whose correct sign earns out-of-sample credibility. The targeting/ranking
// decision layer lives downstream.
//
// ObservedBehaviour is the single source of BOTH these assertions and the numbers
// rendered on the card, so the card cannot show a value the test did not observe.
func TestMeaslesExpectedBehaviour(t *testing.T) {
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
