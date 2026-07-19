# Stochadex — Claude Code Conventions

## Project Overview

Stochadex is a Go SDK for building, configuring, and running simulations of stochastic
processes and complex systems. Its execution model is **partition-based**: a simulation is
a set of independent components ("partitions"), each advancing its own state each step, run
concurrently by a coordinator and wired together through a small set of well-defined
channels.

- **Module**: `github.com/umbralcalc/stochadex`
- **Entry point**: `cmd/stochadex/main.go` → `api.RunWithParsedArgs(api.ArgParse())`

## Repository map

```
pkg/
  simulator/   — Core engine: the Iteration interface, PartitionCoordinator, state/time
                 histories, output & termination conditions, ConfigGenerator, run harnesses
  api/         — YAML config loading, CLI arg parsing, code generation from templates
  continuous/  — Continuous-time processes (Wiener, GBM, Ornstein–Uhlenbeck, drift–diffusion…)
  discrete/    — Discrete / jump processes (Poisson, Bernoulli, Hawkes, categorical transitions…)
  general/     — Domain-agnostic iterations (constants, copy, cumulative, embedded simulation,
                 sorting, resampling…)
  kernels/     — Integration kernels for time-weighted aggregation (exponential, periodic, Gaussian…)
  inference/   — Parameter estimation: likelihood distributions, posterior mean/covariance iterations
  agents/      — Decision-making agents over a generic Environment[S, A] (ships MCTS/UCT + MAST)
  analysis/    — Post-simulation tooling: CSV/DataFrame & PostgreSQL I/O, plotting, grouped
                 aggregation, rolling-window likelihoods, posterior-estimation & self-play wiring
  keyboard/    — Real-time keyboard input for interactive simulations
cmd/stochadex/ — CLI binary
cfg/           — Example YAML configs
models/        — Domain-models catalogue: data-free SDK stubs of real-world domains, wired
                 into engine CI (see models/CONVENTIONS.md)
test/          — Integration tests mirroring the nbs/ notebook examples
nbs/           — Jupyter (GoNB) notebooks with worked examples
docs/          — Documentation source and build script
```

**Where the depth lives.** This file stays high-level on purpose. Packages with non-obvious
internal architecture carry a `doc.go` package comment that is the authoritative source —
**read it before working in that package**. In particular `pkg/agents/doc.go` (agent /
MCTS / MAST decomposition and cycle-breaking), `pkg/analysis/doc.go`, and
`pkg/inference/doc.go`. Load-bearing invariants are documented next to the code they
constrain (e.g. the `backupVisits` docstring in `pkg/agents/mcts_tree.go`) — trust and
preserve those comments rather than re-deriving the reasoning.

## Core abstraction: the Iteration interface

Every simulation component implements `simulator.Iteration`:

```go
type Iteration interface {
    Configure(partitionIndex int, settings *Settings)
    Iterate(params *Params, partitionIndex int, stateHistories []*StateHistory,
            timestepsHistory *CumulativeTimestepsHistory) []float64
}
```

- `Configure` runs once at setup — seed RNGs, allocate buffers, cache indices.
- `Iterate` runs each step, returning the partition's next state as `[]float64`.
- **`Iterate` must not mutate `params`** — the test harness checks this.
- **Keep iterations stateless between runs**: all mutable state must be re-initialisable in
  `Configure`, because the harness runs a simulation twice and compares outputs.

## How partitions fit together

Partitions are wired through the config, not by direct calls. The three channels:

- **`params_from_upstream`** — pipes one partition's current-step output into another's
  params. This is a **within-step** read: the consumer sees the producer's *this-step*
  value, so it imposes a computation order.
