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

## [0.5.0] — 2026-07-18

The whole engine becomes drivable as data. 0.4.0 made a single partition's *update*
expressible without Go (`expressions:`); this release extends that to everything a run
is — its iterations, simulation controls, run mode, analysis/inference/optimisation
macros, and data sources — so a config that names no Go anywhere resolves and runs
**in-process with no toolchain**. This was chosen over designing a bespoke modelling
language: experiments (a fresh agent authoring working configs from a doc alone,
including the mutual-coupling deadlock case) showed YAML passes because agents already
know it, a notation was no shorter, and the one thing a grammar buys — catching bad
wiring — was already built in `pkg/graph` and merely unwired.

### Added
- **Iteration registry — `iteration: {type: wiener_process, ...}` (`pkg/api`).** 35 of the
  framework's iterations are constructible as data: 21 data-only (their numeric parameters come
  from `params:`, so the spec carries no fields) and 14 composable, whose interface- or func-typed
  fields nest recursively — a kernel, likelihood, jump distribution, prior, nested iteration, or a
  framework-shipped named function (`{type: data_generation, likelihood: {type: normal}}`, the
  recursive `product` kernel, etc.). Two drift tests keep it honest: every registered name
  constructs the type it claims, and a `go/ast` scan requires every `Iterate`-implementing type in
  the candidate packages to be registered or excluded with a reason — so a new iteration fails CI
  until it is classified. A behaviour-equivalence test proves a data-spec iteration produces output
  *identical* to its Go-expr twin, not merely similar.
- **Simulation controls as data (`pkg/simulator`).** The four `SimulationConfig` families
  (output condition/function, termination, timestep) each gain a `{type: ...}` data spelling
  resolved at load. `simulator.RegisterComponent` lets a package downstream of `simulator` add its
  own (e.g. the embedded-window `from_history` timestep), so the registry is not closed to the rest
  of the module.
- **`run:` tier (`pkg/api`).** `{mode: batch | ensemble, seeds, concurrency}` — the one construct
  the partition tiers cannot express (what the coordinator *does* with the assembled generator).
  `ensemble` runs one seeded member per seed via `simulator.RunSeededEnsemble`.
- **In-process execution.** A fully-data config (no `iteration:` string, `extra_vars`, or
  Go-expression simulation component, in the main run and every embedded run) is detected and run
  with no code generation and no `go run`. Measured with `go` absent from `PATH`.
- **`api.CheckForDeadlock`, wired into `api.Run`.** Pre-flights the within-step
  (`params_from_upstream`) dependency graph via `pkg/graph`, turning the opaque
  `all goroutines are asleep - deadlock!` hang into a located error naming the cyclic partitions.
- **Macro / `analysis:` tier (`pkg/api`).** Analysis is nine partition-set-producing constructors;
  a macro expands to a *set* of partitions rather than one. A `data:` tier (a sub-simulation, or a
  file/database source) produces a `StateTimeStorage`, and each macro's `AppliedX` — expressed as
  nested data — is expanded against it. Against-storage macros: `vector_mean`/`variance`/
  `covariance`, `grouped_aggregation`, `scalar_regression_stats`, `likelihood_comparison`,
  `likelihood_mean_function_fit`, and `posterior_estimation` (the full online-Bayesian model as one
  macro, equivalence-tested byte-identical to the hand-written constructor call). Live macros (run
  as a fresh simulation, no storage): `evolution_strategy_optimisation`, and `smc_inference` — whose
  inner particle model is a per-particle template (`{particle}` in partition names and upstream
  references is instantiated once per particle, with a deep-copied params map so particles cannot
  cross-contaminate).
- **`data:` file/database sources.** `data.source.csv`, `data.source.json_log`, and
  `data.source.postgres` load storage from a file or a table, instead of running a sub-simulation.
- **Example configs and a guard test.** `cfg/example_*_config.yaml` for the data-only, composition,
  ensemble, macro, posterior, SMC, evolution-strategy, and CSV-source paths, each exercised in-process
  by `TestExampleConfigsRun`. The full 220-line posterior-estimation model is also shipped as data
  (`cfg/example_inference_data_config.yaml`).

### Changed
- **`simulator.SimulationConfigStrings` and `api.PartitionConfigStrings` component fields are now
  `ComponentSpec` unions, not `string`.** A `ComponentSpec` decodes a YAML value as either a
  Go-expression string (the previous meaning, unchanged in behaviour) or a `{type: ...}` data spec.
  This is a breaking change for code that constructed those structs with bare string literals —
  wrap them as `ComponentSpec{GoExpr: "..."}`. Macro fields decode straight into typed specs (never
  an untyped map), which is load-bearing: YAML 1.1 coerces a bare `y`/`n`/`yes`/`no`/`on`/`off` to a
  boolean for `interface{}` targets but preserves the string for string fields, so a partition named
  `y` only survives typed decoding.
