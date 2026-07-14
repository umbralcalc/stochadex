#!/usr/bin/env python3
"""NumPy side of the branching-coupled comparison — the same threshold-triggered system
as benchmarks/main.go's branchCoupledGen: an OU driver, and a responder that does an
expensive jump (sum of 30 gamma draws) ONLY when the driver crosses a threshold
(~7% of path-steps). This is the coupling that is hard to vectorize over paths.

Two NumPy implementations, both timed and reported (be fair):
  - "mask" — the idiomatic vectorized version: compute the jump for every path, select
    by mask. Simple, but wastes ~93% of the gamma draws.
  - "gather" — the optimized version: index the (few) triggered paths and compute only
    for them. Faster, but non-obvious gather/scatter code.

Writes results/branch_coupled_numpy.json. Run: python3 benchmarks/numpy_branch_coupled.py
"""
import json
import os
import time

import numpy as np

N = 10_000
STEPS = 2_000
DT = 0.01
TERMS = 30
THRESHOLD = 1.5
REPEATS = 3


def driver_step(a, rng):
    return a + 0.5 * (0.0 - a) * DT + 1.0 * np.sqrt(DT) * rng.standard_normal(N)


def mask(rng):
    a, b = np.zeros(N), np.zeros(N)
    for _ in range(STEPS):
        a = driver_step(a, rng)
        b *= 0.99
        jumps = rng.gamma(2.0, 1.0, (TERMS, N)).sum(0)  # every path, then discard ~93%
        b += np.where(a > THRESHOLD, jumps, 0.0)
    return b


def gather(rng):
    a, b = np.zeros(N), np.zeros(N)
    for _ in range(STEPS):
        a = driver_step(a, rng)
        b *= 0.99
        idx = np.nonzero(a > THRESHOLD)[0]
        if idx.size:
            b[idx] += rng.gamma(2.0, 1.0, (TERMS, idx.size)).sum(0)
    return b


def best_seconds(fn):
    best = float("inf")
    for _ in range(REPEATS):
        rng = np.random.default_rng(1)
        t0 = time.perf_counter()
        fn(rng)
        best = min(best, time.perf_counter() - t0)
    return best


def main():
    out = {"process": "branch_coupled", "total_paths": N, "steps": STEPS,
           "numpy_mask_seconds": best_seconds(mask),
           "numpy_gather_seconds": best_seconds(gather)}
    print(f"  numpy mask (compute all, select): {out['numpy_mask_seconds']:.3f}s")
    print(f"  numpy gather (triggered only):    {out['numpy_gather_seconds']:.3f}s")
    results_dir = "benchmarks/results" if os.path.isdir("benchmarks/results") else "results"
    with open(os.path.join(results_dir, "branch_coupled_numpy.json"), "w") as f:
        json.dump([out], f, indent=2)
    print("wrote", os.path.join(results_dir, "branch_coupled_numpy.json"))


if __name__ == "__main__":
    main()
