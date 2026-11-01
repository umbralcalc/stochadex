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

# Or with Go, build from a checkout (the CLI is its own module):
git clone https://github.com/umbralcalc/stochadex && cd stochadex/cmd/stochadex && go build -o stochadex .

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

Swap `stochadex-` for `stochadex-accel-` in the download URL. Both run the same configs. The container carries the accelerated set already. Run `--version` to see what yours has, or `stochadex catalogue` for everything it can run (see [what a build can run](reference.html#what-a-build-can-run)).

Each release also carries a `SHA256SUMS` file covering every binary asset. Check a download against it:

```bash
curl -LO "https://github.com/umbralcalc/stochadex/releases/latest/download/SHA256SUMS"
sha256sum --check --ignore-missing SHA256SUMS   # macOS: shasum -a 256 --check --ignore-missing
```

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
    termination_condition: {type: number_of_steps, max_steps: 5}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
```

```bash
stochadex --config walk.yaml
```

With no `outputs:` declared, every step prints to stdout, one row per step,
`<time> <partition> [<state values>]`:

```
0 walk [0]
1 walk [-0.10275106104846077]
2 walk [1.724114244166499]
3 walk [0.7336019413185719]
4 walk [0.01102228754125667]
5 walk [1.101502572408065]
```

To serve it over a websocket for live dashboards, add a `run:` block (see
[run modes](reference.html#run-modes)) and publish the port for the container:

```bash
docker run --rm -p 2112:2112 -v "$PWD:/work" ghcr.io/umbralcalc/stochadex:latest \
  --config cfg/example_serve_config.yaml
```

## Next

The [config reference](reference.html) covers everything a config can say, with every example
run by a test:
- [how a step runs](reference.html#how-a-step-runs): time, reading other partitions within the
  step or one step behind, history, and the one rule (no cycle within a step);
- [partitions](reference.html#partitions) and [nested runs](reference.html#nested-runs);
- [inputs](reference.html#inputs), including live streams, and [outputs](reference.html#outputs);
- [run modes](reference.html#run-modes): ensembles, serving, `--set` and sharding;
- [macros](reference.html#macros-analysis-inference-and-optimisation) for analysis,
  inference and optimisation;
- [checking and inspecting](reference.html#checking-inspecting-and-running-from-go) a config
  without running it, exit codes, and running a config from Go.

## Authoring with an agent

Using stochadex from an AI agent is [robert](https://github.com/umbralcalc/robert)'s job, not
this repo's; see its documentation. It drives stochadex through the same CLI described above,
with `--check`, `inspect --io`, `graph`, `expand` and `catalogue` as its machine-readable
contract.

## Example analysis notebooks

- [Examples with CSV files](https://github.com/umbralcalc/stochadex/blob/main/nbs/csv.ipynb)
- [Examples with Dataframes](https://github.com/umbralcalc/stochadex/blob/main/nbs/dataframe.ipynb)
- [Examples with JSON Log Entries](https://github.com/umbralcalc/stochadex/blob/main/nbs/logs.ipynb)
- [Examples with Partitions](https://github.com/umbralcalc/stochadex/blob/main/nbs/partitions.ipynb)
- [Examples with a Postgres DB](https://github.com/umbralcalc/stochadex/blob/main/nbs/postgres.ipynb)

## Where to look next

- [`cfg/`](https://github.com/umbralcalc/stochadex/tree/main/cfg): worked example configs (composition, ensembles, inference, optimisation, regression, data sources).
- [Config reference](reference.html): what a config means, section by section.
- [How it works](how_it_works.html): the execution model behind partitions and histories.
- [API package docs](simulator.html): the Go interfaces the config tier resolves to.
