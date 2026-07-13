// Package cardgen holds the shared types and rendering for the generated
// "Observed behaviour" block in a model card. A model exposes its verified
// response claims (each a plain-language statement plus the ensemble numbers it
// produces) as []Claim; cmd/model-graphs renders them into card.md between
// generated-block markers, and TestCardsUpToDate guards that the committed cards
// match — so a card's numbers are emitted by the model's own behaviour suite and
// cannot silently drift from the code.
//
// Numbers are rendered rounded (see decimals) so the committed cards are stable
// across architectures: the model iterations are pure-Go float math, whose only
// cross-arch wobble is FMA contraction (~1e-12), far below the rounding boundary.
package cardgen

import (
	"fmt"
	"strings"
)

// decimals is the fixed precision every observed value is rendered to. Coarse
// enough to be architecture-stable, fine enough to show the response.
const decimals = 2

// Observation is one measured point in a claim: a label (the varied input or the
// baseline) and the value the model produced for it.
type Observation struct {
	Label string
	Value float64
}

// Claim is a named, plain-language response claim whose direction is asserted by
// a binding test, carrying the ensemble observations that claim produces.
//
//   - ID matches the binding test's subtest name (the claim↔test bond).
//   - Statement is the human-readable claim rendered on the card.
//   - Unit annotates what the values are (e.g. "ensemble-mean final log-density").
//   - Monotone is the direction the values should move across Observations in
//     order: +1 increasing, -1 decreasing. The binding test asserts exactly this.
type Claim struct {
	ID           string
	Statement    string
	Unit         string
	Monotone     int
	Observations []Observation
}

// FormatValue renders a single value at the fixed card precision.
func FormatValue(v float64) string {
	return fmt.Sprintf("%.*f", decimals, v)
}

// renderObservations joins a claim's points as "label value" separated by " · ".
func renderObservations(obs []Observation) string {
	parts := make([]string, len(obs))
	for i, o := range obs {
		parts[i] = fmt.Sprintf("%s %s", o.Label, FormatValue(o.Value))
	}
	return strings.Join(parts, " · ")
}

// Binding names the test that enforces a model's claims: the test function whose
// subtests are its claim ids, and the source file it lives in (a card-relative
// path, so the link resolves both on GitHub and on the rendered docs site).
type Binding struct {
	TestName string // e.g. "TestAnglersimExpectedBehaviour"
	TestFile string // e.g. "behaviour_test.go"
}

// ObservedBehaviourMarkdown renders the body of the generated block (without the
// markers) for a model's claims. Each row is one bound object — the plain-language
// claim, a link to the exact test/subtest that enforces it, and the number that
// test produced — so claim, test, and result cannot drift apart. Returns "" if
// there are no claims.
func ObservedBehaviourMarkdown(claims []Claim, binding Binding, regenCmd string) string {
	if len(claims) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Observed behaviour\n\n")
	fmt.Fprintf(&b,
		"Every row below is one *bound* object: a plain-language response claim, the "+
			"test subtest that enforces it, and the number that test produced (ensemble "+
			"values rounded to %d dp). Nothing here is hand-written — the claims and their "+
			"numbers are emitted by `%s` (via `%s`), so a claim cannot drift from its test "+
			"or its result. If the model's behaviour changes, either the binding test fails "+
			"(a claim's direction broke) or `TestCardsUpToDate` fails (a number moved) — a "+
			"broken claim cannot reach the card silently.\n\n",
		decimals, binding.TestName, regenCmd,
	)
	b.WriteString("| Response claim | Enforced by | Observed |\n")
	b.WriteString("|---|---|---|\n")
	for _, c := range claims {
		observed := renderObservations(c.Observations)
		if c.Unit != "" {
			observed = fmt.Sprintf("%s — %s", c.Unit, observed)
		}
		test := fmt.Sprintf("[`%s/%s`](%s)", binding.TestName, c.ID, binding.TestFile)
		fmt.Fprintf(&b, "| %s | %s | %s |\n", c.Statement, test, observed)
	}
	return strings.TrimRight(b.String(), "\n")
}
