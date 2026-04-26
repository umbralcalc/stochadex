# Stochadex — Claude Code Conventions

## Project Overview

Stochadex is a Go SDK for building, configuring, and running simulations of stochastic processes and complex systems. It provides a flexible, partition-based execution model where independent simulation components run concurrently.

- **Module**: `github.com/umbralcalc/stochadex`
- **Entry point**: `cmd/stochadex/main.go` — delegates to `api.RunWithParsedArgs(api.ArgParse())`

## Architecture

```
pkg/
  simulator/   — Core engine: Iteration interface, PartitionCoordinator, state/time histories, output/termination conditions
  api/         — YAML config loading, CLI arg parsing, code generation templates
  continuous/  — Continuous stochastic processes (Wiener, GBM, Ornstein-Uhlenbeck, drift-diffusion, etc.)
  discrete/    — Discrete processes (Poisson, Bernoulli, Hawkes, categorical state transitions, etc.)
  general/     — General-purpose iterations (constant values, copy, cumulative, embedded simulation, sorting, resampling, etc.)
  kernels/     — Integration kernels for time-weighted aggregation (exponential, periodic, Gaussian, etc.)
  inference/   — Parameter estimation: likelihood distributions, posterior mean/covariance iterations
  agents/      — Agents over a generic `Environment[S, A]`. Currently ships MCTS (UCT) decomposed into `MCTSTreeIteration` (selection + backup), `MCTSRolloutIteration` (one rollout per step), and the agent-generic `ApplyIteration` (state advancer); plus MAST variants `MASTRolloutIteration` (MAST-biased playout, emits scores + path) and `MASTAggregationIteration` (running per-key aggregates as row state). Plus `MCTSTree` value type, rollout adapters (`UniformRandomRollout`, `FromProgress`, `WinnerToTerminal`), and a one-shot `RunMCTSSearch` helper. Shared environment fixture (tic-tac-toe) at `pkg/agents/agentstest`. Future agent algorithms (alpha-beta, MCMC-based, learned policies, etc.) belong here too — `Environment` is the framework.
  analysis/    — Post-simulation: CSV/DataFrame I/O, PostgreSQL, grouped aggregations, plotting, rolling likelihood windows, posterior estimation helpers, online scalar OLS (`ScalarRegressionStatsIteration`, `NewScalarRegressionStatsPartition`), MCTS self-play wiring (`NewMCTSSelfPlayPartitions` returns the apply + embedded-search partition pair)
  keyboard/    — Real-time keyboard input for interactive simulations
cmd/stochadex/ — CLI binary
cfg/           — Example YAML configs
test/          — Integration tests (correspond to notebook examples in nbs/)
nbs/           — Jupyter notebooks (GoNB) with worked examples
docs/          — Documentation source and build script
```

## Core Abstraction: the Iteration Interface

Every simulation component implements `simulator.Iteration`:

```go
type Iteration interface {
    Configure(partitionIndex int, settings *Settings)
    Iterate(params *Params, partitionIndex int, stateHistories []*StateHistory,
            timestepsHistory *CumulativeTimestepsHistory) []float64
}
```

- `Configure` is called once at setup. Use it to seed RNGs, allocate buffers, etc.
- `Iterate` is called each step. It receives params, all partition state histories, and time info. It returns the next state as `[]float64`.
- Partitions communicate via `params_from_upstream` in config, which pipes one partition's state values into another's params.
- The `Iterate` method must NOT mutate `params` — the test harness checks this.

## Build & Run

```bash
go build ./...                          # compile all packages
go test ./...                           # run all unit tests
go test ./pkg/continuous/...            # run tests for a specific package
go build -o bin/ ./cmd/stochadex        # build the CLI binary
./bin/stochadex --config cfg/example_config.yaml   # run a simulation
```

Optional: set `core.hooksPath` to `scripts/git-hooks` so each commit (except template-only pin commits) bumps `template/` to `github.com/umbralcalc/stochadex@main`. See `scripts/git-hooks/README.md`.

## Testing Conventions

- **Unit tests** live alongside source in `pkg/*/` as `*_test.go` files.
- Use `t.Run("description", func(t *testing.T) { ... })` subtests.
- Always include a subtest that runs with `simulator.RunWithHarnesses(settings, implementations)` — this wraps iterations in a test harness that checks for NaN outputs, wrong state widths, params mutation, history integrity, and statefulness residues (runs the simulation twice and compares outputs).
- Settings are loaded from colocated YAML files (e.g., `wiener_process_settings.yaml` next to `wiener_process_test.go`).
- Use `gonum.org/v1/gonum/floats` for float comparisons — never raw `==` on floats.
- No mocking — use real implementations.
- **Integration tests** in `test/` ensure the notebook examples in `nbs/` work. After adding a feature, create or extend a `_test.go` in the relevant `pkg/` directory, then ask the developer if they want to add an integration test too.

