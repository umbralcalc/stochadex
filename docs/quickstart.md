---
title: "Quickstart"
logo: true
---

# Quickstart
<div style="height:0.75em;"></div>

## Your first simulation

Add the module:

```bash
go get github.com/umbralcalc/stochadex
```

Then this is a complete, runnable program: a random walk advanced for five steps, with every step recorded and printed:

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

Run it:

```bash
go run .
```

```
times: [0 1 2 3 4 5]
walk:  [[0] [-0.27282789148858066] [-1.3375369499117022] [-2.548435603601376] [-0.6832544462398817] [-0.3886233811019282]]
```

That is a working stochastic simulation. Change `MaxNumberOfSteps` for a longer run, `variances` for a wilder walk, or `Seed` for a different trajectory.

## What you just built

Three ideas, in the order they appear above:

- A **partition** is one component of the simulation (here, the single `walk`). A simulation is a *set* of partitions advancing together each step; add more `SetPartition` calls to run and couple several.
- An **`Iteration`** is the rule that advances a partition one step. `WienerProcessIteration` is one of many built in, but you write your own by implementing the two-method [`Iteration`](https://stochadex.github.io/pkg/simulator.html#Iteration) interface (`Configure` once, `Iterate` each step), and it slots in exactly the same way. This one interface is what the whole engine is built on.
- The **state history** is what each partition remembers. `StateHistoryDepth: 1` keeps only the latest value; a larger depth lets an iteration read its own past (needed for memory-ful processes like Hawkes). The `OutputFunction` copies each step into storage so you can read it back, as above, or straight to CSV, a database, or Apache Arrow.

See [How it works](https://stochadex.github.io/pkg/how_it_works.html) for the full picture: coupling partitions, writing custom iterations, and worked examples (Itô's lemma, Hawkes processes, embedded simulations, online parameter inference).

## Where the results go

The `walk` output above is plain `[][]float64`, but the same recorded run flows straight into the data ecosystem:

- **CSV / DataFrame / JSON logs**: the [`analysis`](https://stochadex.github.io/pkg/analysis.html) package reads and writes these directly.
- **PostgreSQL / TimescaleDB / QuestDB**: write output to, or load history from, any Postgres-wire database (supply your own `*sql.DB`).
- **Apache Arrow → Polars / pandas / DuckDB**: the opt-in [`arrowstore`](https://stochadex.github.io/pkg/arrowstore.html) module builds Arrow directly, for zero-conversion columnar interchange.

See the [Integrations table](https://stochadex.github.io/#integrations) for the full set.

## Running from a config file

You can also drive the engine from YAML instead of Go. Build the CLI once and point it at a config:

```bash
git clone git@github.com:umbralcalc/stochadex.git && cd stochadex
go build -o bin/ ./cmd/stochadex
./bin/stochadex --config ./cfg/example_config.yaml
```

Stream results over a websocket (for live dashboards):

```bash
./bin/stochadex --config ./cfg/example_config.yaml --socket ./cfg/socket.yaml
```

Or run it containerised (may need `sudo`):

```bash
docker build -t stochadex -f Dockerfile.stochadex .
docker run -p 2112:2112 stochadex --config ./cfg/example_config.yaml --socket ./cfg/socket.yaml
```

## Example analysis notebooks

- [Examples with CSV files](https://github.com/umbralcalc/stochadex/blob/main/nbs/csv.ipynb)
- [Examples with Dataframes](https://github.com/umbralcalc/stochadex/blob/main/nbs/dataframe.ipynb)
- [Examples with JSON Log Entries](https://github.com/umbralcalc/stochadex/blob/main/nbs/logs.ipynb)
- [Examples with Partitions](https://github.com/umbralcalc/stochadex/blob/main/nbs/partitions.ipynb)
- [Examples with a Postgres DB](https://github.com/umbralcalc/stochadex/blob/main/nbs/postgres.ipynb)
