package lob

import (
	"testing"

	"github.com/umbralcalc/stochadex/models/cardgen"
)

// TestLimitOrderBookExpectedBehaviour is the expected-behaviour suite. Each subtest is
// named by a claim ID from ObservedBehaviour and verifies that claim's stated direction
// with cardgen.Verify.
//
// This model is purely structural — like floodrisk, its decision layer lives entirely
// downstream (order placement, execution and the calibration loop are the trading /
// inference application's concern), so the stub has no in-stub actionable lever. The suite
// is instead comprehensive on the structural drivers the world sets: the activity-dependent
// damping brake (the headline), limit-arrival intensity, marketable-order rate and size, and
// quote-churn — one claim per major mechanism of the book.
//
// ObservedBehaviour is the single source of BOTH these assertions and the numbers rendered
// on the card, so the card cannot show a value the test did not observe.
func TestLimitOrderBookExpectedBehaviour(t *testing.T) {
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
