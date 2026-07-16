# Changelog

All notable changes to stochadex are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project aims to follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

**Versioning policy.** stochadex is deliberately pre-1.0 (`v0.x`). Under SemVer, a
`v0.x` project may make breaking changes in a **minor** bump. This is honest about where
the project is: the trust/CI work and the ongoing promotion of recurring domain-model
extensions into the core **will** break the API, and `v0` signals that callers should pin
an exact version rather than assume stability across minors.

> **Reconstruction note (read before trusting version boundaries below).**
> This project ran for ~4 years (Jul 2022 → Jul 2026, ~669 commits) with **no release
> tags** and almost entirely linear development on `main`. There were no real historical
> releases to recover. Rather than fabricate a version lineage, everything before the first
> real tag is recorded honestly under **[Pre-versioning history](#pre-versioning-history)**
> as a narrative reconstructed from commit messages — grouped by capability, not presented
> as releases. The first *real* tag is `v0.1.0` at the current state; earlier history is
> narrative only, deliberately not tagged.

## [Unreleased]

### Added
- **Opt-in Apache Arrow egress (`pkg/arrowstore`, a separate module).** An Arrow-native
  `simulator.OutputFunction` + storage (`ArrowStateTimeStorage`) at the output boundary, kept
  in a **separate Go module** so Arrow and its gonum-v0.17 requirement stay entirely out of the
  engine's `go.mod` (the engine stays lean and WASM-clean; opt in by importing it). It builds
  Arrow arrays directly — one contiguous `FixedSizeListBuilder` per partition (lock-free) and a
  shared deduplicated time column — so output lands ready for DuckDB/Polars/pandas with no
  conversion pass. Measured (M4): getting output *into Arrow* is ~2.2–2.7× faster with far fewer
  allocations than appending to `StateTimeStorage` and converting; the append hot path itself
  trades a constant allocation count (a GC-pressure win) for higher transient memory, so it is
  **interchange-optimized, not a general-purpose faster store** — the pure-Go `StateTimeStorage`
  stays the default. Foundation for the analytical-sink integrations (DuckDB next).
- **`benchmarks/`** — reproducible, fair CPU-to-CPU performance benchmarks with committed
  numbers and plots (Apple M4 reference machine): ensemble scaling (independent simulations
  via `RunSeededEnsemble` are embarrassingly parallel — ~4.4× on 10 heterogeneous cores),
  warmup-free cold-start (~2 µs to first result), whole-process simulation vs NumPy across
  **every execution model** (ensemble wins; branching processes favour the engine),
  linearly-coupled (~parity) and branching-coupled (~32× over idiomatic NumPy, 2.7× over
  hand-optimized) chains, execution-strategy regimes (where `Inline`/`SpawnPerStep`/
  `PersistentWorker` each win), and per-partition vector-op throughput vs NumPy (AXPY
  parity; DOT via the `cblas` backend below), and stock-vs-tuned single-core comparisons
  showing the single-core gap vs NumPy is mostly the *stock* iterations, recoverable in
  pure Go: OU (§3a) ~3.7× to NumPy parity, and the branching-coupled system (§3c-tuned)
  0.55×→0.90× of hand-optimized gather NumPy — by hoisting param slices, owning one RNG,
  and sampling gamma inline via Marsaglia–Tsang instead of the stock per-element map lookups
  and per-draw `distuv` allocation. Deliberately not a GPU-framework race.
- **Opt-in accelerated BLAS backend (`cblas` build tag).** `pkg/simulator/blas_accelerated.go`
  registers gonum's netlib backend against a linked system C BLAS (Apple Accelerate,
  OpenBLAS, or MKL) via a one-line `blas64.Use(...)` in `init()`, gated behind
  `//go:build cblas`. It lifts BLAS-heavy ops for anyone building with `-tags cblas` — no
  code change, just the flag (measured DOT ~2.7 → ~107 GFLOP/s at cache-resident sizes,
  matching/edging NumPy's Accelerate). The default build
  stays pure-Go and **WASM-clean** (Invariant B): cgo accelerators never sit on the default
  path.
- **"When to use it" on the docs frontpage** — a short, defensible positioning section:
  the combination stochadex uniquely offers in Go, and links ceding the ground it doesn't
  hold (Stan/PyMC/SciML, `godes`, gonum, Python for neural-net training).
- **Frontpage status badges.** Version (from the latest git tag), CI status, and test
  coverage badges on the docs frontpage. Coverage is published to Codecov from CI
  (`go test -coverprofile` → `codecov/codecov-action`); version and CI use shields.io.
  (Superseded the short-lived self-hosted-SVG badge approach.)

### Changed
- **Iteration hot-loop performance.** Two bit-identical optimisations to the stochastic
  iterations (same seed → same stream; all unit tests and model card numbers unchanged):
  1. **Hoisted per-dimension `params.GetIndex(name, i)` reads** (each a string-keyed map
     lookup) out of the per-element loops in `OrnsteinUhlenbeck(Exact)`,
     `GeometricBrownianMotion`, `WienerProcess`, `DriftJumpDiffusion`, `CompoundPoissonProcess`,
     `PoissonProcess`, plus `CopyValues` and `GroupedAggregation` — each param slice is now read
     once per step and indexed directly. This is the dominant win: ~1.7× for one-param
     iterations, ~3.7× for three-param ones (OU: 0.36 s → 0.10 s over 10,000 paths × 2,000 steps).
  2. **New `pkg/rng.Sampler`** — a small owned-`math/rand/v2` sampler (with its own `doc.go`)
     that the stochastic draws now use instead of `distuv.X{Src}.Rand()`, skipping distuv's
     per-call value-copy and wrapper construction (and, for the compound distributions, its
     bound-method-value indirection) for a further ~7–13% on the draw. It covers Normal,
     Uniform, Exponential, **Gamma, Beta, and Poisson** — the last three reproduce distuv's
     exact algorithm (Marsaglia–Tsang / Liu–Martin–Syring gamma; two-gamma beta; direct/PTRS
     Poisson), so every draw is **bit-identical** to distuv for the same seed, guaranteed by
     `pkg/rng`'s stream-identity tests. Applied to the Normal/Uniform iterations, the
     `CompoundPoisson` gamma jump, and the Gamma/Beta/Poisson/NegativeBinomial likelihood
     samplers (which keep distuv for `LogProb`, using the Sampler only for `GenerateNewSamples`).
     gonum's `math/rand/v2` distuv doesn't allocate, so this is throughput, not allocations.
     Binomial (a three-branch algorithm, one site) and Categorical (a stateful sampling heap)
     stay on distuv — the copied-algorithm cost there outweighs the small per-draw saving.
- **Multivariate likelihood-gradient performance.** `EvaluateLogLikeMeanGrad` on the
  `Normal`, `T`, and `Wishart` likelihood distributions re-factorised the covariance/scale
  matrix (O(d³) Cholesky, plus a matrix inverse for Wishart) on **every call** — and the
  gradient iteration calls it once per row of a data batch that all share one covariance.
  The factorisation now happens once per parameterisation (cached, invalidated in
  `SetParams`, recomputed lazily so the log-like and sampling paths never pay for it) and is
  reused across the batch. **Bit-identical** (deterministic factorisation, no RNG; all tests
  and model card numbers unchanged); ~5× faster at batch depth 10, ~8× at depth 50. (For
  these multivariate distributions the Cholesky, not the RNG draw, is the cost — so they are
  left on gonum's `distmv`/`distmat` rather than moved to `pkg/rng`.)
- Renamed the generated "Cross-model index" page to **"Domain model index"** (heading, docs nav, and page title).
- **Docs pipeline reliability.** CI now explicitly requests a GitHub Pages build after
  force-pushing `gh-pages` — a force-push doesn't reliably auto-trigger a Pages
  redeploy (and rapid successive publishes get throttled), which could leave the live
  site stale even though `gh-pages` was current. Generated docs output
  (`docs/index.html`, `docs/pkg/`, `docs/sitemap.xml`, `docs/robots.txt`,
  `docs/model-index.json`) is now gitignored — CI builds it for `gh-pages`, so `main`
  holds only sources.

## [0.2.0] — 2026-07-13

The trust layer: every published card claim is now bound to an enforced test and
every card number is generated from the code, across all nine catalogue models, made
checkable by a generated cross-model index — on top of the CI, docs-automation, and
versioning foundation. (Phase 1 of the improvement plan — the credibility spine.)

### Added
- **Generated card numbers (flagship: anglersim).** A model's card now shows an
  "Observed behaviour" table whose numbers are emitted by the model's own
  expected-behaviour suite and rendered into `card.md` by `cmd/model-graphs`, never
  hand-typed. `models/cardgen` holds the shared `Claim`/`Observation` types;
  `anglersim.ObservedBehaviour()` is the single source of both the test assertions
  and the card numbers, so the card cannot show a value the test did not observe.
  `TestCardsUpToDate` fails CI if the committed numbers drift from the code.
- **Claim↔test binding on the card (flagship: anglersim).** The generated "Observed
  behaviour" table now renders every response claim as one bound object — the
  plain-language statement, a link to the exact test subtest that enforces it
  (`TestAnglersimExpectedBehaviour/<claim-id>`), and the number that test produced.
  A claim cannot appear without a test enforcing it, nor carry a number the test did
  not produce; a broken claim fails CI (the binding test on a sign break, or
  `TestCardsUpToDate` on a number move). Folded into the frozen card format in
  `models/CONVENTIONS.md` so new entries adopt it from birth.

- **Generated card numbers + claim↔test binding on all three other flagships.**
  `antimicrobial-resistance`, `floodrisk`, and `energy-balancer` each gain an
  `ObservedBehaviour()` and a bound "Observed behaviour" table, matching anglersim.
  `cardgen.Claim` gained threshold assertions (`Thresholds`) alongside monotone, and
  a testing-free `cardgen.Verify`, so sign/level claims (e.g. energy-balancer's
  net-seller `revenue > 0`, net-buyer `SoC > initial`) and difference-of-differences
  claims (AMR's "prescribing acts only through selection") bind the same way.

- **Generated card numbers + claim↔test binding on the five remaining models.**
  `bathing-water-forecaster`, `business-survival`, `homark`, `measles-risk-forecaster`,
  and `trywizard` each gain an `ObservedBehaviour()` and a bound "Observed behaviour"
  table — so **all nine catalogue models** now carry generated, test-bound card numbers
  with no hand-typed results, and the cross-model index shows every model behaviour-bound.
- **Cross-model index (`cmd/model-index`).** A generated view across all nine catalogue
  models — each model's core-package usage, the bespoke iterations beside its stub, and
  whether its behaviour claims are test-bound — derived by inspecting the real stubs, never
  hand-maintained. Makes the generality claim checkable (concrete core-package reuse:
  `pkg/simulator` ×9, `pkg/continuous` ×3, `pkg/general` ×2, `pkg/discrete` ×1) and surfaces
  the extension-promotion signal (bespoke concepts recurring across models). Published as a
  human page (`models/INDEX.md`, rendered onto the docs site) and a machine-readable artifact
  (`models/index.json`, served as `model-index.json`); `TestModelIndexUpToDate` guards it.

### Changed
- `cmd/model-graphs` now regenerates both the partition-wiring diagram and the
  observed-behaviour block; each flagship's behaviour helpers moved from `_test.go`
  into `behaviour.go` so they are shared by the tests and the card generator. The
  behaviour tests now consume `ObservedBehaviour()` and verify each claim with
  `cardgen.Verify` (one computation is the source of the assertions and the numbers).

## [0.1.0] — 2026-07-13

First tagged release, marking the current state of the engine: partition-based stochastic
simulation core, the continuous/discrete/general process libraries, kernels, online
Bayesian inference, decision-making agents (MCTS/UCT + MAST), post-simulation
analysis/storage, the `models/` domain-models catalogue, and the static dependency-graph
tool. See the [pre-versioning history](#pre-versioning-history) for how it was built; the
most recent additions that land in this tag are:

### Added
- **Continuous integration** (`.github/workflows/ci.yml`): full suite on every PR and on
  merge to `main` — `go build`, `go vet`, and `go test ./... -race -count=1` with a
  Postgres service container for the storage tests. Required status check on `main`.
- **Automated docs site**: the docs build (`docs/build.sh`, pandoc + gomarkdoc + `pkg/graph`
  wiring diagrams) runs in CI and, on merge, force-pushes the built site to the `gh-pages`
  branch of `stochadex/stochadex.github.io`, served by GitHub Pages.
- This `CHANGELOG.md`, and a forward discipline of one changelog entry per behaviour/
  API-changing PR.

### Fixed
- `docs/build.sh` portability for CI (Ubuntu/GNU tooling): BSD `sed -i ''` → `perl`;
  pandoc `--syntax-highlighting` → `--highlight-style`; pre-create `docs/pkg`; pass explicit
  `--repository.*` flags to gomarkdoc so source links are generated in CI.

---

## Pre-versioning history

These are **not** releases — they are a narrative of how
the engine was built, grouped by capability epoch. Dates are the span of each epoch's work.
Package boundaries were fluid in 2024 (`streamers`, `params`, `objectives`, `interactions`,
`actors` appeared then were folded away); only `simulator`, `api`, `continuous`, `discrete`,
`general`, `kernels`, `inference`, `analysis`, `keyboard`, `agents`, and `graph` survived —
treat the intermediates as internal, never shipped API.

### Agents, domain-models catalogue, graph, and CI (Apr 2026 → Jul 2026)
- **Added** `pkg/agents`: a full **MCTS** implementation (tree, config, rollout, run-search,
  apply-partition), **MAST** (aggregation + rollout partitions), and a generic
  `Environment[S, A]` with a tic-tac-toe reference environment — wired into the
  partition/channel model as the cycle-breaking worked example.
- **Added** the `models/` **domain-models catalogue**: data-free SDK stubs of real-world
  domains wired into engine CI (flagships antimicrobial-resistance, floodrisk,
  energy-balancer, plus further entries), each with four artifacts including a mandatory
  `behaviour_test.go`; conventions frozen in `models/CONVENTIONS.md`; `/new-model` skill.
- **Added** `pkg/graph`: static partition dependency graph from `ConfigGenerator`, deadlock
  detection, Mermaid/DOT rendering, and a graph CLI.
- **Removed (breaking)** the `template/` and `scripts/` scaffolding — replaced by the
  `models/` catalogue philosophy (the generative core lives here; inference, data, and the
  decision layer move downstream).

### Docs site, execution strategies, and inference polish (Jul 2025 → Apr 2026)
- **Added** the documentation site (quickstart, how-it-works, gomarkdoc-generated package
  docs, architecture diagrams) and `doc.go` package comments across packages.
- **Added** `simulator.ExecutionStrategy` with an **inline execution** option (no
  goroutines/channels) and seeded-ensemble running; a `StateHistory.NextValues` write buffer
  (copy-on-retain, large allocation reduction).
- **Added** modern inference methods: evolutionary-strategies sampler, warm starts,
  sequential Monte Carlo (SMC), OLS regression.
- **Fixed** correctness bugs in data handling (broadcast deep-copy, indexing corruption,
  reweighted sampling, `SetGlobalSeed`).

### Inference maturation (Nov 2024 → Jun 2025)
- **Added** grouped aggregation statistics (mean/var/cov), likelihood-comparison partitions,
  posterior estimation with burn-in and gradient descent, Gaussian-process regression, a GLM
  predictor, and a library of likelihood distributions with analytic gradients (Gamma,
  negative-binomial, normal, Poisson, t-distribution, Wishart, Beta); OLS regression.
- **Added** the reusable iteration **test harness**, extended to detect statefulness residue
  by running a simulation twice and comparing — the invariant still enforced today.
- **Added** the first integration-test suite in `test/`.
- **Changed (breaking)** the inference package: moved to a `params`-based signature; moved
  resampling from `pkg/inference` to `pkg/general`; removed several inference interfaces and
  the kernel-estimation path; deprecated the GP-gradient path in favour of the GLM path.

### The big split, analysis/storage, and config-generator (Sep 2024 → Nov 2024)
- **Added** `pkg/continuous`, `pkg/discrete`, `pkg/general` — the three-way split of the old
  `phenomena` package — plus new discrete iterations, grouped aggregations, and windowed
  weighted statistics.
- **Added** `pkg/keyboard` (real-time input) and `pkg/analysis` (CSV/DataFrame, SQLite and
  PostgreSQL storage, go-echarts plotting, log querying).
- **Added** the `ConfigGenerator` and `StateTimeStorage` (replacing the older histories store
  as the primary data container).
- **Removed (breaking)** `pkg/phenomena` and `pkg/actors`; **rewrote** `pkg/api` around
  partition naming; removed the outdated React dashboard app.

### API, kernels, inference, channel wiring (Feb 2024 → Sep 2024)
- **Added** `pkg/api` (template-based Go code generation from YAML, arg parsing),
  `pkg/kernels` (integration kernels + weighted statistics), and `pkg/inference` (posterior
  mean/covariance, log-normalisation, sampling/resampling); embedded (nested) simulations.
- **Changed (breaking)** inter-partition data flow to **channel-based downstream value
  passing** — the ancestor of today's `params_from_upstream` within-step wiring; narrowed the
  param data types; `.SetNextIncrement` → `.NextIncrement`.

### CLI, dashboard, and the `Configure` refactor (Jul 2023 → Feb 2024)
- **Added** the `cmd/stochadex` CLI (YAML-configured binary), a real-time React dashboard
  (since removed), the agents/environments abstraction, and a Docker container.
- **Changed (breaking)** every iteration to require a **`Configure` method** — the birth of
  the two-method `Iteration` interface (`Configure` + `Iterate`) still core today.
- **Changed** moved the rugby-match domain model out of the core `phenomena` package.

### Simulator core engine (Feb 2023 → Jul 2023)
- **Added** the partition-based execution engine: state/time histories, the coordinator
  (originally "manager"), a worker-pool concurrency model, termination conditions, and
  configurable timestep functions.
- **Added** the first concrete processes (Wiener, Ornstein–Uhlenbeck, compound Poisson, Cox,
  Hawkes, geometric and fractional Brownian motion) and a worked rugby-match simulation.
- **Changed (breaking)** removed the `State` type (state became plain `[]float64`);
  `TimestepsHistory` → `CumulativeTimestepsHistory`.
- **Fixed** a history-window overwriting bug in the core history mechanism.

### Pre-simulator design era (Jul 2022 → Feb 2023)
- Not engine code: LaTeX/Markdown design notes and a Python plotting sandbox working out the
  stochastic-process formalism (diffusions, Poisson noise, windowed history for noise
  dependencies) before any Go engine existed. The pivot to Go begins Feb 2023.

[Unreleased]: https://github.com/umbralcalc/stochadex/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/umbralcalc/stochadex/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/umbralcalc/stochadex/releases/tag/v0.1.0
