package api

import (
	"errors"
	"strings"
	"testing"
)

// TestCheckGoToolchain covers the preflight that guards the code-generation path.
// It matters because the published container image ships no Go toolchain: without
// this check a Go-expression config fails there with an opaque
// `exec: "go": executable file not found in $PATH` panic, which tells a user
// nothing about which half of the config surface they are on.
func TestCheckGoToolchain(t *testing.T) {
	t.Run("passes when go is on PATH", func(t *testing.T) {
		found := func(string) (string, error) { return "/usr/local/go/bin/go", nil }
		if err := checkGoToolchain(found); err != nil {
			t.Errorf("expected no error when go resolves: %v", err)
		}
	})

	t.Run("fails with the actionable message when go is absent", func(t *testing.T) {
		missing := func(string) (string, error) {
			return "", errors.New(`exec: "go": executable file not found in $PATH`)
		}
		err := checkGoToolchain(missing)
		if err == nil {
			t.Fatal("expected an error when go does not resolve")
		}
		// The CI image job greps for this exact phrase to assert the contract, so a
		// reword that drops it breaks that check silently.
		if !strings.Contains(err.Error(), "requires a Go toolchain") {
			t.Errorf("message must contain the phrase CI asserts on, got: %v", err)
		}
		// Both escape routes must be named — installing Go is not always the right
		// fix, and for a container user it is the wrong one.
		for _, want := range []string{"install Go", "data", "in-process"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("message should mention %q, got: %v", want, err)
			}
		}
	})

	t.Run("the exported message is what the check returns", func(t *testing.T) {
		missing := func(string) (string, error) { return "", errors.New("nope") }
		if got := checkGoToolchain(missing).Error(); got != GoToolchainMissingMessage {
			t.Errorf("check returned a different string than the exported constant:\n%s", got)
		}
	})
}
