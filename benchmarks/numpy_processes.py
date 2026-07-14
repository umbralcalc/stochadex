#!/usr/bin/env python3
"""NumPy side of the whole-process comparison — the same stochastic processes as
benchmarks/main.go, implemented the idiomatic efficient way: a step loop in Python,
vectorized over all paths, single-threaded. Same total path count and step count, so
the two are directly comparable.

Writes results/processes_numpy.json. Run after `go run ./benchmarks`:
    python3 benchmarks/numpy_processes.py
"""
import json
import os
import time

import numpy as np

TOTAL_PATHS = 10_000
STEPS = 2_000
DT = 0.01
REPEATS = 3


def gbm(rng):
    s = np.ones(TOTAL_PATHS)
    c = np.sqrt(0.04 * DT)
    for _ in range(STEPS):
        s *= 1.0 + c * rng.standard_normal(TOTAL_PATHS)
    return s


def ou(rng):
    x = np.zeros(TOTAL_PATHS)
    theta, mu, sigma = 0.5, 0.0, 0.3
    c = sigma * np.sqrt(DT)
    for _ in range(STEPS):
        x += theta * (mu - x) * DT + c * rng.standard_normal(TOTAL_PATHS)
    return x


def compound_poisson(rng):
    x = np.zeros(TOTAL_PATHS)
    rate, alpha, beta = 5.0, 2.0, 1.0
    p = rate / (rate + 1.0 / DT)  # per-step jump probability (matches the Go test)
    for _ in range(STEPS):
        jump = rng.random(TOTAL_PATHS) < p
        x += jump * rng.gamma(alpha, 1.0 / beta, TOTAL_PATHS)
    return x


def best_seconds(fn):
    best = float("inf")
    for _ in range(REPEATS):
        rng = np.random.default_rng(1)
        t0 = time.perf_counter()
        fn(rng)
        best = min(best, time.perf_counter() - t0)
    return best


def main():
    procs = {"gbm": gbm, "ou": ou, "compound_poisson": compound_poisson}
    out = []
    for name, fn in procs.items():
        sec = best_seconds(fn)
        out.append({"process": name, "total_paths": TOTAL_PATHS, "steps": STEPS, "numpy_seconds": sec})
        print(f"  {name:<16s}  numpy {sec:.3f}s  ({TOTAL_PATHS} paths x {STEPS} steps)")

    results_dir = "benchmarks/results" if os.path.isdir("benchmarks/results") else "results"
    with open(os.path.join(results_dir, "processes_numpy.json"), "w") as f:
        json.dump(out, f, indent=2)
    print("wrote", os.path.join(results_dir, "processes_numpy.json"))


if __name__ == "__main__":
    main()
