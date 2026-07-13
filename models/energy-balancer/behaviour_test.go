package energybalancer

import (
	"testing"

	"github.com/umbralcalc/stochadex/models/cardgen"
)

// TestEnergyBalancerExpectedBehaviour is the expected-behaviour suite. Each subtest
// is named by a claim ID from ObservedBehaviour and verifies that claim's assertion
// — together they specify how the model behaves for a downstream decision-maker
// (actionable levers: dispatch threshold, sizing, and the net-seller/net-buyer sign
// of a (state,action)→outcome trade) and why it should be trusted off-sample
// (structural drivers: efficiency, price/carbon sensitivity, noise).
//
// ObservedBehaviour is the single source of BOTH these assertions and the numbers
// rendered on the card, so the card cannot show a value the test did not observe.
func TestEnergyBalancerExpectedBehaviour(t *testing.T) {
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