## Config System

Two ways to set up simulations:

1. **Programmatic Go** — Build `Settings` and `Implementations` structs directly in Go code. Used in unit tests and when embedding stochadex as a library.

2. **YAML API path** — Define simulations in YAML (`ApiRunConfig`). Iteration types are Go expressions as strings (e.g., `"&continuous.WienerProcessIteration{}"`). The API generates and runs Go code from a template. Config files use `extra_packages` and `extra_vars` to declare imports and variables.

## Inference and analysis (`pkg/analysis`)

Online posterior stacks are usually built with `NewPosteriorEstimationPartitions`, `NewLikelihoodComparisonPartition`, and storage from `NewStateTimeStorageFromPartitions` / `AddPartitionsToStateTimeStorage`.

- **State history depth**: Windowed likelihoods replay data through `general.FromHistoryIteration`. Each `Window.Data` source partition needs `StateHistoryDepth` at least `Window.Depth` (set `windowSizeByPartition` in `AddPartitionsToStateTimeStorage` accordingly). Optionally set `AppliedLikelihoodComparison.WindowDataHistoryDepth` for a setup-time check, or call `ValidateWindowDataHistoryDepth` with the same map you pass to `AddPartitionsToStateTimeStorage`.
- **Embedded burn-in**: The comparison partition defaults `burn_in_steps` to `Window.Depth` so the inner run aligns with available history; the first outer steps can repeat the same inner log-likelihood. Override with `EmbeddedBurnInSteps` to decouple from window size. `PosteriorLogNormalisationIteration` discounts all rolling history rows—including that padding—via `past_discounting_factor`.
- **MemoryDepth**: Depth of the likelihood partition’s state history used by the posterior log-normalisation rolling window; keep it consistent with how much past you intend to aggregate.
- **Posterior covariance**: `PosteriorCovariance.JustVariance` uses diagonal variances (length N); `NewPosteriorEstimationPartitions` wires the sampler to `variance_partition` automatically. Full covariance defaults must have length N².
- **Normal sampler**: `NormalLikelihoodDistribution.AllowDefaultCovarianceFallback` must be true to substitute `default_covariance` when the streamed matrix is not PD. `cov_burn_in_steps` fixes the proposal covariance to `default_covariance` for the first K outer steps when that param is set.
- **Upstream indices**: `params_from_upstream.indices` are validated against the upstream partition’s state width when `ConfigGenerator.GenerateConfigs` runs.
- **Regression stats**: `analysis.ScalarRegressionStatsIteration` with `NewScalarRegressionStatsPartition` streams through-origin or intercept OLS sufficient statistics and closed-form estimates; use `params_from_upstream` keys `y` and `x` for scalar series. `RegressionStatsWindow` uses a fixed-length buffer (state width O(W)), not exponential forgetting.

For stiff OU mean-reversion with large θΔt, prefer `continuous.OrnsteinUhlenbeckExactGaussianIteration` over Euler–Maruyama `OrnsteinUhlenbeckIteration` when modeling should match the Gaussian transition.

## Agents and MCTS (`pkg/agents`, analysis helpers)

`pkg/agents` hosts decision-making agents over a generic `Environment[S, A]`. Per-player terminal scores are `[]float64` in `[0,1]` (the same convention as inference / progress). Codecs stay downstream — partitions take `Decoder` and `Encoder` function fields rather than depending on a `Codec[S]` interface.

- **Agent-generic vs MCTS-specific**: `Environment[S, A]` and `ApplyIteration[S, A]` are agent-generic — any future agent built on the same framework can reuse them. MCTS-specific symbols are `MCTS`-prefixed (`MCTSConfig`, `MCTSTree`, `MCTSTreeIteration`, `MCTSRolloutIteration`, `MCTSRolloutFn`, `MCTSEdgeStat`, `RunMCTSSearch`, `MCTSDefault*`, `MCTSTreeRow*`, `MCTSTreeParam*`, `MCTSRolloutParamLeaf`, `MCTSRolloutRowWidth`). MAST symbols (`MASTAggregationIteration`, `MASTRolloutIteration`, `MAST*` constants) are MAST-prefixed since MAST is itself a specific MCTS technique. Rollout-adapter helpers (`UniformRandomRollout`, `FromProgress`, `WinnerToTerminal`) are un-prefixed because their context is implicit through their `MCTSRolloutFn` return type.
- **MCTS decomposition (Architecture K)**: MCTS is split along its natural fault lines.
  - `MCTSTreeIteration[S, A]` owns the tree on its struct (graph state, fundamentally not row-shaped). Each step backs up the path it selected last step (with scores arriving lag-1 via state history from an upstream `MCTSRolloutIteration`) then walks UCB to select a new leaf for next step's rollout. Row exposes `best_root_idx`, the leaf state for rollout consumption, `has_leaf` flag, and per-action root visit / win sums (padded to `MaxLegalActions`). Use `MCTSTreeRowWidth(W, K)` and the `MCTSTreeRow*Offset` helpers to compute slot offsets.
  - `MCTSRolloutIteration[S, A]` is stateless. Reads leaf state via `params_from_upstream` from `MCTSTreeIteration` within-step, runs one rollout, outputs `[scores(P), ok(1)]` row. Swap the rollout function (`Cfg.Rollout`) without touching wiring — `UniformRandomRollout`, `FromProgress`, `WinnerToTerminal` all plug in.
  - `ApplyIteration[S, A]` owns the encoded game state. Reads a best-action signal from upstream and applies one ply per outer step. Two read modes: direct param (within-step, via `ApplyParamBestIdx`) or state-history (lag-1, via `ApplyParamBestIdxPartition` + `BestIdxSlot`). State-history mode breaks the apply ↔ search cycle in self-play.
