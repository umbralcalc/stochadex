---
title: "Quickstart"
logo: true
---

# Quickstart
<div style="height:0.75em;"></div>

## Your first simulation

```bash
go get github.com/umbralcalc/stochadex
```

A complete program: a random walk, five steps, every step recorded.

```go
package main

import (
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func main() {
	gen := simulator.NewConfigGenerator()

	// One component: a Wiener process (random walk) starting at 0.
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              "walk",
		Iteration:         &continuous.WienerProcessIteration{},
		Params:            simulator.NewParams(map[string][]float64{"variances": {1.0}}),
		InitStateValues:   []float64{0.0},
		StateHistoryDepth: 1,
		Seed:              42,
	})

	// Run for five steps, recording every step into storage.
	store := simulator.NewStateTimeStorage()
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		OutputFunction:       &simulator.StateTimeStorageOutputFunction{Store: store},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 5},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
	})
	simulator.NewPartitionCoordinator(gen.GenerateConfigs()).Run()

	fmt.Println("times:", store.GetTimes())
	fmt.Println("walk: ", store.GetValues("walk"))
}
```

```bash
go run .
```

```
times: [0 1 2 3 4 5]
walk:  [[0] [-0.27282789148858066] [-1.3375369499117022] [-2.548435603601376] [-0.6832544462398817] [-0.3886233811019282]]
```

That is a working stochastic simulation. Tweak `MaxNumberOfSteps`, `variances`, or `Seed`.

## What you just built

