package api

import (
	"bytes"
	"strings"
	"testing"
)

// withBuildInfo swaps the package build-info globals for the duration of a test and
// restores them, so cases cannot leak state into one another (or into other tests
// that read LogRunProvenance's output).
func withBuildInfo(t *testing.T, version, revision string, features []string) {
	t.Helper()
	origV, origR, origF := BuildVersion, BuildRevision, BuildFeatures
	t.Cleanup(func() {
		BuildVersion, BuildRevision, BuildFeatures = origV, origR, origF
	})
	BuildVersion, BuildRevision, BuildFeatures = version, revision, features
}

func TestLogRunProvenance(t *testing.T) {
	t.Run("reports version, os and arch on a single line", func(t *testing.T) {
		withBuildInfo(t, "v1.2.3", "", nil)
		var buf bytes.Buffer
		LogRunProvenance(&buf)
		line := buf.String()
		if strings.Count(line, "\n") != 1 {
			t.Fatalf("expected exactly one line, got %q", line)
		}
		for _, want := range []string{"stochadex-run", "version=v1.2.3", "os=", "arch="} {
			if !strings.Contains(line, want) {
				t.Errorf("provenance line %q missing %q", line, want)
			}
		}
	})

	t.Run("omits image and features when unset", func(t *testing.T) {
		withBuildInfo(t, "dev", "", nil)
		var buf bytes.Buffer
		LogRunProvenance(&buf)
		line := buf.String()
		if strings.Contains(line, "image=") {
			t.Errorf("expected no image= without %s, got %q", imageDigestEnv, line)
		}
		if strings.Contains(line, "features=") {
			t.Errorf("expected no features= with none compiled in, got %q", line)
		}
	})

	t.Run("echoes an injected image digest", func(t *testing.T) {
		withBuildInfo(t, "v1.2.3", "", nil)
		t.Setenv(imageDigestEnv, "sha256:deadbeef")
		var buf bytes.Buffer
		LogRunProvenance(&buf)
		if !strings.Contains(buf.String(), "image=sha256:deadbeef") {
			t.Errorf("provenance line %q did not echo injected digest", buf.String())
		}
	})

	t.Run("reports a stamped revision verbatim", func(t *testing.T) {
		withBuildInfo(t, "v1.2.3", "abc123", nil)
		var buf bytes.Buffer
		LogRunProvenance(&buf)
		line := buf.String()
		if !strings.Contains(line, "revision=abc123") {
			t.Errorf("provenance line %q missing stamped revision", line)
		}
		// A stamped revision comes with no build info, so nothing can claim dirty.
		if strings.Contains(line, "dirty=") {
			t.Errorf("stamped revision must not carry a dirty flag, got %q", line)
		}
	})

	t.Run("lists compiled-in features sorted", func(t *testing.T) {
		withBuildInfo(t, "v1.2.3", "", []string{"s3", "arrow", "postgres"})
		var buf bytes.Buffer
		LogRunProvenance(&buf)
		if !strings.Contains(buf.String(), "features=arrow,postgres,s3") {
			t.Errorf("expected sorted features, got %q", buf.String())
		}
	})
}
