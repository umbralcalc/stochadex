---
title: "Performance vs NumPy"
logo: true
---

# Performance vs NumPy
<div style="height:0.75em;"></div>

Fair, **CPU-to-CPU** measurements against idiomatic single-thread NumPy — the honest baseline
for the systems-performance claims that are actually stochadex's. This is deliberately *not* a
peak-FLOPs race against GPU frameworks (JAX, Julia SciML); those win on their own hardware and
problem shapes, and [**when to use it**](../index.html#when-to-use-it) says so plainly. Numbers
are from an **Apple M4** reference machine and are machine-specific — the [full benchmark suite
in the repository](https://github.com/umbralcalc/stochadex/tree/main/benchmarks) has every
result, the methodology, and one-command reproduction.

## The short version

- **Single-core: at NumPy parity.** On simple processes (GBM, Ornstein–Uhlenbeck) a single
  stochadex core now matches idiomatic NumPy's SIMD-over-paths; on a branching process it is
  already ahead.
- **All cores: several× faster.** Independent runs as a barrier-free ensemble use every core —
  **~5–14× over NumPy** across processes.
- **Hard-to-vectorize coupling: the engine wins outright.** Where the work has per-path
  conditionals — the case SIMD handles badly — stochadex is **~43× faster than idiomatic NumPy**
  and beats even a hand-optimized gather/scatter.
- **Warmup-free.** ~1 µs from an unbuilt simulation to the first result — a statically compiled
  binary with no interpreter or JIT to warm up.

## Whole-process simulation, across every execution model

The most representative test: simulate the *same* stochastic process — 10,000 paths × 2,000
steps — end to end, in NumPy (a step loop vectorized over paths, single thread) and in stochadex
across each execution model it offers.

<center><img src="plots/processes.svg" style="max-width:100%" /></center>

| seconds (lower is better) | GBM | Ornstein–Uhlenbeck | compound-Poisson (branching) |
|---|---|---|---|
| NumPy — 1 thread | 0.081 | 0.093 | 0.258 |
| stochadex — 1 core | 0.095 | 0.096 | **0.108** |
| stochadex — **ensemble, all cores** | **0.017** | **0.017** | **0.018** |

Single-core is a dead heat on GBM/OU and a **~2.4× win** on the branching process; the ensemble
is **4.8× / 5.5× / 14×** faster than NumPy. You choose the execution model that fits the
workload — NumPy gives you exactly one way to run.

## Where the engine pulls ahead: hard-to-vectorize coupling

Add a per-path conditional — a responder that does expensive work *only* when a driver crosses a
threshold (~7% of path-steps). SIMD-over-paths cannot do this cleanly: it must either compute the
expensive branch for *every* path and discard ~93%, or hand-write gather/scatter index juggling.
A scalar per-path `if` just takes the branch.

<center><img src="plots/branch_coupled.svg" style="max-width:100%" /></center>

stochadex is **~43× faster than the idiomatic NumPy** most people would write, and its ensemble
beats even a *hand-optimized* gather NumPy by **3.6×** — with far simpler code (a plain `if`).
This is where real decision-support models live: regime switches, thresholded dispatch, event
cascades, mutually-exciting processes.

## Primitive throughput isn't sacrificed either

Per-partition vector operations (via gonum) hold their own against NumPy's Accelerate BLAS, and
the one gap — DOT — closes with a single build flag that links the *same* C BLAS NumPy uses.

<center><img src="plots/vectorized_ops.svg" style="max-width:100%" /></center>

At cache-resident sizes AXPY is at parity by default; DOT jumps from ~2.7 to ~107 GFLOP/s with
`-tags cblas`, matching NumPy — so you keep the pure-Go / WASM-clean binary by default and get
NumPy-class BLAS when you opt in.

## Macros cost nothing as a single run

A `macros:` config is a shortcut: it expands into ordinary partitions that run as **one**
simulation (`api.Expand`), with output bit-identical to running each macro in turn over the
recorded data. Expanded, every shipped macro config runs level with the macro-by-macro path,
and choosing the run's execution strategy, which only the expanded form can, makes most of
them faster:

| Config (macros) | Macro by macro | One run, expanded | Fastest strategy |
|---|---|---|---|
| `example_macro_config` (two chained aggregations) | 14.8 ms | 14.8 ms | inline, 10.8 ms |
| `example_regression_config` (`scalar_regression_stats`) | 2.9 ms | 3.4 ms | inline, 2.0 ms |
| `example_posterior_macro_config` (`posterior_estimation`) | 375 ms | 385 ms | inline, 339 ms |
| `example_state_space_count_config` (count filter + SMC) | 377 ms | 347 ms | level |
| `example_planning_config` (`mcts_planning`) | 122 ms | 118 ms | level |
| `example_evolution_strategy_config` | 38.1 ms | 38.3 ms | persistent worker, 34.6 ms |
| `example_smc_config` (`smc_inference`) | 20.7 ms | 21.2 ms | level |
| `example_mcts_config` (`mcts_self_play`) | 4.4 ms | 4.4 ms | persistent worker, 4.1 ms |
| `example_data_source_config` (aggregation over a CSV) | 2.9 ms | 3.1 ms | inline, 2.1 ms |

Medians of six runs on an Apple M4, each run reading its inputs afresh. Across all of them the
expanded run takes 2% more time, within noise for most rows (benchstat, p > 0.05); the
data-source and posterior examples are 8% and 3% slower (p = 0.04), and the planning example
3% faster. Bytes and objects allocated are level, and peak heap is 3% lower. The "fastest
strategy" column sets the expanded run's `execution_strategy`, which the macro-by-macro path
has no way to choose. `BenchmarkMacroConfigs` in `pkg/api` reproduces it.

Run as `api.RunToStorage` and the CLI run it, a `macros:` config expands first, once per
loaded config, so repeated runs of one config cost what the table shows. A single load and run,
as each CLI invocation is, pays for expanding once: 0.3 ms for the regression example, 0.5 ms
for the CSV one, 0.9 ms for self-play and 2 ms for SMC, which is 8% of these short examples'
run time (geometric mean) and within 3% for the long ones. It is a fixed cost when the config
loads, not per step. `BenchmarkMacroConfigsFirstRun` reproduces it.

## Reproduce it

Every number here is committed and regenerable on your own hardware. See
[`benchmarks/`](https://github.com/umbralcalc/stochadex/tree/main/benchmarks) for the full
tables (including the coupled-chain and execution-strategy benchmarks), the methodology, and the
exact commands.
