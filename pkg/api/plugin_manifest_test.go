package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// repoRoot is where the plugin manifests and the CHANGELOG live, relative to this package.
const repoRoot = "../.."

// changelogVersionPattern matches a released CHANGELOG heading, e.g. "## [0.5.3] — 2026-07-20".
// "## [Unreleased]" deliberately does not match, so the newest *released* version wins.
var changelogVersionPattern = regexp.MustCompile(`(?m)^## \[(\d+\.\d+\.\d+)\]`)

// TestPluginManifestsMatchRelease guards the Claude Code plugin packaging against two
// silent-drift failure modes:
//
//  1. **Stale version.** Both `.claude-plugin/plugin.json` and `.claude-plugin/marketplace.json`
//     carry a version that must track the release. The CHANGELOG's newest released heading is the
//     source of truth (it is in-repo, needs no git or network, and matches the release ritual:
//     stamp `[X.Y.Z]`, then tag). Forgetting to bump a manifest advertises the wrong version to
//     everyone installing the plugin.
//  2. **Broken skill pointer.** `plugin.json`'s `skills` path is what makes the bundled skill
//     discoverable once installed. If it stops resolving to a directory containing the
//     stochadex-model SKILL.md, the plugin still installs and simply ships no skill — which no
//     other test would catch.
func TestPluginManifestsMatchRelease(t *testing.T) {
	wantVersion := newestReleasedVersion(t)

	var plugin struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Skills  string `json:"skills"`
	}
	readJSON(t, filepath.Join(repoRoot, ".claude-plugin", "plugin.json"), &plugin)

	var marketplace struct {
		Metadata struct {
			Version string `json:"version"`
		} `json:"metadata"`
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	readJSON(t, filepath.Join(repoRoot, ".claude-plugin", "marketplace.json"), &marketplace)

	t.Run("versions track the newest released CHANGELOG entry", func(t *testing.T) {
		if plugin.Version != wantVersion {
			t.Errorf("plugin.json version = %q, want %q (newest released CHANGELOG entry) — "+
				"bump it when stamping a release", plugin.Version, wantVersion)
		}
		if marketplace.Metadata.Version != wantVersion {
			t.Errorf("marketplace.json metadata.version = %q, want %q — "+
				"bump it when stamping a release", marketplace.Metadata.Version, wantVersion)
		}
	})

	t.Run("plugin skills path resolves to the bundled skill", func(t *testing.T) {
		if plugin.Skills == "" {
			t.Fatal("plugin.json has no skills path — the plugin would ship no skill")
		}
		skillsDir := filepath.Join(repoRoot, filepath.Clean(plugin.Skills))
		info, err := os.Stat(skillsDir)
		if err != nil || !info.IsDir() {
			t.Fatalf("plugin.json skills path %q does not resolve to a directory (%v)",
				plugin.Skills, err)
		}
		// The skill the plugin exists to ship must actually be under that path.
		skillFile := filepath.Join(skillsDir, "stochadex-model", "SKILL.md")
		if _, err := os.Stat(skillFile); err != nil {
			t.Errorf("bundled skill missing at %s: %v — the plugin would install with no skill",
				skillFile, err)
		}
	})

	t.Run("marketplace lists the plugin at a resolvable source", func(t *testing.T) {
		if len(marketplace.Plugins) == 0 {
			t.Fatal("marketplace.json lists no plugins")
		}
		entry := marketplace.Plugins[0]
		if entry.Name != plugin.Name {
			t.Errorf("marketplace plugin name %q != plugin.json name %q", entry.Name, plugin.Name)
		}
		// The source must contain the plugin manifest Claude Code reads on install.
		manifest := filepath.Join(repoRoot, filepath.Clean(entry.Source), ".claude-plugin", "plugin.json")
		if _, err := os.Stat(manifest); err != nil {
			t.Errorf("marketplace source %q has no .claude-plugin/plugin.json (%v)", entry.Source, err)
		}
	})
}

// newestReleasedVersion returns the first semver heading in the CHANGELOG, which is the most
// recent released version ([Unreleased] is skipped by the pattern).
func newestReleasedVersion(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repoRoot, "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("reading CHANGELOG: %v", err)
	}
	match := changelogVersionPattern.FindSubmatch(body)
	if match == nil {
		t.Fatal("no released version heading found in CHANGELOG.md")
	}
	return string(match[1])
}

func readJSON(t *testing.T, path string, into interface{}) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if err := json.Unmarshal(body, into); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
}
