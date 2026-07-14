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

## 3. Per-partition vector-op throughput vs NumPy — CPU-to-CPU parity

The elementwise/reduction ops a partition does on its state, via gonum's (pure-Go) BLAS,
against NumPy (Apple Accelerate BLAS). The point is parity, not winning: you don't give up
vectorized throughput by being in Go.

![vectorized ops](plots/vectorized_ops.svg)

- **AXPY** (`y += a·x`, elementwise — what iterations actually do): gonum ~7–8 GFLOP/s vs
  NumPy ~3–6.7 GFLOP/s → **parity** (gonum is even ahead at small sizes; both are
  memory-bound at large sizes).
- **DOT** (reduction): NumPy's Accelerate BLAS is faster on cache-resident sizes; gonum's
  default pure-Go BLAS trails here. If a workload is DOT-heavy, gonum can be linked against
  a C BLAS (OpenBLAS/Accelerate) to close the gap — out of the box it is pure Go.

## Reproducing

From the repo root:

```bash
go run ./benchmarks          # Go: ensemble scaling, cold start, gonum vector ops -> results/*.json
python3 benchmarks/numpy_ops.py   # NumPy vector ops -> results/vectorized_ops_numpy.json
python3 benchmarks/plot.py        # render plots/*.svg from results/*.json
```

`results/*.json` and `plots/*.svg` are committed (measured on the reference machine above).
Re-running overwrites them with your machine's numbers.
