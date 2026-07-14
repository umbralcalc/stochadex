//go:build cblas

// This file is compiled only with the `cblas` build tag. It routes gonum's BLAS
// (used across continuous/inference/analysis for dot, gemv, gemm, etc.) to a linked
// system C BLAS — OpenBLAS, Apple Accelerate, or Intel MKL, the same optimized library
// NumPy uses — for a large speedup on BLAS-heavy operations. The init runs once at
// startup for any binary that uses the simulator, so enabling it is just a build flag.
//
// TRADEOFF (why it is opt-in): this pulls in cgo, so the binary is no longer pure-Go
// and cannot target WASM. The default build (no tag) stays pure-Go and WASM-clean — the
// engine's deployment property — so enable this only when you want NumPy-class BLAS
// throughput and do not need WASM or a static single binary.
//
// Build with the system BLAS linked, e.g.
//
//	# macOS (Apple Accelerate):
//	CGO_ENABLED=1 CGO_LDFLAGS="-framework Accelerate" go build -tags cblas ./...
//	# Linux (OpenBLAS):
//	CGO_ENABLED=1 CGO_LDFLAGS="-lopenblas" go build -tags cblas ./...
//
// Requires the dependency gonum.org/v1/netlib (already in go.mod).
package simulator

import (
	"gonum.org/v1/gonum/blas/blas64"
	"gonum.org/v1/netlib/blas/netlib"
)

func init() {
	blas64.Use(netlib.Implementation{})
}
