#!/usr/bin/env python3
"""NumPy side of the coupled-system comparison — the same coupled OU chain as
benchmarks/main.go's coupledGen (chainLen = 4). Component j mean-reverts toward
component j-1's current value, so the K updates per step must run in dependency
order (they cannot be fused). Vectorized over all paths, single-threaded.

Writes results/coupled_numpy.json. Run: python3 benchmarks/numpy_coupled.py
"""
import json
import os
import time

import numpy as np

TOTAL_PATHS = 10_000
STEPS = 2_000
DT = 0.01
CHAIN_LEN = 4
REPEATS = 3


def coupled_chain(rng):
    comps = [np.zeros(TOTAL_PATHS) for _ in range(CHAIN_LEN)]
    theta, sigma = 1.0, 0.3
    c = sigma * np.sqrt(DT)
    for _ in range(STEPS):
        for j in range(CHAIN_LEN):
            mu = comps[j - 1] if j > 0 else 0.0  # track the upstream component
            comps[j] += theta * (mu - comps[j]) * DT + c * rng.standard_normal(TOTAL_PATHS)
    return comps


def main():
    best = float("inf")
    for _ in range(REPEATS):
        rng = np.random.default_rng(1)
        t0 = time.perf_counter()
        coupled_chain(rng)
        best = min(best, time.perf_counter() - t0)
    print(f"  coupled_ou_chain_len4  numpy {best:.3f}s  ({TOTAL_PATHS} paths x {STEPS} steps)")

    results_dir = "benchmarks/results" if os.path.isdir("benchmarks/results") else "results"
    with open(os.path.join(results_dir, "coupled_numpy.json"), "w") as f:
        json.dump([{"process": "coupled_ou_chain_len4", "total_paths": TOTAL_PATHS,
                    "steps": STEPS, "numpy_seconds": best}], f, indent=2)
    print("wrote", os.path.join(results_dir, "coupled_numpy.json"))


if __name__ == "__main__":
    main()
