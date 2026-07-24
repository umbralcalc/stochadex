// Command stochadex-full is the distributed stochadex CLI: the same engine and the same
// YAML surface as cmd/stochadex, plus the egress integrations that live in opt-in modules.
//
// It exists because imports drive go.mod. Adding Arrow or DuckDB to cmd/stochadex would
// impose them on every downstream repo that imports the engine as a library, so the richer
// CLI lives in its own module and registers the extra sinks through the engine's public
// downstream-extension hook (simulator.RegisterComponent) — no engine change required.
//
// Two builds come from this one package:
//
//	# pure Go, cross-compiles to every platform, no toolchain needed to run:
//	CGO_ENABLED=0 go build -o stochadex .
//
//	# accelerated: optimised system BLAS + DuckDB (needs cgo, builds natively):
//	CGO_ENABLED=1 CGO_LDFLAGS="-framework Accelerate" \
//	    go build -tags "cblas duckdb_arrow" -o stochadex-accel .   # macOS
//	CGO_ENABLED=1 CGO_LDFLAGS="-lopenblas" \
//	    go build -tags "cblas duckdb_arrow" -o stochadex-accel .   # Linux
//
// The `cblas` tag routes gonum's BLAS to the linked system library (see
// pkg/simulator/blas_accelerated.go); `duckdb_arrow` compiles the DuckDB sink below.
package main

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/umbralcalc/stochadex/pkg/api"
)

// version is stamped at build time with -ldflags "-X main.version=<tag>". It is a real
// variable, not a placeholder: without one the -X flag silently does nothing.
var version = "dev"

// revision is the git commit, stamped with -ldflags "-X main.revision=<sha>" only by
// builds that cannot embed VCS info themselves — the OCI image, whose context excludes
// .git. It stays empty for the binary releases, which the toolchain stamps
// automatically from the repository (see api.BuildRevision).
var revision = ""

func main() {
	// Handled before ArgParse so it needs no config and cannot be refused by argument
	// validation — `--version` must answer even on a machine with nothing set up.
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-version" || arg == "version" {
			printVersion()
			return
		}
	}
	// Hand this build's version stamp and compiled-in feature list to the engine so
	// the per-run provenance line (api.LogRunProvenance) reports the accelerated CLI
	// as what actually ran, not the base engine's "dev"/no-features default.
	api.BuildVersion = version
	api.BuildFeatures = features
	api.BuildRevision = revision
	api.RunWithParsedArgs(api.ArgParse())
}

// printVersion reports the build and, crucially, the optional capabilities compiled in.
// The portable and accelerated assets are the same CLI with different features, so this
// is how a caller confirms it has the binary it needs — e.g. before writing a config that
// uses `output_function: {type: duckdb}`, which only the accelerated build can serve.
func printVersion() {
	compiled := append([]string(nil), features...)
	sort.Strings(compiled)
	fmt.Printf("stochadex %s %s/%s\n", version, runtime.GOOS, runtime.GOARCH)
	fmt.Printf("features: %s\n", strings.Join(compiled, " "))
}
