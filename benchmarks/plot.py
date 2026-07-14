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


def plot_processes():
    import numpy as np

    go = {d["process"]: d for d in load("processes_go.json")}
    npv = {d["process"]: d for d in load("processes_numpy.json")}
    procs = ["gbm", "ou", "compound_poisson"]
    pretty = {"gbm": "GBM (simple)", "ou": "Ornstein–Uhlenbeck", "compound_poisson": "compound-Poisson (branching)"}

    # (json key, legend label, colour) — NumPy first, then stochadex models.
    models = [
        (None, "NumPy — 1 thread (SIMD/paths)", GREY),
        ("single wide inline partition (1 core)", "sx: 1 wide inline partition (1 core)", "#cfe0c3"),
        ("one sim, N partitions, spawn-per-step", "sx: N partitions, spawn-per-step", "#a7c69a"),
        ("one sim, N partitions, persistent-worker", "sx: N partitions, persistent-worker", "#84ab72"),
        ("one sim, N partitions, inline (serial)", "sx: N partitions, inline (1 core)", "#6b9457"),
        ("ensemble, N inline members (all cores)", "sx: ensemble, all cores", GREEN),
    ]
    x = np.arange(len(procs))
    n = len(models)
    w = 0.8 / n
    fig, ax = plt.subplots(figsize=(11, 5))
    for i, (key, label, colour) in enumerate(models):
        if key is None:
            vals = [npv[p]["numpy_seconds"] for p in procs]
        else:
            vals = [go[p]["seconds"][key] for p in procs]
        ax.bar(x + (i - (n - 1) / 2) * w, vals, w, color=colour, label=label)
    ax.set_xticks(x)
    ax.set_xticklabels([pretty[p] for p in procs])
    ax.set_ylabel("seconds (lower is better)")
    ax.set_title("Whole-process simulation across execution models — 10,000 paths × 2,000 steps (Apple M4)")
    ax.grid(True, axis="y", alpha=0.3)
    ax.legend(fontsize=8, ncol=2)
    fig.tight_layout()
    fig.savefig(os.path.join(PLOTS, "processes.svg"))
    plt.close(fig)


def plot_coupled():
    import numpy as np

    seconds = load("coupled_go.json")[0]["seconds"]
    npy = load("coupled_numpy.json")[0]["numpy_seconds"]
    order = [
        ("__numpy__", "NumPy — 1 thread", GREY),
        ("single wide inline chain (1 core)", "sx: 1 wide inline chain (1 core)", "#cfe0c3"),
        ("one sim, N chains, spawn-per-step", "sx: N chains, spawn-per-step", "#a7c69a"),
        ("one sim, N chains, persistent-worker", "sx: N chains, persistent-worker", "#84ab72"),
        ("one sim, N chains, inline (serial)", "sx: N chains, inline (1 core)", "#6b9457"),
        ("ensemble, N inline chains (all cores)", "sx: ensemble, all cores", GREEN),
    ]
    vals = [npy if k == "__numpy__" else seconds[k] for k, _, _ in order]
    colours = [c for _, _, c in order]
    labels = [l for _, l, _ in order]
    fig, ax = plt.subplots(figsize=(9, 4.6))
    ax.barh(np.arange(len(vals)), vals, color=colours)
    ax.set_yticks(np.arange(len(vals)))
    ax.set_yticklabels(labels, fontsize=9)
    ax.invert_yaxis()
    ax.set_xlabel("seconds (lower is better)")
    ax.set_title("Coupled OU chain (4 within-step-coupled components) — 10,000 paths × 2,000 steps")
    ax.grid(True, axis="x", alpha=0.3)
    fig.tight_layout()
    fig.savefig(os.path.join(PLOTS, "coupled.svg"))
    plt.close(fig)


def plot_branch_coupled():
    import numpy as np

    go = load("branch_coupled_go.json")[0]["seconds"]
    npv = load("branch_coupled_numpy.json")[0]
    rows = [
        ("NumPy — idiomatic (mask: compute every path)", npv["numpy_mask_seconds"], GREY),
        ("NumPy — optimized (gather triggered paths)", npv["numpy_gather_seconds"], "#94a3b8"),
        ("sx: single wide inline (1 core)", go["single wide inline (1 core)"], "#cfe0c3"),
        ("sx: N systems, inline (1 core)", go["one sim, N systems, inline (serial)"], "#6b9457"),
        ("sx: ensemble, all cores", go["ensemble, N inline systems (all cores)"], GREEN),
    ]
    labels = [r[0] for r in rows]
    vals = [r[1] for r in rows]
    colours = [r[2] for r in rows]
    fig, ax = plt.subplots(figsize=(9.5, 4.2))
    ax.barh(np.arange(len(vals)), vals, color=colours)
    ax.set_yticks(np.arange(len(vals)))
    ax.set_yticklabels(labels, fontsize=9)
    ax.invert_yaxis()
    ax.set_xscale("log")
    ax.set_xlabel("seconds (log scale, lower is better)")
    ax.set_title("Branching-coupled system (hard to vectorize) — 10,000 paths × 2,000 steps")
    for i, v in enumerate(vals):
        ax.text(v * 1.05, i, f"{v:.2f}s", va="center", fontsize=8)
    ax.grid(True, axis="x", alpha=0.3, which="both")
    fig.tight_layout()
    fig.savefig(os.path.join(PLOTS, "branch_coupled.svg"))
    plt.close(fig)


def main():
    os.makedirs(PLOTS, exist_ok=True)
    plot_ensemble_scaling()
    plot_vectorized_ops()
    plot_processes()
    plot_coupled()
    plot_branch_coupled()
    print("wrote", PLOTS, "*.svg")


if __name__ == "__main__":
    main()
