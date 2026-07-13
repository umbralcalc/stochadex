package main

import (
	"strings"
	"testing"
)

// TestCardsUpToDate fails if any models/<domain>/card.md generated block is out
// of sync with the code — either the partition-wiring diagram (from BuildStub) or
// the observed-behaviour numbers (from the model's ObservedBehaviour). This is the
// CI guard: changing a stub's wiring or its generative behaviour without
// regenerating the cards breaks the build, so a card can never show a stale number.
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
			"stale generated card sections (wiring diagram or observed-behaviour numbers) in: %s\n"+
				"run `go generate ./cmd/model-graphs` to regenerate",
			strings.Join(changed, ", "),
		)
	}
}
