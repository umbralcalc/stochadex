package main

import (
	"strings"
	"testing"
)

// TestCardsUpToDate fails if any models/<domain>/card.md wiring diagram is out
// of sync with its stub's BuildStub wiring. This is the CI guard: changing a
// stub's partition wiring without regenerating the cards breaks the build.
//
// To fix a failure, run:
//
//	go generate ./cmd/model-graphs   (or: go run ./cmd/model-graphs)
func TestCardsUpToDate(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	changed, err := run(root, false, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) > 0 {
		t.Errorf(
			"stale partition-wiring diagrams in: %s\nrun `go generate ./cmd/model-graphs` to regenerate",
			strings.Join(changed, ", "),
		)
	}
}
