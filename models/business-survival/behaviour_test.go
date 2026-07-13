package bizsurvival

import (
	"testing"

	"github.com/umbralcalc/stochadex/models/cardgen"
)

// TestBusinessSurvivalExpectedBehaviour is the expected-behaviour suite. Each
// subtest is named by a claim ID from ObservedBehaviour and verifies that claim's
// assertion (its stated direction) with cardgen.Verify — covering both the
// actionable support levers a downstream decision-maker controls (headline hazard
// support, formation support, first-year relief, sector targeting) and the
// structural drivers the world sets (baseline demography, both macro channels, the
// distress channel, and sector heterogeneity). The runs are all in deterministic
// mean-field mode so each signed effect is exact.
//
// ObservedBehaviour is the single source of BOTH these assertions and the numbers
// rendered on the card, so the card cannot show a value the test did not observe.
func TestBusinessSurvivalExpectedBehaviour(t *testing.T) {
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
