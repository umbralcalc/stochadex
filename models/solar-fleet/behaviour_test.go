package solarfleet

// The expected-behaviour suite: one subtest per named response claim, each asserting
// the model's output moves the way the claim's name says. The subtest name is the
// claim id, so this file reads as a behavioural specification of the model. This
// entry is purely structural (its decision layer lives downstream), so every claim
// is a structural-driver response; there are no in-stub actionable levers.

import (
	"testing"

	"github.com/umbralcalc/stochadex/models/cardgen"
)

func TestSolarFleetExpectedBehaviour(t *testing.T) {
	claims := ObservedBehaviour()
	if len(claims) == 0 {
		t.Fatal("ObservedBehaviour returned no claims")
	}
	for _, c := range claims {
		t.Run(c.ID, func(t *testing.T) {
			if err := cardgen.Verify(c); err != nil {
				t.Error(err)
			}
		})
	}
}
