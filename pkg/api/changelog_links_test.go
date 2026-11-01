package api

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestChangelogReleaseLinksResolve guards the reference-style version links at the foot of
// the CHANGELOG: every released "## [X.Y.Z]" heading needs a matching
// "[X.Y.Z]: …compare/vW...vX.Y.Z" link definition, or it renders unlinked, and
// "[Unreleased]" must compare from the latest release. The prepare-release workflow
// maintains both; this test is the backstop that fails CI if any release regresses either
// one. Every released heading is checked.
func TestChangelogReleaseLinksResolve(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(repoDir, "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("reading CHANGELOG: %v", err)
	}
	text := string(body)

	// All released headings, e.g. "## [0.15.0] — 2026-08-09" → "0.15.0".
	headings := regexp.MustCompile(`(?m)^## \[(\d+\.\d+\.\d+)\]`).FindAllStringSubmatch(text, -1)
	if len(headings) == 0 {
		t.Fatal("no released version headings found in CHANGELOG.md")
	}

	t.Run("every released heading has a link definition", func(t *testing.T) {
		for _, h := range headings {
			version := h[1]
			// e.g. "[0.15.0]: https://…/compare/v0.14.0...v0.15.0" at column 0.
			def := regexp.MustCompile(fmt.Sprintf(`(?m)^\[%s\]:\s+\S`, regexp.QuoteMeta(version)))
			if !def.MatchString(text) {
				t.Errorf("`## [%s]` heading has no `[%s]: <url>` link definition — the "+
					"heading renders unlinked. Add the compare link when stamping the "+
					"release (the prepare-release workflow does this).", version, version)
			}
		}
	})

	t.Run("Unreleased compares from the newest released version", func(t *testing.T) {
		// Headings are newest-first, so the first is the newest released version.
		newest := headings[0][1]
		want := fmt.Sprintf("v%s...HEAD", newest)
		line := regexp.MustCompile(`(?m)^\[Unreleased\]:\s+(\S+)`).FindStringSubmatch(text)
		if line == nil {
			t.Fatal("CHANGELOG.md has no `[Unreleased]: <url>` link definition")
		}
		if !strings.HasSuffix(line[1], want) {
			t.Errorf("`[Unreleased]` link is %q, want it to compare from the newest release "+
				"(…/%s) — bump it when stamping a release", line[1], want)
		}
	})
}
