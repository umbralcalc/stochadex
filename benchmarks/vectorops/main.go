// Command vectorops measures per-partition vector-op throughput (AXPY, DOT) via gonum's
// BLAS — the "no engine involved" micro-benchmark of README §5. It is a separate command
// from the engine suite (benchmarks/) precisely because it is the only benchmark that
// touches BLAS, so it is the only one the accelerated backend changes.
//
// Two builds, two committed result files:
//
//	go run ./benchmarks/vectorops                                            # pure-Go BLAS -> vectorized_ops_go.json
//	CGO_ENABLED=1 CGO_LDFLAGS="-framework Accelerate" \
//	  go run -tags cblas ./benchmarks/vectorops                              # Accelerate BLAS -> vectorized_ops_go_cblas.json
//
// The blank import of pkg/simulator is what makes `-tags cblas` bite: it compiles the
// accelerated-BLAS registration in pkg/simulator/blas_accelerated.go, whose init routes
// gonum's blas64 to a linked system C BLAS (see resultSuffix in variant_*.go).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gonum.org/v1/gonum/blas/blas64"

	_ "github.com/umbralcalc/stochadex/pkg/simulator"
)

type opPoint struct {
	Size        int     `json:"size"`
	Op          string  `json:"op"`
	BestSeconds float64 `json:"best_seconds"`
	GFLOPs      float64 `json:"gflops"`
}

func main() {
	sizes := []int{1_000, 10_000, 100_000, 1_000_000, 10_000_000}
	const repeats = 20
	var out []opPoint
	for _, n := range sizes {
		x := make([]float64, n)
		y := make([]float64, n)
		for i := range x {
			x[i], y[i] = float64(i%7)+0.5, float64(i%5)+0.5
		}
		vx := blas64.Vector{N: n, Inc: 1, Data: x}
		vy := blas64.Vector{N: n, Inc: 1, Data: y}

		// AXPY: 2n flops.
		best := time.Duration(1 << 62)
		for r := 0; r < repeats; r++ {
			start := time.Now()
			blas64.Axpy(1.0000001, vx, vy)
			if d := time.Since(start); d < best {
				best = d
			}
		}
		out = append(out, opPoint{Size: n, Op: "axpy", BestSeconds: best.Seconds(),
			GFLOPs: 2 * float64(n) / best.Seconds() / 1e9})

		// DOT: 2n flops.
		best = time.Duration(1 << 62)
		for r := 0; r < repeats; r++ {
			start := time.Now()
			_ = blas64.Dot(vx, vy)
			if d := time.Since(start); d < best {
				best = d
			}
		}
		out = append(out, opPoint{Size: n, Op: "dot", BestSeconds: best.Seconds(),
			GFLOPs: 2 * float64(n) / best.Seconds() / 1e9})
		fmt.Printf("  vec n=%-9d  axpy %.2f GFLOP/s  dot %.2f GFLOP/s\n",
			n, out[len(out)-2].GFLOPs, out[len(out)-1].GFLOPs)
	}

	dir := "benchmarks/results"
	if _, err := os.Stat("results"); err == nil {
		dir = "results" // running from inside benchmarks/
	}
	name := "vectorized_ops_go" + resultSuffix + ".json"
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		panic(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		panic(err)
	}
	fmt.Println("wrote", filepath.Join(dir, name))
}