- **`params_as_partitions`** — resolves partition *names* to their integer indices and
  passes them as params (so an iteration can read another partition's state history by index).
- **State-history reads** — inside `Iterate`, reading `stateHistories[i]` gives partition
  `i`'s value from the **previous** step (lag-1), regardless of computation order.

**Cycle-breaking rule (general).** `params_from_upstream` is within-step and will deadlock
if two partitions depend on each other within the same step. Break the cycle by mixing
*one* within-step direction with *one* lag-1 (state-history) direction; the 1-step lag
aligns because the consumer applies the producer's previous output to its own state, which
has not moved since the producer ran. (The MCTS pipeline in `pkg/agents` is the worked
example — see its `doc.go`.)

Simulations are usually assembled with a `simulator.ConfigGenerator` (`SetPartition` /
`SetSimulation` → `GenerateConfigs`). `GenerateConfigs` validates wiring, e.g.
`params_from_upstream` indices against the upstream partition's state width.

## Config: two entry paths

1. **Programmatic Go** — build `Settings` + `Implementations` (or a `ConfigGenerator`)
   directly in Go. Used in unit tests and when embedding stochadex as a library.
2. **YAML API path** — define the run in YAML (`ApiRunConfig`). Every position that holds a
   framework component is a **union**: a Go-expression string (e.g.
   `"&continuous.WienerProcessIteration{}"`, resolved by generating and running Go from a
   template, with `extra_packages` / `extra_vars` declaring imports and variables) **or** a
   `{type: ...}` data spec resolved at load with no toolchain (`iteration: {type:
   wiener_process}`; `timestep_function: {type: constant, stepsize: 1.0}`). A config that names
   no Go anywhere runs **in-process** — no codegen, no `go run`.

   The registries and tiers (all in `pkg/api`, with the four simulation-component families in
   `pkg/simulator`):
   - **Iterations** — `pkg/api/registry.go` (data-only) and `registry_compose.go` (composable,
     recursive kernel/likelihood/jump/prior/nested-iteration/named-func specs). A partition's
     bespoke *maths* still goes through `expressions:`; the registry is for the framework's own
     catalogue. Two drift tests guard it (`registry_test.go`, `registry_coverage_test.go`): every
     registered name constructs the type it claims, and a `go/ast` scan requires every `Iterate`
     type to be registered or excluded-with-reason.
   - **`run:`** — `{mode: batch | ensemble, seeds, concurrency}` (`RunModeConfig`).
   - **`data:` + `macros:`** (`pkg/api/macros*.go`) — the analysis tier. `data:` (a sub-simulation
     or a `csv`/`json_log`/`postgres` source) produces a `StateTimeStorage`; each macro expands one
     of the `pkg/analysis` constructors against it (`posterior_estimation`, `likelihood_comparison`,
     the aggregations, `scalar_regression_stats`) or runs live (`evolution_strategy_optimisation`,
     `smc_inference`). Macro inputs are typed spec structs decoded straight from YAML.

**Invariant A restated for this surface (repo boundary).** Inference-*as-forward-simulation* — a
posterior being stepped as a partition — is in scope here; `posterior_estimation` and the other
inference macros belong in this engine. Inference *against real data* is the `data:` resource
(the observed dataset), which a downstream repo supplies. The engine owns the forward and
inferential model; it does not own the dataset, the calibration loop, or the decision layer.
`mcts_self_play` stays Go on purpose — its `agents.Environment` is arbitrary game rules (the
decision layer), not representable as data.

## Testing conventions

- Unit tests live beside source as `pkg/*/*_test.go`, using `t.Run("…", …)` subtests.
- **Always include a subtest that runs `simulator.RunWithHarnesses(settings, implementations)`**
  — it checks for NaNs, wrong state widths, `params` mutation, history-integrity, and
  statefulness residue (running the sim twice and comparing).
- Settings for a test are loaded from a colocated YAML file (e.g. `wiener_process_settings.yaml`).
- Compare floats with `gonum.org/v1/gonum/floats`, never raw `==`. No mocking — use real
  implementations.
- Integration tests in `test/` keep the `nbs/` notebook examples working. After adding a
  feature, extend the relevant `pkg/*` unit test, then ask whether to add an integration test.

## Domain-models catalogue (`models/`)

`models/` is a catalogue of real-world domains, each a **data-free, SDK-built stub of its
generative core** wired into engine CI. It replaced the old `template/` scaffold: rather
than pushing frozen structure downstream, applications teach the engine what good domain
models look like, and recurring bespoke extensions surface for promotion into core. The
repo boundary follows the **generative/inferential split** — this engine owns the forward
model; downstream repos own inference, data, calibration, and the decision layer.

The full spec (per-entry artifacts — `card.md`, `stub.go`, `stub_test.go`, the mandatory
`behaviour_test.go` expected-behaviour suite, and a `declarative.yaml` twin with its
equivalence test — plus the actionable/structural response-claim taxonomy and the two-category
promotion triage) lives in **`models/CONVENTIONS.md`**. Add entries with the `/new-model`
skill; the reference entries are `models/antimicrobial-resistance/`, `models/floodrisk/`,
and `models/energy-balancer/`.

**The declarative twin is the promotion triage.** Each entry is also stated as data
(`declarative.yaml`, a `general.ExpressionIteration` per partition, run through `pkg/api` with
no Go). Whether that twin can be written is the test: if it can, the bespoke Go is a
convenience and promotion is optional (earn it with a benchmark); if it cannot, the engine has
a real capability gap and one model is enough to prove it. Never change a model to make its
twin agree, and never widen a tolerance to hide a gap — step the oracle down instead
(exact → claim-level → distributional) and say which you used.

## Build, run, and docs

```bash
go build ./...                                   # compile all packages
go test ./...                                    # run all unit tests
go test ./pkg/continuous/...                     # test one package
go build -o bin/ ./cmd/stochadex                 # build the CLI
./bin/stochadex --config cfg/example_config.yaml # run a simulation
cd docs && bash build.sh                         # build HTML docs (needs pandoc + gomarkdoc)
```

## Code style

- Standard Go conventions (`gofmt`, idiomatic naming); exported types and functions have
  doc comments.
- Keep iteration implementations stateless between runs (see Core abstraction above).
- Put non-obvious package architecture in that package's `doc.go`, and load-bearing
  invariants in a comment next to the code — keep this file a map, not an encyclopedia.
