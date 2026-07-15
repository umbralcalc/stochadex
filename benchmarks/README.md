# Benchmarks

Fair, **CPU-to-CPU** measurements of the systems-performance claims that are actually
stochadex's. Deliberately **not** a peak-FLOPs race against GPU frameworks (JAX, Julia
SciML) — those win on their own hardware and problem shapes, and comparing them on a
laptop CPU would be apples-to-oranges. See the docs frontpage
[**When to use it**](../docs/README.md#when-to-use-it) section for where those tools are the
right call.

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
| sims/sec | 4226 | 7705 | 11413 | 13245 | 14967 | 15614 | 15538 |

~3.7× at 10 workers — near-linear across the **performance** cores, with diminishing
returns as the slower efficiency cores and memory bandwidth are added (a heterogeneous
Apple-silicon effect; on a homogeneous many-core server this curve is flatter and higher).
Absolute throughput is well up on earlier numbers because each member is now a
recently-optimised iteration (§3a); with the per-member *work* smaller, the fixed
per-member cost that does not parallelise — a fresh `ConfigGenerator` plus seeding per
member — is a larger share, so the scaling *ratio* is slightly lower even as members/sec is
higher.

> **Important — this is the right place to measure concurrency.** Partitions *within one
> simulation* are step-synchronised (a barrier every step) for **coupled** components that
> exchange state each step. The within-sim strategies *do* parallelise those partitions
> (see §3, §"Execution strategies"), but the barrier caps the speedup below this barrier-free
> ensemble. **Decoupled, embarrassingly-parallel work is best run as an ensemble of separate
> simulations**, which is what this benchmark measures. Each member here is a single edge-free
> partition run under `InlineExecution` (no per-step goroutine spawn), with all parallelism at
> the ensemble level.

## 2. Cold start — warmup-free

Time from an unbuilt simulation to the first produced result (config assembly + first
step): **~1 µs**, stable run-to-run. A statically compiled Go binary has no interpreter or
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
| stochadex: 1 wide `Inline` partition (1 core) | 0.095 | 0.096 | 0.108 |
| stochadex: one sim, N partitions, `Inline` (1 core) | 0.096 | 0.097 | 0.109 |
| stochadex: one sim, N partitions, `SpawnPerStep` | 0.059 | 0.058 | 0.063 |
| stochadex: one sim, N partitions, `PersistentWorker` | 0.060 | 0.058 | 0.065 |
| stochadex: **ensemble, N `Inline` members, all cores** | **0.017** | **0.017** | **0.018** |

What the models teach — and why the freedom to choose matters:

- **The ensemble wins everything.** Independent paths are embarrassingly parallel; running
  them as an ensemble of separate simulations (no per-step barrier) uses every core and is
  fastest on all three — **4.8× / 5.5× / 14× faster** than idiomatic NumPy.
- **Single-core is now at NumPy parity.** After the iteration optimisations (§3a: param-slice
  hoisting + an owned RNG, shipped in the stock iterations), stochadex's *single-core* configs
  match NumPy on GBM (0.095 vs 0.081) and OU (0.096 vs 0.093), and **beat** it on
  compound-Poisson (0.108 vs 0.258, ~2.4×). Earlier these single-core rows trailed NumPy by
  2–4× on GBM/OU — that gap is gone.
- **Within one simulation, partition-parallelism helps — modestly.** Switching from `Inline`
  (serial) to `SpawnPerStep`/`PersistentWorker` parallelises the partitions within each step:
  OU 0.096 → 0.058 s (~1.6×). The per-step barrier caps it below the barrier-free ensemble, so
  for *independent* work the ensemble is the better parallel path; the within-sim strategies
  are for *coupled* work that needs the barrier. (See §"Execution strategies".)
- **vs NumPy, it depends on the process.** On simple, trivially-vectorizable processes (GBM,
  OU) it is now a single-core dead heat, and stochadex pulls away with cores. On the
  **branching** compound-Poisson, stochadex is ahead even single-threaded (0.108 s vs 0.258 s)
  and runs away with cores (0.063 s within-sim, 0.018 s ensemble) — masking + conditional
  draws are where vectorization loses.

Takeaway: the more complex or path-dependent the process, the better the engine looks — and
either way you have several execution models to tune, not one. Run *your* process.

### 3a. The single-core iterations are optimised — a hand-tuned copy no longer beats them

The single-core rows above were, until recently, 2–4× slower. The stock iterations were
written for clarity and paid two avoidable costs per path per step: three string-keyed param
**map lookups** (`params.GetIndex("thetas", i)` …) and a `distuv.Normal.Rand()` call that
copies the distribution and rebuilds an RNG wrapper on every draw. Neither is a property of
the coordinator, so both were fixed **in the engine**: the stock iterations now hoist their
param slices out of the loop and draw from an owned `math/rand/v2` generator
([`pkg/rng`](../pkg/rng)), staying pure-Go/WASM-clean (see the CHANGELOG, "iteration hot-loop
performance").

The proof it is fully applied: a hand-tuned `tunedOUIteration` — identical math, maximally
hoisted, owned generator — no longer beats the stock one. They are the same speed, and both
sit on NumPy:

| OU, 1 core, 10,000 paths × 2,000 steps | seconds | vs NumPy |
|---|---|---|
| stock `OrnsteinUhlenbeckIteration` (inline) | 0.095 | 0.98× |
| hand-tuned OU iteration (inline, pure-Go) | 0.095 | 0.98× |
| NumPy — 1 thread, SIMD over paths | 0.093 | — |

So the single-core rows throughout this doc *are* the optimised code — there is no single-core
overhead left to squeeze out of the `Iterate` body (the ~3.7× that tuning used to buy, e.g. OU
0.356 → 0.095 s, now ships by default). The residual vs NumPy is the batched-C-vs-scalar-Go RNG
floor from §5's DOT story; a vectorized native RNG behind a build tag, like `-tags cblas` for
BLAS, would close that too.

### 3b. Coupled systems — linear coupling

The above simulate *independent* paths. Coupled systems — where components exchange state
within a step — are what the partition coordinator exists for. Here each unit is a chain of
**4 Ornstein–Uhlenbeck components**, where component *j* mean-reverts toward component *j−1*'s
current-step value (a within-step `ParamsFromUpstream` edge). The NumPy version must
hand-order the same cross-dependencies (the four updates cannot be fused).

![coupled OU chain](plots/coupled.svg)

| execution model | coupled chain (s) | vs NumPy |
|---|---|---|
| NumPy — 1 thread | 0.381 | — |
| stochadex: 1 wide inline chain (1 core) | 0.399 | 0.95× |
| stochadex: one sim, N chains, inline (1 core) | 0.424 | 0.90× |
| stochadex: one sim, N chains, spawn / persistent | ~0.21 | ~1.8× |
| stochadex: **ensemble, all cores** | **0.115** | **3.3×** |

**~Parity single-core, a clear win with cores.** With the optimised iterations (§3a) a
single-core chain (0.399 s) matches NumPy (0.381 s) — earlier it was 1.48 s, so this is the
same ~3.7× iteration speedup landing on the coupled case. NumPy vectorizes *linear* coupling
fine (it is just sequential array reads), so there is no vectorization gap to exploit; stochadex
pulls ahead purely by using cores — the within-sim strategies parallelise the coupling to
~0.21 s (~1.8× over NumPy) and the ensemble to 0.115 s (**3.3×**).

The honest reading: for a *linearly*-coupled system it is now a genuine speed win via cores
(and a dead heat single-core), on top of the expressiveness of declarative wiring. The gap
widens further when the coupling is **hard to vectorize** — next.

### 3c. Branching-coupled — hard to vectorize, where the engine wins

Now the coupling has a **per-path conditional**: an OU driver, and a responder that does
expensive work (a sum of 30 gamma draws) **only when the driver crosses a threshold** (~7%
of path-steps). This is what SIMD-over-paths cannot do cleanly — it must either compute the
expensive branch for *every* path and discard ~93%, or gather the few triggered paths with
non-obvious index juggling. A scalar per-path `if` just takes the branch.

![branching-coupled](plots/branch_coupled.svg)

| implementation | coupled system (s) |
|---|---|
| NumPy — idiomatic (mask: compute every path, select) | 5.598 |
| NumPy — optimized (gather only triggered paths) | 0.466 |
| stochadex — 1 core, stock `distuv.Gamma` responder | 0.579 |
| stochadex — 1 core, owned-gamma responder | 0.518 |
| stochadex — within-sim, spawn-per-step | 0.244 |
| stochadex — **ensemble, all cores** | **0.129** |

**This is where the engine leads.** stochadex is **~43× faster than idiomatic NumPy** (the
version most people would write — masking computes the expensive branch for every path and
discards ~93%). Against a *hand-optimized* gather/scatter NumPy (0.466 s), stochadex wins with
cores — **3.6× as an ensemble, 1.9× with within-sim parallelism** — because a scalar per-path
branch has no wasted work and no gather overhead, and the code is far simpler (a plain `if`).

Single-threaded, the optimized NumPy still edges stochadex, and this is the one place the RNG
detail shows: the responder's 30 gamma draws per triggered path are the cost. With the stock
`distuv.Gamma` the single-core system runs 0.579 s (0.80× of gather); switching the responder
to an owned generator that samples gamma inline (Marsaglia–Tsang, verified bit-identical) takes
it to 0.518 s (**0.90× — near-parity**). That is not hypothetical: the engine's own jump
distributions (e.g. `CompoundPoisson`'s `GammaJumpDistribution`) already sample from
[`pkg/rng`](../pkg/rng), so real models get it. The last sliver is the batched-C-vs-scalar-Go
gamma floor (§5's DOT story), which the engine erases with cores anyway.

And this is a *mild* branch — one rare condition, one kind of expensive work; real coupled
models (regime switches, thresholded dispatch, event cascades, mutually-exciting processes)
branch far more, widening the gap.

Together, 3b and 3c are the honest rule: **linearly-coupled → NumPy vectorizes it, ~parity;
conditionally/branching-coupled → the engine's per-path model wins outright**, and that is
where real decision-support models live.

## 4. Execution strategies — where each shines

stochadex lets you choose how a simulation runs its partitions each step. Which strategy
wins depends on the workload — here are three regimes, each won by a different one. Bar
labels (and the last column) are heap allocations during the run — GC pressure, which matters
for sustained work even when wall-clock ties.

![execution strategies](plots/strategies.svg)

| regime | `Inline` | `SpawnPerStep` | `PersistentWorker` |
|---|---|---|---|
| few partitions, light work, many steps | **~0.000 s / 0 allocs** | 0.005 s / 32k | 0.005 s / 0 |
| many partitions, light work, many steps | **0.008 s / 0** | 0.094 s / 768k | 0.108 s / 0 |
| many partitions, **heavy** work | 3.56 s / 0 | 0.55 s / 38k | 0.57 s / 0 |

- **`Inline` shines on light work** (few *or* many partitions): no goroutine/channel overhead
  per step — 0.008 s vs 0.094 s in the many-light regime — and it is allocation-free and
  deterministic. This is why ensemble members (single partitions) run inline.
- **`SpawnPerStep` / `PersistentWorker` shine on heavy per-step work with many partitions:**
  they parallelise the partitions across cores — ~6.5× over serial inline (0.55 s vs 3.56 s).
- **`PersistentWorker` matches `SpawnPerStep`'s speed with near-zero allocations** — it reuses
  a worker pool instead of spawning a goroutine per partition per step (768k → 0 allocs in the
  many-light regime). For sustained or GC-sensitive runs, that is the one to pick.

Rule of thumb: **light or single-partition → `Inline`; heavy multi-partition →
`PersistentWorker`** (or `SpawnPerStep` for simplicity); and for embarrassingly-parallel
independent work, an **ensemble** of inline members beats all of them.

## 5. Per-partition vector-op throughput vs NumPy — CPU-to-CPU parity (micro)

A supporting micro-benchmark: the raw elementwise/reduction ops a partition does on its
state, measured three ways — gonum on its **default pure-Go BLAS**, gonum with the
[`cblas` build tag](../pkg/simulator/blas_accelerated.go) (linked against the **same Apple
Accelerate BLAS** NumPy uses), and **NumPy**. No engine involved — just confirming you
don't give up vectorized throughput at the primitive level by being in Go, and that the
one gap has a one-flag fix.

![vectorized ops](plots/vectorized_ops.svg)

Peak (cache-resident, n=100k), GFLOP/s:

| op | gonum default (pure-Go) | gonum `-tags cblas` (Accelerate) | NumPy (Accelerate) |
|---|---|---|---|
| **AXPY** (`y += a·x`) | 8.8 | **52.7** | 7.3 |
| **DOT** (reduction) | 2.7 | **106.7** | 80.0 |

- **AXPY** (elementwise): even the *default* pure-Go gonum is at parity with NumPy (~8.8 vs
  ~7.3). It is memory-bandwidth-bound, so at the largest sizes all three converge (~8
  GFLOP/s); the C BLAS pulls ahead only where the data is cache-resident.
- **DOT** (reduction): the only real gap on the default build — gonum's *pure-Go* DOT
  trails (~2.7 GFLOP/s) — but it is a one-line fix. Building with `-tags cblas` routes gonum
  to a linked system C BLAS — the *same* Accelerate/OpenBLAS NumPy uses — taking DOT to
  **~107 GFLOP/s** (a ~40× jump), matching and even edging NumPy's ~80. The tradeoff is cgo
  (no pure-Go/WASM binary), so it is opt-in and off the default path
  (see [`pkg/simulator/blas_accelerated.go`](../pkg/simulator/blas_accelerated.go)).

So on the **same** BLAS the two are neck-and-neck — gonum with `-tags cblas` matches or
beats NumPy on both ops; the choice is whether you want the pure-Go/WASM-clean default or
NumPy-class BLAS behind one build flag. (All three series measured back-to-back in one
session; DOT is the same libraries on both sides, so treat the small gonum-over-NumPy edge
as "equal, not a win.")

## Reproducing

From the repo root:

```bash
go run ./benchmarks                # Go engine suite: ensemble scaling, cold start, process/coupled/tuned-OU models -> results/*.json
go run ./benchmarks tuned          # just the stock-vs-tuned comparisons (§3a and §3c) -> results/tuned_{ou,branch}_go.json

# Vector-op micro-benchmark (§5), the only BLAS-dependent one — two builds:
go run ./benchmarks/vectorops                                            # gonum default pure-Go BLAS -> vectorized_ops_go.json
CGO_ENABLED=1 CGO_LDFLAGS="-framework Accelerate" \
  go run -tags cblas ./benchmarks/vectorops                             # gonum + Accelerate (cblas) -> vectorized_ops_go_cblas.json
                                                                         #   (Linux: CGO_LDFLAGS="-lopenblas")

python3 benchmarks/numpy_processes.py      # NumPy whole-process comparison -> results/processes_numpy.json
python3 benchmarks/numpy_coupled.py        # NumPy coupled-chain comparison -> results/coupled_numpy.json
python3 benchmarks/numpy_branch_coupled.py # NumPy branching-coupled comparison -> results/branch_coupled_numpy.json
python3 benchmarks/numpy_ops.py            # NumPy vector-op micro-benchmark -> results/vectorized_ops_numpy.json
python3 benchmarks/plot.py                 # render plots/*.svg from results/*.json
```

`results/*.json` and `plots/*.svg` are committed (measured on the reference machine above).
Re-running overwrites them with your machine's numbers. The three §5 series
(`vectorized_ops_go.json`, `..._go_cblas.json`, `..._numpy.json`) are measured
back-to-back in one session so they are mutually comparable; the `-tags cblas` build needs
a system BLAS linked (Accelerate on macOS, OpenBLAS/MKL on Linux).
