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
  stochadex core matches idiomatic NumPy's SIMD-over-paths; on a branching process it is
  already ahead.
- **All cores: several× faster.** Independent runs as a barrier-free ensemble use every core —
  **~5–14× over NumPy** across processes.
- **Hard-to-vectorize coupling: the engine wins outright.** Where the work has per-path
  conditionals — the case SIMD handles badly — stochadex is **~43× faster than idiomatic NumPy**
  and beats even a hand-optimized gather/scatter.
- **Warmup-free.** ~1 µs from an unbuilt simulation to the first result — a statically compiled
  binary with no interpreter or JIT to warm up.
- **Built-ins over expressions.** A built-in iteration runs the same maths as an `expression`
  **~4× faster** (up to 9× for a single process), so reach for one first.

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

## Macros run as one simulation

A `macros:` config is a shortcut: it expands into ordinary partitions that run as **one**
simulation (`api.Expand`). Because the expansion is an ordinary run, its execution strategy can
be chosen like any other's, which makes most of the shipped examples faster:

| Config (macros) | Run by default | Fastest strategy |
|---|---|---|
| `example_macro_config` (two chained aggregations) | 14.8 ms | inline, 10.8 ms |
| `example_regression_config` (`scalar_regression_stats`) | 3.4 ms | inline, 2.0 ms |
| `example_posterior_macro_config` (`posterior_estimation`) | 385 ms | inline, 339 ms |
| `example_state_space_count_config` (count filter + SMC) | 347 ms | level |
| `example_planning_config` (`mcts_planning`) | 118 ms | level |
| `example_evolution_strategy_config` | 38.3 ms | persistent worker, 34.6 ms |
| `example_smc_config` (`smc_inference`) | 21.2 ms | level |
| `example_mcts_config` (`mcts_self_play`) | 4.4 ms | persistent worker, 4.1 ms |
| `example_data_source_config` (aggregation over a CSV) | 3.1 ms | inline, 2.1 ms |

Medians of six runs on an Apple M4, each run reading its inputs afresh. The "fastest strategy"
column sets the expanded run's `execution_strategy`. `BenchmarkMacroConfigs` in `pkg/api`
reproduces it.

Run as `api.RunToStorage` and the CLI run it, a `macros:` config expands first, once per
loaded config, so repeated runs of one config cost what the table shows. A single load and run,
as each CLI invocation is, pays for expanding once: 0.3 ms for the regression example, 0.5 ms
for the CSV one, 0.9 ms for self-play and 2 ms for SMC, which is 8% of these short examples'
run time (geometric mean) and within 3% for the long ones. It is a fixed cost when the config
loads, not per step. `BenchmarkMacroConfigsFirstRun` measures a load and run.

## Built-in iterations beat the same maths as an expression

The `expression` iteration lets a config state any update as maths, but it is interpreted,
where a built-in iteration is compiled Go. So when a built-in, or a chain of them wired with
`params_from_upstream`, already does what a model needs, it is the faster choice, often by a lot.
Each pair below is one process written both ways, checked to compute the same thing (the same
draws in the same order, so equal to rounding, step by step) and timed over 1,000 steps, with
nothing written out:

| Process | Width | Built-in | Expression | Built-in is faster by |
|---|---|---|---|---|
| `wiener_process` | 1 | 0.77 ms | 3.96 ms | 5.2× |
| | 100 | 1.58 ms | 10.9 ms | 6.9× |
| `ornstein_uhlenbeck` | 1 | 0.78 ms | 3.97 ms | 5.1× |
| | 100 | 1.59 ms | 11.7 ms | 7.3× |
| `geometric_brownian_motion` | 1 | 0.77 ms | 7.25 ms | 9.5× |
| | 100 | 1.59 ms | 14.1 ms | 8.8× |
| `poisson_process` | 1 | 0.77 ms | 6.61 ms | 8.6× |
| | 100 | 1.87 ms | 13.8 ms | 7.4× |
| A Cox process: a rate partition feeding `poisson_process`, against one expression | 1 | 4.24 ms | 8.48 ms | 2.0× |
| | 100 | 6.12 ms | 14.9 ms | 2.4× |
| A running mean: `values_function_cumulative_mean` | 1 | 3.51 ms | 6.56 ms | 1.9× |
| | 100 | 8.44 ms | 11.6 ms | 1.4× |
| A 20-row rolling mean: `values_function_vector_mean` | 1 | 5.73 ms | 9.70 ms | 1.7× |
| | 100 | 15.8 ms | 20.5 ms | 1.3× |

Medians of six runs on an Apple M4, run by default (one goroutine per partition per step).
Across all fourteen, the built-in is 3.9× faster (geometric mean), and the expression also
allocates more, up to 8× as many bytes at width 100. The gap is widest for a single process,
where the arithmetic is most of the step; where a model has several partitions, the
coordinator's work each step is a larger share of the whole, and the gap narrows.

Two practical points follow. Look in `stochadex catalogue` for a built-in before writing an
expression, and factor a model to use them: the Cox process above is two building blocks, a
rate and a plain Poisson process, wired by `params_from_upstream`. And an expression remains the
right tool for maths no built-in states. `BenchmarkBuiltinVsExpression` in `pkg/api` reproduces
the table, and `TestBuiltinsAndExpressionsCompute` checks each pair agrees.

## Reproduce it

Every number here is committed and regenerable on your own hardware. See
[`benchmarks/`](https://github.com/umbralcalc/stochadex/tree/main/benchmarks) for the full
tables (including the coupled-chain and execution-strategy benchmarks), the methodology, and the
exact commands.