- A **partition** is one component. A simulation is a *set* of them advancing together; add more `SetPartition` calls to couple several.
- An **`Iteration`** advances a partition one step. `WienerProcessIteration` is built in; write your own by implementing the two-method [`Iteration`](https://stochadex.github.io/pkg/simulator.html#Iteration) interface (`Configure` once, `Iterate` each step). The whole engine is built on this one interface.
- The **state history** is what a partition remembers. `StateHistoryDepth: 1` keeps the latest value; more depth lets an iteration read its own past (needed for memory-ful processes like Hawkes).

[How it works](https://stochadex.github.io/pkg/how_it_works.html) covers coupling, custom iterations, and worked examples (Itô's lemma, Hawkes, embedded simulations, online inference).

## Where the results go

The `walk` output is plain `[][]float64`, but the same run flows straight out:

- **CSV / DataFrame / JSON logs**: the [`analysis`](https://stochadex.github.io/pkg/analysis.html) package reads and writes these.
- **PostgreSQL / TimescaleDB / QuestDB**: any Postgres-wire database (supply your own `*sql.DB`).
- **Apache Arrow → Polars / pandas / DuckDB**: the opt-in [`arrowstore`](https://stochadex.github.io/pkg/arrowstore.html) module builds Arrow directly.

See the [Integrations table](https://stochadex.github.io/#integrations) for the full set.

## Running from a config file

No Go needed. Describe a whole run in **one YAML file** and run it with a prebuilt binary. A config that names no Go runs **in-process**: no codegen, no toolchain.

### Install the CLI

```bash
# Prebuilt binary, picks your platform's asset from the latest release:
curl -L "https://github.com/umbralcalc/stochadex/releases/latest/download/stochadex-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')" -o stochadex
chmod +x stochadex

# Or with Go:
go install github.com/umbralcalc/stochadex/cmd/stochadex@latest

# Or as a container:
docker pull ghcr.io/umbralcalc/stochadex:latest
```

The image's working directory is `/work`, so mount your project there and every path below works as written:

```bash
docker run --rm -v "$PWD:/work" ghcr.io/umbralcalc/stochadex:latest --config my_config.yaml
```

### Which build?

| Asset | Contains | Notes |
|---|---|---|
| `stochadex-<os>-<arch>` | engine, **Postgres**, **Arrow**, **S3** | The default. Pure Go, runs anywhere with no system dependencies. |
| `stochadex-accel-<os>-<arch>` | the above plus **system BLAS** and **DuckDB** output | For BLAS-heavy workloads (see [performance](performance.html)) or DuckDB output. macOS/Linux, amd64/arm64. |

Swap `stochadex-` for `stochadex-accel-` in the download URL. Both run the same configs. The container carries the accelerated set already. Run `--version` to see what yours has.

## Your first config

A 1-D random walk, recorded every step:

```yaml
# walk.yaml
main:
  partitions:
  - name: walk
    iteration: {type: wiener_process}
    params: {variances: [1.0]}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 7
  simulation:
    output_condition: {type: every_step}
    output_function: {type: stdout}
    termination_condition: {type: number_of_steps, max_steps: 5}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
```

```bash
stochadex --config walk.yaml
```

One row per step, `<time> <partition> [<state values>]`:

```
0 walk [0]
1 walk [-0.10275106104846077]
2 walk [1.724114244166499]
3 walk [0.7336019413185719]
4 walk [0.01102228754125667]
5 walk [1.101502572408065]
```

Stream over a websocket for live dashboards (publish the port for the container):

```bash
stochadex --config walk.yaml --socket cfg/socket.yaml

docker run --rm -p 2112:2112 -v "$PWD:/work" ghcr.io/umbralcalc/stochadex:latest \
  --config walk.yaml --socket cfg/socket.yaml
```

## The anatomy of a partition

A **partition** advances a vector state each step from its **params** and, optionally, other partitions' states.

| Field | Meaning |
|---|---|
| `params` | Named inputs; every value is a list of float64 (a scalar is `[0.5]`). |
| `init_state_values` | The state vector at *t*=0. Its length is the state width. |
| `state_history_depth` | How many past steps to retain (≥1). |
| `seed` | Per-partition RNG seed. |
| `iteration` | A library process named as data, *or* omit it and supply `expressions`. |

The `simulation` block is all data too: `output_condition`
(`every_step` / `every_n_steps` / `only_given_partitions` / `nil`), `output_function`
(`stdout` / `json_log` / `arrow` / `duckdb` / `postgres` / `s3` / `nil`), `termination_condition`
(`number_of_steps` / `time_elapsed`), `timestep_function`
(`constant` / `exponential_distribution`).

### Writing results out

Beyond `stdout` and `json_log`, write columnar output directly:

```yaml
    output_function: {type: arrow, path: run.arrow}                    # Arrow IPC file
    output_function: {type: duckdb, path: run.duckdb, table: results}  # DuckDB table
```

`postgres` takes local credentials, or `driver`/`dsn` through `database/sql` to reach **any Postgres-wire database** (TimescaleDB, CockroachDB, a managed instance):

```yaml
    output_function: {type: postgres, driver: pgx, dsn: "postgres://...", table: results}
```

`s3` is a **transport, not a format**: give it a `format:` and it reuses the normal sink, so anything writable locally is writable to object storage. Credentials come from the standard AWS chain, never the config file. Set `endpoint:` for any S3-compatible store (MinIO, R2, Ceph):

```yaml
    output_function: {type: s3, bucket: my-bucket, key: runs/out.arrow, format: arrow}
```

The same formats work as `data:` sources to read a run back in: `{arrow: {path: run.arrow}}`, `{postgres: {...}}`, `{s3: {bucket, key, format}}`. Name a source the binary lacks and the error lists the ones it has.

`arrow` writes one IPC file (a `time` column plus a fixed-size list column per partition), read natively by Polars, pandas and DuckDB:

```python
import pyarrow.ipc as ipc
table = ipc.open_file("run.arrow").read_all()
```

`duckdb` lands the same data in a DuckDB table (zero-copy). Both write once at the end, so they need an `output_condition` that emits every partition every step.

> `arrow`, `postgres`, `s3` are in every binary; the container adds `duckdb`. `duckdb` needs the **accelerated** binary. `stochadex --version` prints a `features:` line.

## Two ways to write an update

**A library process**, named with its params:

```yaml
  - name: walk
    iteration: {type: wiener_process}
    params: {variances: [1.0, 4.0]}      # a 2-D Wiener process
```

Registered names span the catalogue: `ornstein_uhlenbeck`, `geometric_brownian_motion`, `poisson_process`, `hawkes_process`, `categorical_state_transition`, and more, including *composable* ones that nest (`{type: data_generation, likelihood: {type: normal}}`).

**Bespoke maths**, written as expressions:

```yaml
main:
  partitions:
  - name: growth
    params: {rate: [0.05], capacity: [100.0], noise: [0.1]}
    init_state_values: [10.0]
    state_history_depth: 1
    seed: 42
  expressions:
  - partition: growth
    fields: [{name: x}]                  # names the state slots, in order
    bindings:                            # optional intermediates, evaluated in order
    - {name: drift, expr: "rate * x * (1 - x / capacity)"}
    outputs:                             # one expression per field = the next state
    - "x + drift * dt + noise * x * shared(normal(0, 1)) * sqrt(dt)"
```

Expressions use field names, params keys, `dt`, `t`, `step`, earlier bindings, and upstream aliases. Functions: `sqrt pow exp log abs min max clamp where floor sin cos erf`, `slice`, `concat`, `lag`, plus arithmetic and comparisons, all elementwise with length-1 broadcasting.

> **The most common mistake.** A random draw with all-scalar parameters has ambiguous width and fails. Wrap it: `shared(normal(0, 1))` for one sample, `iid(n, normal(0, 1))` for *n*. A draw whose parameter is already a vector needs no wrapper.

## Coupling partitions, and the one rule that matters

Partitions read each other two ways, differing in **timing**:

1. **`upstreams`** (expressions block): the other partition's **previous**-step value (one-step lag).
2. **`params_from_upstream`** (partitions block): another partition's value injected into a params key, read **within** the same step, imposing a computation order.

`params_from_upstream` **deadlocks** if two partitions each depend on the other within a step. Break the cycle with a lag-1 `upstreams` read in at least one direction. For mutually-coupled models (predator-prey and friends), lag-1 both ways is the faithful explicit-Euler step. The run pre-flights this and names the cycle instead of hanging.

## Run modes

```yaml
run:
  mode: ensemble           # one member per seed, run concurrently
  seeds: [11, 22, 33, 44]  # output rows are prefixed member=<i> seed=<s>
  # concurrency: 4         # optional; defaults to GOMAXPROCS
```

Omit `run` for a single batch run.

## Analysis, inference and optimisation

A `data` block produces a dataset (a sub-simulation, or a `csv` / `json_log` / `postgres` source). Each `macros` entry expands a framework analysis constructor into a *set* of partitions against it. All data, all in-process.

```yaml
data:
  steps: 500
  timestep: 1.0
  partitions:
  - name: data_stream
    iteration: {type: data_generation, likelihood: {type: normal}}
    params: {mean: [1.8, 5.0], covariance_matrix: [2.5, 0.0, 0.0, 9.0]}
    init_state_values: [1.3, 8.3]
    state_history_depth: 200
    seed: 291
macros:
- type: vector_mean
  name: rolling_mean
  data: {partition_name: data_stream}
  kernel: {type: exponential}
  params: {exponential_weighting_timescale: [100.0]}
  window: 100
```

Macros: the aggregations (`vector_mean` / `vector_variance` / `vector_covariance`, `grouped_aggregation`), `scalar_regression_stats`, `likelihood_comparison`, `likelihood_mean_function_fit`, `posterior_estimation`, and the two live ones, `evolution_strategy_optimisation` and `smc_inference`, which need no `data` block.

### The learning macros have levers

Four macros *converge* or merely *run* depending on hyperparameters. Each ships as a converging example under [`cfg/`](https://github.com/umbralcalc/stochadex/tree/main/cfg), pinned by a test that asserts it recovers a known answer:

| Macro | Recovers | Levers that decide convergence |
|---|---|---|
| `evolution_strategy_optimisation` | a reward's optimum | Keep covariance `learning_rate` slow (≈0.1); a fast rate collapses the search width before the mean arrives. `discount_factor: 0.0` for a static objective. |
| `posterior_estimation` | the data-generating parameters | The `comparison` **must** read the sampler (the posterior weights *sampled* params by loglike). Proposal covariance wide enough to explore prior→truth; `past_discount` near 1. |
| `smc_inference` | the observed stream's mean | `num_particles`, `num_rounds`, `priors` ranges. |
| `scalar_regression_stats` | slope and intercept | None; OLS is closed-form. With an intercept, cumulative mode is width 9: `[n, Sx, Sy, Sxx, Sxy, Syy, alpha, beta, sigma2]`. |

## Authoring with an agent

The repo ships a Claude Code plugin bundling the `stochadex-model` skill, a self-contained authoring guide with the four converging recipes above:

```bash
claude plugin marketplace add umbralcalc/stochadex
claude plugin install stochadex@stochadex
```

Then describe a system in plain language. The skill drives the same CLI.

## Example analysis notebooks

- [Examples with CSV files](https://github.com/umbralcalc/stochadex/blob/main/nbs/csv.ipynb)
- [Examples with Dataframes](https://github.com/umbralcalc/stochadex/blob/main/nbs/dataframe.ipynb)
- [Examples with JSON Log Entries](https://github.com/umbralcalc/stochadex/blob/main/nbs/logs.ipynb)
- [Examples with Partitions](https://github.com/umbralcalc/stochadex/blob/main/nbs/partitions.ipynb)
- [Examples with a Postgres DB](https://github.com/umbralcalc/stochadex/blob/main/nbs/postgres.ipynb)

## Where to look next

- [`cfg/`](https://github.com/umbralcalc/stochadex/tree/main/cfg): worked example configs (composition, ensembles, inference, optimisation, regression, data sources).
- [How it works](how_it_works.html): the execution model behind partitions and histories.
- [API package docs](simulator.html): the Go interfaces the config tier resolves to.
