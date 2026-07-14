#!/usr/bin/env python3
"""NumPy side of the vectorized-op comparison (a fair CPU-to-CPU check).

Runs the identical AXPY (y += a*x) and DOT ops as benchmarks/main.go's gonum path,
at the same sizes, and writes results/vectorized_ops_numpy.json. The point is not to
win — it is to show parity: you don't give up vectorized throughput by being in Go.

Run: python3 benchmarks/numpy_ops.py   (from the repo root, after `go run ./benchmarks`)
"""
import json
import os
import time

import numpy as np

SIZES = [1_000, 10_000, 100_000, 1_000_000, 10_000_000]
REPEATS = 50


def best_gflops(fn, flops):
    best = float("inf")
    for _ in range(REPEATS):
        t0 = time.perf_counter()
        fn()
        dt = time.perf_counter() - t0
        best = min(best, dt)
    return best, flops / best / 1e9


def main():
    out = []
    for n in SIZES:
        x = ((np.arange(n) % 7) + 0.5).astype(np.float64)
        y = ((np.arange(n) % 5) + 0.5).astype(np.float64)

        a = 1.0000001
        sec, g = best_gflops(lambda: np.add(y, a * x, out=y), 2 * n)
        out.append({"size": n, "op": "axpy", "best_seconds": sec, "gflops": g})

        sec, g = best_gflops(lambda: np.dot(x, y), 2 * n)
        out.append({"size": n, "op": "dot", "best_seconds": sec, "gflops": g})
        print(f"  vec n={n:<9d}  axpy {out[-2]['gflops']:.2f} GFLOP/s  dot {out[-1]['gflops']:.2f} GFLOP/s")

    results_dir = "benchmarks/results" if os.path.isdir("benchmarks/results") else "results"
    os.makedirs(results_dir, exist_ok=True)
    with open(os.path.join(results_dir, "vectorized_ops_numpy.json"), "w") as f:
        json.dump(out, f, indent=2)
    meta_path = os.path.join(results_dir, "numpy_meta.json")
    with open(meta_path, "w") as f:
        json.dump({"numpy_version": np.__version__}, f, indent=2)
    print("wrote", os.path.join(results_dir, "vectorized_ops_numpy.json"))


if __name__ == "__main__":
    main()
