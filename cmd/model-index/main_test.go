package main

import "testing"

// TestModelIndexUpToDate fails if the committed cross-model index artifacts
// (models/INDEX.md and models/index.json) are out of sync with the model stubs.
// This is the CI guard: adding or changing a model's core-package usage, bespoke
// iterations, or behaviour binding without regenerating the index breaks the build.
//
// To fix a failure, run:
//
//	go generate ./cmd/model-index   (or: go run ./cmd/model-index)
func TestModelIndexUpToDate(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	changed, err := run(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("stale cross-model index — run `go generate ./cmd/model-index` to regenerate")
	}
}