- **Cycle-breaking rule**: stochadex's `params_from_upstream` is *within-step* and deadlocks on cycles. Mix one within-step direction with one lag-1 (state-history) direction to break a cycle. The MCTS pipeline does this twice:
  - tree ↔ rollout: rollout reads tree within-step, tree reads rollout lag-1.
  - apply ↔ search: search reads apply within-step, apply reads search lag-1.
  The 1-step lag aligns correctly because the consumer applies the producer's previous output to its current state, which hasn't moved since the producer ran.
- **`backupVisits` is load-bearing**: the no-signal-tolerant backup variant exists because of the engine-stall deadlock (truncated rollouts with no progress signal would leave every child at visits=0 and UCB would lock onto the first-listed legal action). Do not “simplify” it back to `backupScores`.
- **Single rollout signature**: `agents.MCTSRolloutFn[S, A] = func(env, s, max, seed) ([]float64, bool, error)`. Compose with `UniformRandomRollout`, `FromProgress`, or `WinnerToTerminal` rather than overloading the signature.
- **MAST as two extra partitions** (in `pkg/agents`): when MAST-biased rollouts are wanted, swap `MCTSRolloutIteration` for `MASTRolloutIteration[S, A]` (does its own MAST-aware playout, emits `[scores, ok, num_path, (key_idx, reward)*MaxPath]`) and add an `MASTAggregationIteration[A]` downstream of it whose row holds the running `(count, sum)` pairs per action key. The aggregation partition feeds back into the rollout via `params_as_partitions` (lag-1) so the policy improves over the course of a search. Action vocabulary must be bounded — supply `KeyToIdx func(A) int` and `MaxKeys` upfront.
- **Defaults are filled inside `MCTSTree.RunOne` / `MCTSTree.SelectLeaf`**: `MCTSConfig{Rollout: ...}` works out of the box; you don't need to pre-populate `Simulations` / `MaxTreeDepth` / etc. unless you want to override them.
- **Self-play wiring helper**: `analysis.NewMCTSSelfPlayPartitions[S, A](spec)` returns the outer apply + search partition pair. The search partition is an `EmbeddedSimulationRunIteration` wrapping a `MCTSTreeIteration` + `MCTSRolloutIteration` inner sim that runs `SimsPerPly` inner steps per outer step.
- **Shared test fixture**: `pkg/agents/agentstest` exports a tic-tac-toe `Environment` plus codec/key helpers (`TTTGame`, `TTTState`, `TTTAction`, `TTTEncode`, `TTTDecode`, `TTTKey`, `TTTFromGrid`). Reuse from any `_test.go` rather than redefining a fixture per package.
- **File naming**: MCTS-specific files in `pkg/agents/` are `mcts_`-prefixed (`mcts_tree.go`, `mcts_rollout_iteration.go`, etc.); agent-generic files (`environment.go`, `apply_iteration.go`) and MAST-flavoured files (`mast_*.go`) keep their respective prefixes. Each partition has its own `*_test.go` + `*_settings.yaml` next to its source, with two `t.Run` blocks (plain run + `RunWithHarnesses`) following the convention used in `pkg/continuous/` etc.

## Documentation

Generated via `docs/build.sh` which requires `pandoc` and `gomarkdoc`. It produces HTML docs from Go source comments and markdown files with MathJax and bibliography support.

```bash
cd docs && bash build.sh
```

## Code Style

- Standard Go conventions (`gofmt`, idiomatic naming).
- Exported types and functions have doc comments.
- Keep iteration implementations stateless between runs — all mutable state must be re-initializable via `Configure`.
