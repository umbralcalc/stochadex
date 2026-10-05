#!/usr/bin/env bash
# Print the CHANGELOG.md section body for a single release version.
#
# This is the single source of truth for release-note text. Both the annotated
# tag message (tag-release.yml) and the published GitHub Release body
# (release.yml) are produced from it, so the two are byte-identical by
# construction.
#
# Why this exists rather than `gh release create --notes-from-tag`: on a tag
# push, actions/checkout fetches the tag by commit SHA, which materialises a
# *lightweight* local tag even when the pushed tag is annotated. `gh
# --notes-from-tag` reads the tag body with `git for-each-ref %(contents)`,
# and on a lightweight tag that resolves to the *commit* message (the squash-
# merge message) instead of the annotation — so the notes came out as the PR
# commit body on every release. Reading CHANGELOG.md directly sidesteps the
# tag object entirely and is deterministic regardless of how the tag was
# fetched.
#
# Usage: changelog-section.sh <version-or-vTag> [changelog-path]
#   changelog-section.sh v0.16.0
#   changelog-section.sh 0.16.0 path/to/CHANGELOG.md
set -euo pipefail

raw="${1:?usage: changelog-section.sh <version> [changelog-path]}"
version="${raw#v}"                       # accept either "v0.16.0" or "0.16.0"
changelog="${2:-CHANGELOG.md}"

# Emit everything between this version's "## [X.Y.Z]" heading and the next
# "## [" heading, then drop leading blank lines. Mirrors the extraction the
# tag annotation has always used.
section=$(awk -v v="[${version}]" '
  /^## \[/ { if (found) exit; if (index($0, v)) { found = 1; next } }
  found    { print }
' "$changelog" | sed '/./,$!d')

if [ -z "$section" ]; then
  echo "changelog-section.sh: no section found for [${version}] in ${changelog}" >&2
  exit 1
fi

printf '%s\n' "$section"
