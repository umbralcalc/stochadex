#!/usr/bin/env python3
"""Render the committed benchmark plots from results/*.json.

Run: python3 benchmarks/plot.py   (after `go run ./benchmarks` and `python3 benchmarks/numpy_ops.py`)
Writes benchmarks/plots/*.svg.
"""
import json
import os

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt

BASE = "benchmarks" if os.path.isdir("benchmarks/results") else "."
RESULTS = os.path.join(BASE, "results")
PLOTS = os.path.join(BASE, "plots")
GREEN = "#4D7F37"
GREY = "#64748b"


def load(name):
    with open(os.path.join(RESULTS, name)) as f:
        return json.load(f)


def plot_ensemble_scaling():
    data = load("ensemble_scaling.json")
    x = [d["max_concurrency"] for d in data]
    y = [d["members_per_sec"] for d in data]
    fig, ax = plt.subplots(figsize=(7, 4.2))
    ax.plot(x, y, "o-", color=GREEN, linewidth=2, markersize=6, label="measured throughput")
    # Ideal linear scaling from the single-worker point (reference line).
    ax.plot(x, [y[0] * xi for xi in x], "--", color=GREY, linewidth=1, label="ideal linear")
    ax.set_xlabel("max concurrency (parallel ensemble members)")
    ax.set_ylabel("independent simulations / second")
    ax.set_title("Ensemble scaling — independent simulations are embarrassingly parallel")
    ax.grid(True, alpha=0.3)
    ax.legend()
    fig.tight_layout()
    fig.savefig(os.path.join(PLOTS, "ensemble_scaling.svg"))
    plt.close(fig)


def plot_vectorized_ops():
    go = load("vectorized_ops_go.json")
    npv = load("vectorized_ops_numpy.json")
    fig, axes = plt.subplots(1, 2, figsize=(11, 4.2))
    for ax, op in zip(axes, ["axpy", "dot"]):
        gx = [d["size"] for d in go if d["op"] == op]
        gy = [d["gflops"] for d in go if d["op"] == op]
        nx = [d["size"] for d in npv if d["op"] == op]
        ny = [d["gflops"] for d in npv if d["op"] == op]
        ax.plot(gx, gy, "o-", color=GREEN, linewidth=2, label="gonum (Go, pure-Go BLAS)")
        ax.plot(nx, ny, "s--", color=GREY, linewidth=2, label="NumPy (Accelerate BLAS)")
        ax.set_xscale("log")
        ax.set_xlabel("vector length")
        ax.set_ylabel("GFLOP/s")
        ax.set_title(op.upper())
        ax.grid(True, alpha=0.3)
        ax.legend()
    fig.suptitle("Per-partition vector-op throughput — CPU-to-CPU (Apple M4)")
    fig.tight_layout()
    fig.savefig(os.path.join(PLOTS, "vectorized_ops.svg"))
    plt.close(fig)


def main():
    os.makedirs(PLOTS, exist_ok=True)
    plot_ensemble_scaling()
    plot_vectorized_ops()
    print("wrote", PLOTS, "*.svg")


if __name__ == "__main__":
    main()
