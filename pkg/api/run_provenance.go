package api

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
)

// BuildVersion and BuildFeatures describe the executable orchestrating a run. They
// default to the base engine's view — "dev", no optional features — and are
// overwritten by a CLI's main package before RunWithParsedArgs is called.
// cmd/stochadex sets them from its own -ldflags version stamp and its
// compiled-in feature list, so a provenance line reports exactly the binary that ran
// rather than a generic default.
var (
	BuildVersion  = "dev"
	BuildFeatures []string
)

// BuildRevision is the git commit a build was stamped with via -ldflags, for builds
// that cannot read it from the embedded Go build info. The OCI image is exactly this
// case: its build context excludes .git (see .dockerignore), so the toolchain has no
// VCS to read and the release workflow passes the commit in explicitly instead. When
// set it wins over the embedded build info; when empty the build info is used, which
// is how the binary releases (built with .git present) report their revision.
var BuildRevision = ""

// imageDigestEnv names the environment variable an orchestrator injects with the
// digest of the OCI image a run is executing from. A binary cannot know the digest
// of the image that wraps it — the digest is a hash of the image, so nothing
// knowable at build time can be baked in as the answer. The image instead carries
// its git revision (via the Go build info below) and its human tag (STOCHADEX_VERSION
// / the OCI version label), and a deployer that resolved an image@sha256:… reference
// can close the loop by passing that digest through this variable. When it is set,
// the run echoes it back; when it is not, the version and revision still pin the
// source that produced a result.
const imageDigestEnv = "STOCHADEX_IMAGE_DIGEST"

// LogRunProvenance writes a single machine-parseable provenance line to w at the
// start of a run. It is deliberately sent to stderr, never stdout, so it never
// corrupts a data output (StdoutOutputFunction writes result rows to stdout); in a
// containerised run — the image's whole reason for being — the job log that captures
// stderr is the durable record a result is reproduced against. The line is
// space-separated key=value pairs so it survives log aggregation and greps cleanly:
//
//	stochadex-run version=v0.7.0 os=linux arch=arm64 revision=d44bcd8… features=arrow,cblas,duckdb,postgres,s3 image=sha256:abc…
//
// revision and dirty come from the Go build info the toolchain embeds automatically
// (buildvcs, on by default); image= is present only when STOCHADEX_IMAGE_DIGEST is
// set. A result is only reproducible if you know which build produced it, and this
// line is that binding for the CLI/image surface.
func LogRunProvenance(w io.Writer) {
	fields := []string{
		"stochadex-run",
		"version=" + BuildVersion,
		"os=" + runtime.GOOS,
		"arch=" + runtime.GOARCH,
	}
	if BuildRevision != "" {
		// Explicitly stamped (the image path): trust it as-is. The build info is
		// unavailable here, so there is no dirty flag to report.
		fields = append(fields, "revision="+BuildRevision)
	} else if revision, dirty, ok := vcsRevision(); ok {
		fields = append(fields, "revision="+revision)
		if dirty {
			// A working tree with uncommitted changes cannot be pinned to a commit,
			// so say so rather than let the revision imply a clean build.
			fields = append(fields, "dirty=true")
		}
	}
	if len(BuildFeatures) > 0 {
		feats := append([]string(nil), BuildFeatures...)
		sort.Strings(feats)
		fields = append(fields, "features="+strings.Join(feats, ","))
	}
	if digest := os.Getenv(imageDigestEnv); digest != "" {
		fields = append(fields, "image="+digest)
	}
	fmt.Fprintln(w, strings.Join(fields, " "))
}

// vcsRevision reads the git revision and dirty flag the Go toolchain embeds in the
// binary at build time. ok is false when the build carried no VCS info (e.g. built
// with -buildvcs=false, or from outside a repository), in which case the caller
// simply omits the field.
func vcsRevision() (revision string, dirty bool, ok bool) {
	info, available := debug.ReadBuildInfo()
	if !available {
		return "", false, false
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			dirty = setting.Value == "true"
		}
	}
	if revision == "" {
		return "", false, false
	}
	return revision, dirty, true
}