- **`simulator.PartitionConfig` gains an `IterationSpec` field** (the loaded `iteration:` value as a
  `ComponentSpec`); `api.ApiRunConfig` gains `Run`, `Data`, and `Macros`. Existing configs are
  unaffected — an omitted field is zero-valued and the Go-expression path behaves exactly as before.

### Boundary
- **`mcts_self_play` remains Go, by design.** Its `agents.Environment[S, A]`
  (`Legal`/`Apply`/`Terminal`/`Actor`/`Players` over generic types) is arbitrary game rules — not
  representable as data without a general-purpose language — and MCTS self-play is the decision
  layer, which the repo boundary assigns downstream. A `postgres` write path likewise stays Go
  (a live `*sql.DB`). Everything in the engine's own domain — generative forward models plus
  inferential/analysis/optimisation — is now data.

## [0.4.0] — 2026-07-17

### Added
- **`general.ExpressionIteration`: a whole partition specified as data (`pkg/general`).** The
  per-step update is given as string expressions rather than Go, so a model needs no
  compilation step and no Go toolchain — which is what lets a simulation be written by
  something that does not write Go. The update is a small DAG: ordered `Bindings` are named
  intermediates, then one `Outputs` expression per field. Everything is elementwise over
  vectors with length-1 broadcasting, so one expression serves a scalar partition and a
  10,000-element one. Expressions are parsed once at `Configure` by `go/parser`, so a
  malformed spec is a configuration error rather than something found mid-run. Draws come from
  `pkg/rng`, seeded exactly as a hand-written iteration's is, which is what makes a
  declarative model comparable to compiled Go *by value* rather than only in distribution.
  Deliberately not a general-purpose language: no assignment, no recursion, and the only
  repetition is a bounded comprehension, so an expression always terminates.
- **An `expressions:` root field on the API config (`pkg/api`).** Binds a declarative spec to a
  partition by name, so a partition may omit its `iteration` exactly as an embedded-run-backed
  one may. Unlike `iteration` — a Go expression requiring code generation — this is loaded
  straight from YAML and evaluated at run time. Wired on `RunConfig` rather than
  `ApiRunConfig`, so embedded runs get it too.
- **A declarative twin for every catalogue entry (9 of 9).** Each model is now also stated as
  data in a `declarative.yaml`, with an `expression_equivalence_test.go` proving it is the
  *same* model rather than one that behaves similarly. Where the streams align — the common
  case — agreement is exact to rounding (deviations of 0 to ~1e-14), so a twin reproduces its
  card's numbers, not merely their directions. Verification is two mandatory layers, because
  each is blind where the other sees: step-for-step catches a mis-stated formula, and re-running
  `ObservedBehaviour()` against the declarative build catches wrong wiring, params or state
  layout. Documented in `models/CONVENTIONS.md` §5.
- **DSL constructs, each added because a real model proved it was needed.** `sin`/`cos`/`erf`/
  `erfc` and `pi` (seasonality, and the Gaussian CDF a probit or exceedance probability needs);
  `slice`/`concat` (a block inside a flat vector, and its assembly); `width`; `lag(name, n)` (a
  partition's committed state *n* rows back, where a bare name gives only row 0); and
  `each(n, i, expr)`, the one construct that is not elementwise — element *i* may read element
  *i-1* (so a cohort ages), a lane's `where` is scalar and therefore lazy (so a switched-off
  lane draws nothing), and lanes run in order (so a lane's draws interleave as a loop's do).

### Changed
- **A config key that nothing reads is now rejected (breaking: `pkg/api` config loading).**
  `yaml.v2` ignores an unknown key in silence, so a typo, or a key left behind by an older
  schema, did nothing at all while looking load-bearing — `state_width` sat in every config in
  this repo doing exactly that (width comes from `init_state_values`), and `pkg/simulator`'s
  partition fixture still named a wiring schema that no longer exists. Strict parsing alone
  cannot express the rule, because the two views deliberately share one file and split its
  keys: the concrete view owns `params` and `seed` but has no `iteration`, the
  code-generation view owns `iteration` and `simulation` but has no `params`, and each rejects
  the other's. Neither is the whole schema; their union is. A key is therefore dead only when
  **both** views reject it, which is what is checked — needing no second copy of the schema to
  drift out of sync. Configs carrying a dead key now panic naming it, where they previously
  loaded and quietly ignored it.
- **Promotion triage is no longer a frequency test (`models/CONVENTIONS.md`).** The old rule —
  an extension recurring across several entries wants promoting — cannot fire until several
  stubs exist. "Can the DSL express it?" is decidable on the *first* model, and splits
  candidates in two: if it can, the bespoke Go is a convenience and promotion must be earned by
  a measurement; if it cannot, the engine has a real capability gap and one model proves it.
  All five gaps the catalogue surfaced are closed, and the four structural ones rhymed — every
  one was about structured access or per-lane control of draws, the axis a strictly elementwise
  evaluator gives up.
