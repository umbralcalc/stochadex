package anglersim

import (
	"testing"

	"github.com/umbralcalc/stochadex/models/cardgen"
)

// TestAnglersimExpectedBehaviour is the expected-behaviour suite. Each subtest is
// named by a claim ID from ObservedBehaviour and asserts that the claim's observed
// values move in the direction the claim states (Monotone) — covering both the
// actionable habitat/water-management levers a downstream decision-maker controls
// (flow and dissolved-oxygen management) and the structural drivers the world sets
// (warming, growth, density dependence, process noise, depensation).
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
			assertMonotone(t, c)
		})
	}
}

// assertMonotone checks a claim's observations move strictly in its stated
// direction across the listed points (+1 increasing, -1 decreasing).
func assertMonotone(t *testing.T, c cardgen.Claim) {
	t.Helper()
	if len(c.Observations) < 2 {
		t.Fatalf("claim %q needs at least two observations to test a direction", c.ID)
	}
	if c.Monotone != 1 && c.Monotone != -1 {
		t.Fatalf("claim %q has invalid Monotone %d (want +1 or -1)", c.ID, c.Monotone)
	}
	for i := 1; i < len(c.Observations); i++ {
		prev, cur := c.Observations[i-1], c.Observations[i]
		delta := cur.Value - prev.Value
		if c.Monotone == 1 && !(delta > 0) {
			t.Fatalf("%s: expected increase from %q (%.4f) to %q (%.4f)",
				c.ID, prev.Label, prev.Value, cur.Label, cur.Value)
		}
		if c.Monotone == -1 && !(delta < 0) {
			t.Fatalf("%s: expected decrease from %q (%.4f) to %q (%.4f)",
				c.ID, prev.Label, prev.Value, cur.Label, cur.Value)
		}
	}
}
