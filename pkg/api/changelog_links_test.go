package api

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestChangelogReleaseLinksResolve guards the release ritual against a silent-drift bug
// that shipped in v0.15.0: the stamp added the `## [0.15.0]` heading but no matching
// `[0.15.0]: …compare/v0.14.0...v0.15.0` reference-link definition, so the heading rendered
// unlinked, and it left `[Unreleased]` comparing from the previous version. Both are now
// managed by the prepare-release workflow; this test is the backstop that fails CI if a
// future release regresses either one.
//
// It checks only the NEWEST released version, deliberately: several historical headings
// (0.8.0–0.11.0) predate the reference-link convention and are not worth backfilling, so a
// blanket "every heading is linked" rule would fail on that narrative history. The release
// ritual is what this protects, and the ritual only ever adds the newest one.
func TestChangelogReleaseLinksResolve(t *testing.T) {
	version := newestReleasedVersion(t)
	body, err := os.ReadFile(filepath.Join(repoRoot, "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("reading CHANGELOG: %v", err)
	}
	text := string(body)

	t.Run("the newest released heading has a link definition", func(t *testing.T) {
		// e.g. "[0.15.0]: https://…/compare/v0.14.0...v0.15.0" at column 0.
		def := regexp.MustCompile(fmt.Sprintf(`(?m)^\[%s\]:\s+\S`, regexp.QuoteMeta(version)))
		if !def.MatchString(text) {
			t.Errorf("CHANGELOG.md has a `## [%s]` heading but no `[%s]: <url>` link "+
				"definition — the heading renders unlinked. Add the compare link when "+
				"stamping the release (the prepare-release workflow does this).", version, version)
		}
	})

	t.Run("Unreleased compares from the newest released version", func(t *testing.T) {
		want := fmt.Sprintf("v%s...HEAD", version)
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
