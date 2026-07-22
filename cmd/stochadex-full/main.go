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
	"github.com/umbralcalc/stochadex/pkg/api"
)

func main() {
	api.RunWithParsedArgs(api.ArgParse())
}
