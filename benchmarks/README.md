# Benchmarks

Fair, **CPU-to-CPU** measurements of the systems-performance claims that are actually
stochadex's. Deliberately **not** a peak-FLOPs race against GPU frameworks (JAX, Julia
SciML) — those win on their own hardware and problem shapes, and comparing them on a
laptop CPU would be apples-to-oranges. See [`WHEN_TO_USE.md`](../WHEN_TO_USE.md) for where
those tools are the right call.

## Reference machine

Numbers below were measured on **Apple M4 (10 cores: 4 performance + 6 efficiency),
macOS, Go 1.25, NumPy 2.4.3**. Benchmark numbers are machine-specific — reproduce on your
own hardware with the commands under [Reproducing](#reproducing). They are **not**
regenerated in CI: shared CI runners are not performance-stable, so committed numbers come
from this documented reference machine (CI only checks the benchmark still builds/runs).

## 1. Ensemble scaling — the parallelism claim

Independent simulations run as an **ensemble** (`simulator.RunSeededEnsemble`) are
embarrassingly parallel: there is no per-step barrier between members, so throughput
scales with `maxConcurrency` up to the core count.

![ensemble scaling](plots/ensemble_scaling.svg)

| workers | 1 | 2 | 3 | 4 | 6 | 8 | 10 |
|---|---|---|---|---|---|---|---|
| sims/sec | 2758 | 5204 | 7623 | 8661 | 10491 | 11043 | 12241 |

~4.4× at 10 workers — near-linear across the **performance** cores, with diminishing
returns as the slower efficiency cores and memory bandwidth are added (a heterogeneous
Apple-silicon effect; on a homogeneous many-core server this curve is flatter and higher).

> **Important — this is the right place to measure concurrency.** Partitions *within one
> simulation* are step-synchronised (a barrier every step) because they are for **coupled**
> components that exchange state each step; that is not the parallelism story and does not
> scale with partition count. **Decoupled, embarrassingly-parallel work is an ensemble of
> separate simulations**, which is what this benchmark measures. Each member here is a
> single edge-free partition run under `InlineExecution` (no per-step goroutine spawn), with
> all parallelism at the ensemble level.

## 2. Cold start — warmup-free

Time from an unbuilt simulation to the first produced result (config assembly + first
step): **~2 µs**, stable run-to-run. A statically compiled Go binary has no interpreter or
JIT to warm up — the warmup-free, single-binary deployment property, stated as an absolute
rather than a rigged race against a JIT stack.

## 3. Whole-process simulation across execution models — the engine in the comparison

The most representative comparison: simulate the *same stochastic process* — 10,000 paths ×
2,000 steps — end to end, in NumPy (idiomatic: a step loop vectorized over paths, single
thread) and in stochadex across **every execution model it offers**. This puts the whole
engine (coordinator + `Iteration` + ensemble) in the loop, and makes explicit *which model
wins, why, and that you are free to tune it* — where NumPy gives you exactly one way to run.
Neither side stores history (fair timing of the simulation compute).

![whole-process simulation across execution models](plots/processes.svg)

Wall-clock seconds (lower is better); **bold** = fastest per process:

| execution model | GBM (simple) | Ornstein–Uhlenbeck | compound-Poisson (branching) |
|---|---|---|---|
| NumPy — 1 thread, SIMD over paths | 0.081 | 0.093 | 0.258 |
| stochadex: 1 wide `Inline` partition (1 core) | 0.193 | 0.387 | 0.284 |
| stochadex: one sim, N partitions, `SpawnPerStep` | 0.110 | 0.172 | 0.130 |
| stochadex: one sim, N partitions, `PersistentWorker` | 0.110 | 0.170 | 0.127 |
| stochadex: one sim, N partitions, `Inline` (1 core) | 0.112 | 0.173 | 0.128 |
| stochadex: **ensemble, N `Inline` members, all cores** | **0.038** | **0.072** | **0.052** |

What the models teach — and why the freedom to choose matters:

- **The ensemble wins everything.** Independent paths are embarrassingly parallel; running
  them as an ensemble of separate simulations (no per-step barrier) uses every core and is
  fastest on all three processes — 2.2× / 1.3× / 5.0× faster than idiomatic NumPy.
- **Within one simulation, partition-parallelism gives ~no speedup** — `SpawnPerStep` ≈
  `PersistentWorker` ≈ `Inline` (all ≈ 0.11 s for GBM). Partitions in one sim are
  step-synchronised (a barrier every step) *for coupled components*; forcing independent
  paths through it does not scale. Lesson: run decoupled work as an **ensemble**, not as
  partitions of one sim. (stochadex lets you do either — the point is to pick the right one.)
- **Layout matters too:** N narrow partitions beat one wide partition even single-threaded
  (0.112 vs 0.193 s, GBM) — cache locality. Another free knob.
- **vs NumPy, it depends on the process.** On simple, trivially-vectorizable processes
  (GBM, OU) NumPy's SIMD over paths beats stochadex's single-core configs; stochadex wins by
  using cores. But on the **branching** compound-Poisson, stochadex is faster than NumPy
  **even single-threaded** (0.128 vs 0.258 s) — masking + conditional draws are where
  vectorization loses — and 5× faster in parallel.

Takeaway: the more complex or path-dependent the process, the better the engine looks — and
either way you have several execution models to tune, not one. Run *your* process.

### 3b. Coupled systems — where the engine is designed to lead (and a bottleneck it found)

The above simulate *independent* paths. Coupled systems — where components exchange
state within a step — are what the partition coordinator exists for. Here each unit is a
chain of **4 Ornstein–Uhlenbeck components**, where component *j* mean-reverts toward
component *j−1*'s current-step value (a within-step `ParamsFromUpstream` edge). The NumPy
version must hand-order the same cross-dependencies (the four updates cannot be fused).

![coupled OU chain](plots/coupled.svg)

| execution model | coupled chain (s) | vs NumPy |
|---|---|---|
| NumPy — 1 thread | 0.382 | — |
| stochadex: ensemble, all cores | 0.361 | 1.06× |
| stochadex: one sim, N chains, inline (1 core) | 0.577 | 0.66× |
| stochadex: one sim, N chains, spawn / persistent | ~0.57 | ~0.67× |
| stochadex: 1 wide inline chain (1 core) | 1.556 | 0.25× |

**Honest result, and a benchmark doing its job.** On a *linearly*-coupled chain, NumPy
vectorizes the coupling fine (it is just sequential array reads), so stochadex is only ~at
parity even using every core — and, tellingly, the coupled **ensemble parallelises poorly
(~1.6×, vs ~3× for independent processes)**.

The cause is measurable, not fundamental: under inline execution, forwarding an upstream's
output into a downstream's params allocates and copies the full state vector **every step,
per coupled edge** (`UpdateUpstreamParamsInline`: `append([]float64(nil), values…)`). For
this chain that is ~60M short-lived slice allocations, whose GC churn throttles the parallel
ensemble. The copy is intentional (a downstream must not mutate the producer's buffer) but
it need not *allocate* — reusing a per-edge buffer (the same copy-on-retain fix already
applied to the `NextValues` write path) would remove it. That is exactly the
allocation-at-the-boundary work Phase 2.3 targets; until then, this is a documented, honest
bottleneck rather than a claimed win.

## 4. Per-partition vector-op throughput vs NumPy — CPU-to-CPU parity (micro)

A supporting micro-benchmark: the raw elementwise/reduction ops (via gonum's pure-Go BLAS)
vs NumPy (Apple Accelerate BLAS). No engine involved — just confirming you don't give up
vectorized throughput at the primitive level by being in Go.

![vectorized ops](plots/vectorized_ops.svg)

- **AXPY** (`y += a·x`, elementwise): gonum ~7–8 GFLOP/s vs NumPy ~3–6.7 GFLOP/s → **parity**.
- **DOT** (reduction): NumPy's Accelerate BLAS is faster on cache-resident sizes; gonum's
  default pure-Go BLAS trails. gonum can be linked against a C BLAS to close it if needed.

## Reproducing

From the repo root:

```bash
go run ./benchmarks                # Go: ensemble scaling, cold start, gonum ops, process models -> results/*.json
python3 benchmarks/numpy_processes.py   # NumPy whole-process comparison -> results/processes_numpy.json
 python3 benchmarks/numpy_coupled.py     # NumPy coupled-chain comparison -> results/coupled_numpy.json
python3 benchmarks/numpy_ops.py         # NumPy vector-op micro-benchmark -> results/vectorized_ops_numpy.json
python3 benchmarks/plot.py              # render plots/*.svg from results/*.json
```

`results/*.json` and `plots/*.svg` are committed (measured on the reference machine above).
Re-running overwrites them with your machine's numbers.
