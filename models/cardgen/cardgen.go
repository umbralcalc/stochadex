// Package cardgen holds the shared types, verification, and rendering for the
// generated "Observed behaviour" block in a model card. A model exposes its
// verified response claims (each a plain-language statement, the ensemble numbers
// it produces, and how those numbers are asserted) as []Claim; its behaviour test
// verifies every claim with Verify, and cmd/model-graphs renders them into card.md
// between generated-block markers. TestCardsUpToDate guards that the committed
// cards match — so a card's numbers are emitted by the model's own behaviour suite
// and cannot silently drift from the code.
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

// Observation is one measured point in a claim: a label (the varied input, the
// baseline, or the measured quantity) and the value the model produced for it.
type Observation struct {
	Label string
	Value float64
}

// Threshold asserts that one observation satisfies a bound against a fixed
// reference — for sign/level claims that are not a two-run comparison (e.g. "net
// revenue > 0", "final SoC < the initial charge"). GreaterThan reports the
// direction; Ref is the reference value and RefLabel how it renders on the card.
type Threshold struct {
	ObsIndex    int
	GreaterThan bool
	Ref         float64
	RefLabel    string
}

// Claim is a named, plain-language response claim, carrying the ensemble
// observations it produces and how they are asserted. At least one assertion is
// required, and both may apply to the same claim:
//
//   - Monotone != 0: the Observations must move that way in order (+1 increasing,
//     −1 decreasing) — the common base-vs-perturbed comparison.
//   - Thresholds: each named observation must satisfy its bound — for sign or level
//     claims (e.g. "revenue > 0", "off-path Δ < 0.01").
//
// ID matches the binding test's subtest name (the claim↔test bond); Statement is
// the human-readable claim; Unit annotates the values (e.g. "ensemble-mean final
// log-density"). Verify enforces the assertion; the card renders it.
type Claim struct {
	ID           string
	Statement    string
	Unit         string
	Monotone     int
	Thresholds   []Threshold
	Observations []Observation
}

// Verify checks a claim's assertions against its observations, returning a
// descriptive error if any does not hold. Monotone and Thresholds both apply when
// both are set. It is testing-free so the behaviour test and any tooling share one
// definition of what each claim asserts.
func Verify(c Claim) error {
	if c.Monotone == 0 && len(c.Thresholds) == 0 {
		return fmt.Errorf("claim %q has no assertion (set Monotone and/or Thresholds)", c.ID)
	}
	if c.Monotone != 0 && c.Monotone != 1 && c.Monotone != -1 {
		return fmt.Errorf("claim %q: Monotone must be -1, 0, or +1 (got %d)", c.ID, c.Monotone)
	}
	if c.Monotone != 0 {
		if len(c.Observations) < 2 {
			return fmt.Errorf("claim %q: monotone needs at least two observations", c.ID)
		}
		for i := 1; i < len(c.Observations); i++ {
			prev, cur := c.Observations[i-1], c.Observations[i]
			delta := cur.Value - prev.Value
			if c.Monotone == 1 && !(delta > 0) {
				return fmt.Errorf("%s: expected increase from %q (%.4f) to %q (%.4f)",
					c.ID, prev.Label, prev.Value, cur.Label, cur.Value)
			}
			if c.Monotone == -1 && !(delta < 0) {
				return fmt.Errorf("%s: expected decrease from %q (%.4f) to %q (%.4f)",
					c.ID, prev.Label, prev.Value, cur.Label, cur.Value)
			}
		}
	}
	for _, th := range c.Thresholds {
		if th.ObsIndex < 0 || th.ObsIndex >= len(c.Observations) {
			return fmt.Errorf("claim %q: threshold ObsIndex %d out of range", c.ID, th.ObsIndex)
		}
		o := c.Observations[th.ObsIndex]
		if th.GreaterThan && !(o.Value > th.Ref) {
			return fmt.Errorf("%s: expected %q (%.4f) > %s (%.4f)",
				c.ID, o.Label, o.Value, th.RefLabel, th.Ref)
		}
		if !th.GreaterThan && !(o.Value < th.Ref) {
			return fmt.Errorf("%s: expected %q (%.4f) < %s (%.4f)",
				c.ID, o.Label, o.Value, th.RefLabel, th.Ref)
		}
	}
	return nil
}

// FormatValue renders a single value at the fixed card precision.
func FormatValue(v float64) string {
	return fmt.Sprintf("%.*f", decimals, v)
}

// renderObserved describes a claim's observations and how they are asserted, for
// the card's "Observed" column.
func renderObserved(c Claim) string {
	parts := make([]string, len(c.Observations))
	for i, o := range c.Observations {
		parts[i] = fmt.Sprintf("%s %s", o.Label, FormatValue(o.Value))
	}
	body := strings.Join(parts, " · ")
	// When a claim carries threshold bounds, append the bound each observation is
	// checked against so the assertion is visible, not just the raw number.
	if len(c.Thresholds) > 0 {
		checks := make([]string, len(c.Thresholds))
		for i, th := range c.Thresholds {
			rel := "<"
			if th.GreaterThan {
				rel = ">"
			}
			checks[i] = fmt.Sprintf("%s %s %s",
				c.Observations[th.ObsIndex].Label, rel, th.RefLabel)
		}
		body = fmt.Sprintf("%s (asserts %s)", body, strings.Join(checks, ", "))
	}
	if c.Unit != "" {
		body = fmt.Sprintf("%s — %s", c.Unit, body)
	}
	return body
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
			"(a claim's assertion broke) or `TestCardsUpToDate` fails (a number moved) — a "+
			"broken claim cannot reach the card silently.\n\n",
		decimals, binding.TestName, regenCmd,
	)
	b.WriteString("| Response claim | Enforced by | Observed |\n")
	b.WriteString("|---|---|---|\n")
	for _, c := range claims {
		test := fmt.Sprintf("[`%s/%s`](%s)", binding.TestName, c.ID, binding.TestFile)
		fmt.Fprintf(&b, "| %s | %s | %s |\n",
			escapeCell(c.Statement), test, escapeCell(renderObserved(c)))
	}
	return strings.TrimRight(b.String(), "\n")
}

// escapeCell escapes markdown table-cell delimiters so a label or statement
// containing "|" cannot break the table structure.
func escapeCell(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
