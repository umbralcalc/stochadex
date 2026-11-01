# Plan: building blocks first, one implementation each

Status: **proposed** (2026-11-01), successor to the config-runtime plan (complete, branch
`claude/config-runtime-plan`).
- **Where this lives:** the long-lived branch `claude/building-blocks-plan`, never merged to
  `main`. Implementation PRs branch from `main` and link here. Code, comments and docs never
  refer to this plan (CLAUDE.md, "Code style").
- **Working rules carried over:** every PR is tested against independent references through the
  public path, mutation-checked, at 100% patch coverage (per package, no `-coverpkg`), with no
  per-step slowdown; goldens change only under the re-record policy (§4). The user reviews and
  merges each PR; downstream repos have no CI, so their tests run locally.

| | State |
|---|---|
| A. Docs and docstrings meet the repo's standards | In progress (session `docs-standards-sweep`) |
| "One simulation, run: says how many times" wording | **In review (#175)** |
| B. Evidence: expression versus built-in cost | To do |
| C. The catalogue says what each iteration computes | To do |
| D. Suggesting built-ins for expressions | To do, after B and C |
| E. One implementation per macro | To do |
| F. Retire deprecated spellings; decide on flattening `main:` | To do, with robert |
| R. Authoring gaps robert surfaces (standing) | R.2, R.3 open |
| robert's half of building blocks first | In progress (session `robert-builtins-first`, robert repo) |

## 0. Where the config language stands (assessment, 2026-11-01)

The question: how fully does one YAML config language now realise the engine's wiring
flexibility and performance?

**Wiring: essentially complete.**
- Every `Iterate` type in the engine has a `{type: ...}` spelling except two, the MAST rollout
  and aggregation iterations, which nothing uses yet (`excludedIterations` in
  `registry_coverage_test.go`).
- All three ways partitions read each other are config: `params_from_upstream` (within the
  step), `params_as_partitions` and expression `upstreams:` (one step behind).
- Nesting is config: `embedded_simulation_run`, `replicated_simulation_run` and top-level
  `embedded:`, to any depth.
- Inputs (sources, pre-pass simulations, nested runs, files, live streams), outputs (views over
  every sink), and run modes (batch, ensemble, serve, sharding) are all config.
- The one deliberate boundary: decision rules (an `agents.Environment`) stay Go, named through
  `RegisterEnvironment`. Planning over a forward model (`mcts_planning`) is config.

**Performance: the engine's speed is reachable, but nothing steers authors to it.**
- An expanded macros config runs level with hand-wired Go, and `execution_strategy` (inline,
  persistent worker) is selectable from the config, which makes most shipped examples faster.
- The gap is in authoring. Built-in iterations are compiled Go; the `expression` DSL is
  interpreted. Nothing measures the difference, nothing tells an author (person or agent) that a
  built-in exists for what they wrote, and nothing shows how to factor a model into building
  blocks wired by upstreams. Every catalogue model's declarative twin is all expressions, by
  design (the twin tests that a model *can* be data), so no shipped model shows the
  building-block style either.
- Composition can multiply cost invisibly (R.2).

**Documentation: now complete in shape.** The config reference states the semantics with every
example run by a test, and `stochadex catalogue` (version 2) gives each component's fields and
params, read from source and checked against every config in use. What it lacks is what each
iteration *computes*: a one-sentence description is not enough to recognise that a model's
update is `wiener_process` or `ornstein_uhlenbeck`.

**Untidiness worth removing.**
- **Two implementations of every macro.** `pkg/macros` constructors build each macro's
  partitions against storage, for Go callers (about 2,570 lines); `pkg/api`'s expanders write
  the same partitions as YAML (about 3,550 lines). Since the macro-by-macro tier was retired,
  only the posterior and mean-function-fit macros are checked against a direct build; the rest
  can drift apart unseen.
- **Deprecated spellings still carried:** `data:`, the `simulation.output_condition` /
  `output_function` pair, `--socket`, `api.RunEnsembleToStorage`,
  `api.LoadApiRunConfigFromYaml` (which panics rather than returning errors), and the
  `stochadex-graph` binary.
- **The `main:` level** (open question 5 of the old plan): flattening it waits on evidence from
  robert's authoring logs.

## 1. Items

### A. Docs and docstrings meet the repo's standards

In progress in its own session: every exported identifier documented, no plan or PR
references or change history in comments and docs, no agent-tooling content, no backticks in Go
doc comments, and claims checked against the code. One PR. Acceptance: the PR, `go vet` and the
docs build clean.

### B. Evidence: what an expression costs against a built-in

Measure before steering. Benchmark a set of common processes written both ways, through the
public path (`RunToStorage` on a config), interleaved, with benchstat:
- `wiener_process`, `ornstein_uhlenbeck`, `geometric_brownian_motion`, `poisson_process`;
- a cumulative sum and running mean (`cumulative`, `values_function_cumulative_mean`);
- a rolling window mean (`values_function_vector_mean`);
- a Cox process as built-ins (a rate partition feeding `poisson_process`'s `rates`) against one
  expression partition.

Then, for two or three catalogue models, write a **building-block twin**: the model factored into
built-ins wired by upstreams, with an expression only where no built-in fits, checked against
the model's existing oracle (exact where the draws allow, else claim-level) and benchmarked
against its expression twin.

Acceptance: results in `docs/performance.md`; the building-block twins in the catalogue with
their equivalence tests. The numbers decide how hard C and D push.

### C. The catalogue says what each iteration computes

Add each iteration's update rule to `stochadex catalogue`, as a structured field (for example
`update`: the rule, its state, and the params it uses), generated from source like the rest of
catalogue v2 and drift-tested. Many iteration doc comments already carry the rule in an
"Implementation" section (`X(t+dt) = X(t) + sqrt(variance * dt) * Z`); the rest get one, in
the code's own doc comment. Kernels, likelihoods and named functions get the same.

Acceptance: every built-in iteration has an update rule; a test fails if one is missing; the
config reference shows how to find a built-in for a given update.

### D. Suggesting built-ins for expressions

A check, in `--check` as a notice or in `inspect`, that recognises an expression partition (or a
small group of them) equivalent to a built-in, and names the replacement, including when the
replacement factors one partition into a built-in plus an upstream wire (a rate expression
feeding a Poisson draw becomes a rate partition feeding `poisson_process`). Precision over
recall: a short list of exact patterns, each tested both ways (it fires on the reimplementation,
and running the suggestion gives the same rows), never a guess.

Acceptance: the suggestions are machine-readable for robert (the robert session's gate can treat
one as a failure to fix); every shipped config and expression twin is either clean or its
suggestions are recorded and justified.

### E. One implementation per macro

First, the safety net: a test that builds every macro both ways (the `pkg/macros` constructor
against storage, and the expansion) and compares the runs bit for bit, so the two cannot drift.
Then consolidate. The likely direction is that `pkg/macros` constructors build by expanding the
macro's spec and resolving it (one definition, the expander), keeping their Go signatures for
library callers; measure the result before deciding. Acceptance: one definition of each macro's
partitions; the cross-check test kept as the guard; no change to any golden.

### F. Retire deprecated spellings; decide on flattening `main:`

Remove `data:`, the output pair, `--socket`, `RunEnsembleToStorage` and
`LoadApiRunConfigFromYaml` (or make it return errors), and stop building `stochadex-graph`, in
one v0.x minor with "Upgrading" notes. Coordinate with robert first: its config contract
(`model/partitions.yaml`) and recipes must move before the release. Flattening `main:` is decided
on robert's authoring logs (how often the `main:` level causes an error or a retry), not before.

### R. Authoring gaps robert surfaces (standing)

Each gap is reproduced as the smallest config that shows it, triaged (engine, error, or
robert's), and logged here with its PR.

| # | Gap | Triage | PR |
|---|---|---|---|
| R.1 | Estimating rates from a CSV: a window longer than its input returns its default forever; regression skipped the first row; per-step output too large | engine and error | #164 (done) |
| R.2 | Cost of composition is invisible until a run is slow: nesting depth and inner steps per outer step | engine: `inspect` reports them | to do |
| R.3 | An expression reading further back than its upstream's `state_history_depth` fails only when the run reaches it; `--check` passes it | error: check a literal lag against the depth at load | to do |

## 2. Order

1. A and the wording (#175), in flight.
2. B, the evidence.
3. C, then D, with robert's session consuming each as it lands.
4. E, starting with its cross-check test (useful on its own).
5. F, once robert has moved.
6. R items as they come; R.2 and R.3 whenever a slot is free.

Release around each contract-visible change (C's catalogue field, D's suggestions, F's
removals), verifying each release as before.

## 3. Policies carried over

- **Goldens** may be re-recorded only for an intended numerical change checked against an
  independent reference, or for a toolchain change shown to be compiler- or FMA-only; the PR
  says which and records both architectures (amd64 from CI's log).
- **Don't stack PRs** on a branch that will be squash-merged; build locally and open after the
  base merges.
- **Bugs found on the way** are proposed as their own sessions, not folded in.

## Appendix A. Progress log

- **2026-11-01: proposed.** After the v0.27.0 release, the user raised four follow-ups: a
  docs-standards sweep (A, started as its own session), the contradictory "one runtime" wording
  (fixed in #175), this assessment, and making robert prefer built-in iterations (robert's half
  started as its own session in the robert repo; the engine's half is B, C and D).
