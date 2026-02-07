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
  analysis/    — Post-simulation: CSV/DataFrame I/O, PostgreSQL, grouped aggregations, plotting
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

## Documentation

Generated via `docs/build.sh` which requires `pandoc` and `gomarkdoc`. It produces HTML docs from Go source comments and markdown files with MathJax and bibliography support.

```bash
cd docs && bash build.sh
```

## Code Style

- Standard Go conventions (`gofmt`, idiomatic naming).
- Exported types and functions have doc comments.
- Keep iteration implementations stateless between runs — all mutable state must be re-initializable via `Configure`.