- **`models/*/behaviour.go` helpers take a `stubBuilder`** so the claim suite can be *pointed
  at* either assembly of a model rather than restated for each. `ObservedBehaviour()` keeps its
  no-arg signature, which `cmd/model-index` detects by AST.

### Fixed
- **A NaN index silently read element 0 on arm64 and panicked on amd64
  (`general.ExpressionIteration`).** Go leaves `int(NaN)` undefined: on arm64 it is 0, so a
  non-finite index passed the bounds check and read the wrong element, while on amd64 it is the
  most negative int and the same expression failed. An architecture-dependent wrong answer, in
  a repo whose cards claim architecture stability. Every float that becomes an index or a count
  is now checked. Found by writing a twin, not by running a model.
- **`antimicrobial-resistance`'s card overclaimed its mechanism.** It said the selection test
  "pins down *why* the stewardship lever works". It does not: prescribing reaches resistance
  both by direct conversion and by competitive release, and *both* are selection-gated, so
  switching selection off cannot separate them — deleting the model's stated causal heart
  passes the entire claim suite. The card now says selection is *necessary* rather than
  attributed, and names the test that does discriminate. The claim itself was always true, so
  no generated number moved.

## [0.3.0] — 2026-07-17

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
- **Opt-in DuckDB analytical egress (`pkg/duckdbstore`, a separate module).** Lands
  `arrowstore` output in DuckDB for SQL analytics, fed **zero-copy** via the DuckDB Go driver's
  Arrow `RegisterView` interface — `IngestToTable` registers the storage's finished Arrow record
  as a view and materialises it with one `CREATE TABLE AS SELECT` (a `time` column plus one
  `ARRAY<DOUBLE>` column per partition), no `[][]float64` round-trip. Both sides use
  `arrow-go/v18`, so the record crosses into DuckDB as shared arrays. Kept in its own module
  because, unlike the engine and `arrowstore`, it is **CGO and not WASM-compatible** (statically
  links the DuckDB C++ library); the driver's Arrow API sits behind its `duckdb_arrow` build tag,
  so this package does too — without the tag nothing pulls in DuckDB or cgo. Edge/server only,
  never core, never on the default path.
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
- **Every execution strategy is now steppable (breaking: the `ExecutionStrategy`
  interface).** Execution strategies previously owned the whole run loop via a single
  `Run(c)` method, so selecting `PersistentWorkerExecution` or `InlineExecution` silently
  gave up the step-by-step driving the default algorithm supports — the interactive,
  keyboard, websocket, and embedded paths, plus the harness's per-step checks, all fell back
  to default execution. The interface's single primitive is now `NewStepper(c) Stepper`
  (`Stepper` is `{ Step(); Close() }`), which holds whatever per-run state the policy needs
  (e.g. persistent worker goroutines) and advances exactly one committed tick per `Step`.
  Both batch `PartitionCoordinator.Run` and the new stepwise
  `PartitionCoordinator.NewStepper` are expressed in terms of it, so **any** strategy can be
  driven one step at a time exactly as the default can — dropping `Run` from the interface
  makes steppability structural rather than per-strategy. The test harness now runs its
  per-step correctness checks (params mutation, NaN, state width, history integrity) under
  *every* strategy instead of only the default, and the websocket run path honours the
  configured strategy. Output stays byte-identical and performance is unchanged (benchstat
  over 8 runs: timing within run-to-run noise, allocations flat — the one extra `Stepper`
  allocation is per-run, not per-step). `PartitionCoordinator.Step(wg)` is retained for the
  default single-step path.
- **Quickstart rewritten to lead with a win (2.4).** The quickstart now opens with a complete,
  runnable ~25-line Go program (a recorded random walk that prints its output) before any
  partition/iteration/history vocabulary — then backfills that worldview, points at where
  results flow (CSV/DB/Arrow → pandas/DuckDB), and demotes the CLI/YAML/Docker path to a
  secondary section. Directly targets the plan's "biggest bounce risk is the mental model —
  lead with a win, explain second." The example is verified to run and produce the shown output.
- **Clean `database/sql` write path (2.3.c).** `analysis.PostgresDb` now accepts a
  caller-provided `*sql.DB` (new exported `DB` field + `NewPostgresDb(db, table)` constructor);
  `OpenTableConnection` only opens a local Postgres from `User`/`Password`/`Dbname` when no
  handle is supplied. So output/read can target **any Postgres-wire database** — a remote
  TimescaleDB or QuestDB with host/port/sslmode, or a pooled `*sql.DB` — through the interfaces
  already owned, no bespoke connector. The credential-based path is unchanged (back-compatible).
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

[Unreleased]: https://github.com/umbralcalc/stochadex/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/umbralcalc/stochadex/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/umbralcalc/stochadex/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/umbralcalc/stochadex/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/umbralcalc/stochadex/releases/tag/v0.1.0
