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

To serve it over a websocket for live dashboards, add a `run:` block (see
[run modes](#run-modes)) and publish the port for the container:

```bash
docker run --rm -p 2112:2112 -v "$PWD:/work" ghcr.io/umbralcalc/stochadex:latest \
  --config cfg/example_serve_config.yaml
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
(`stdout` / `json_log` / `websocket` / `arrow` / `duckdb` / `postgres` / `s3` / `nil`), `termination_condition`
(`number_of_steps` / `time_elapsed`), `timestep_function`
(`constant` / `exponential_distribution`).

### Writing results out

Beyond `stdout` and `json_log`, write columnar output directly:

```yaml
    output_function: {type: arrow, path: run.arrow}                    # Arrow IPC file
    output_function: {type: duckdb, path: run.duckdb, table: results}  # DuckDB table
```

To stream a run live to another service (a dashboard, a recorder), push it to a websocket
server. The run connects as a client when it starts, sends each output as a protobuf
`PartitionState` frame, and closes the connection when it finishes:

```yaml
    output_function: {type: websocket, url: "ws://localhost:8080/ingest"}
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

**All or nothing.** A run's files, objects and tables appear only once the whole run has
ended cleanly. A failed, crashed or killed run leaves nothing at their destinations, and
any earlier run's output there untouched, so a retry is always safe:

| Output | While the run is going | Once it has ended cleanly |
|---|---|---|
| `json_log`, `arrow`, a stream input's `record:` | written to `<path>.partial` | renamed to `<path>`, replacing the file in one step |
| `s3` | staged in a local temporary file | uploaded: an object is replaced whole |
| `duckdb` | held in memory | ingested in one `CREATE TABLE AS` |
| `stdout`, `websocket`, `{type: connection}`, `postgres` | streamed as the run goes | (nothing held back: a failed run leaves what it sent) |

An embedded run's own output is published with its outer run. A served connection's
outputs are published when its session ends, and the client leaving counts as a clean end.
A killed run can leave a `.partial` file behind, which the next run replaces. Publishing
costs one rename per file, once per run and never per step.

### Driving a run from data: `inputs:`

A run can replay recorded or generated data. Declare it under `inputs:`, then replay one of an
input's partitions into a `main:` partition with `{type: from_input}`:

```yaml
inputs:
  obs:  {source: {csv: {path: obs.csv, time_column: 0, state_columns: {flow: [1]}}}}
main:
  partitions:
  - name: flow                      # replays obs's "flow" (partition: renames)
    iteration: {type: from_input, input: obs}
    state_history_depth: 1          # init_state_values default to the input's first row
    seed: 0
  # ... partitions that read flow via params_from_upstream ...
  simulation:
    timestep_function:     {type: from_input, input: obs}       # the clock follows the input
    termination_condition: {type: input_exhausted, input: obs}  # and stops when it runs out
```

An input is any `data.source` (`csv`, `json_log`, `postgres`, plus `arrow` and `s3` in the
distributed CLI), or a pre-pass simulation (`{simulation: {steps, timestep, partitions: ...}}`).
A run's `json_log` output can be the next run's input, which is how separate configs chain.

Inputs are read when the run starts, never when the config is loaded:
- a missing input exits as unavailable (75);
- an input lacking the named partition exits as a data error (65);
- an undeclared or unused input is a config error (78).

A `macros:` config reads `inputs:` too: its macros analyse every input's partitions. When
there are several inputs they must share one time axis, and a partition name may come from
only one of them. `data:` is shorthand for a single input, so existing `data:` configs work
unchanged.

### Setting params from an input, including a live stream

An input can set a partition's params instead of its state. Declare the param with the value it
has until the input sets it, and name where it comes from:

```yaml
  - name: walk
    iteration: {type: wiener_process}
    params: {variances: [1.0]}
    params_from_input: {variances: {input: vol}}   # partition: defaults to the key's name
```

From a stored input, step *k* gets row *k*, as `from_input` does (row 0 is the initial state).
This is the cheaper way to drive params from data: there is no extra partition, and nothing is
copied or allocated per step.

An input can also be a **live stream**, read while the run runs:

```yaml
inputs:
  feed:
    stream: {websocket: {url: "ws://localhost:9000/prices"}}   # connects as a client
    record: feed.log                                           # optional
main:
  partitions:
  - name: trader
    params: {price: [100.0]}                     # the price until the first message
    params_from_input: {price: {input: feed}}
```

- **Messages:** each message is a `json_log` entry, `{"partition_name": "price", "state":
  [101.5]}`, or a JSON array of them. Entries for partitions nothing reads are ignored.
- **Timing:** values enter only between steps. Each step gets the newest value a message has
  brought, and holds it until the next (`on_empty: hold_last`, the default).
- **Replay:** `record:` writes what each step was given as a `json_log`. Swap the stream for
  `{source: {json_log: {path: feed.log}}}` and the run repeats exactly.
- **Errors:** a stream that can't be reached is unavailable (75). A message that can't be
  decoded, or a value of the wrong width, is a data error (65).
- **Not yet:** streams don't apply to `macros:` configs, and `params_from_input` doesn't yet
  apply under `ensemble`.

#### Two-way serving: a client steers its own run

Under `run: {mode: serve}`, a stream can read the client the run is served to:
`stream: {connection: {}}`. Its messages arrive on the same connection the `{type: connection}`
view streams out on, so each client both watches and steers its own run
(`cfg/example_interactive_config.yaml`):

```yaml
inputs:
  controls:
    stream: {connection: {}}
    record: "controls-{connection}.log"     # one record per connection
main:
  partitions:
  - name: walk
    iteration: {type: drift_diffusion}
    params: {drift_coefficients: [0.0], diffusion_coefficients: [0.3]}
    params_from_input: {drift_coefficients: {input: controls, partition: drift}}
    ...
outputs:
  - {name: stream, function: {type: connection}}
run: {mode: serve, websocket: {address: ":2112", handle: /handle}, pace_ms: 100}
```

- **Messages:** a client sends `{"partition_name": "drift", "state": [0.5]}` as JSON. With
  `decode: protobuf_action_state`, it sends an `ActionState` protobuf instead
  (`cmd/messages/action_state.proto`, wire-compatible with dexetera's). Each entry of its
  `partitions` sets that stream partition. With none, its `values` set the stream partition
  named `values`, which any number of partitions can read.
- **Per connection:** a config reads one served client, and under `serve`, `record:` paths need
  `{connection}`.
- **Errors:** a message that can't be decoded ends the connection with a close frame giving
  the reason.

### Several outputs from one run

To send one run to several places, each with its own filter, list them under a top-level
`outputs:` instead of setting `output_condition` / `output_function`. Each entry is a
*view*: a name, an optional `condition` (default `every_step`), and a `function`:

```yaml
outputs:
  - {name: log,  function: {type: json_log, path: run.log}}
  - {name: dash, condition: {type: only_given_partitions, partitions: [price]},
     function: {type: websocket, url: "ws://localhost:8080/ingest"}}
  - {name: db,   condition: {type: every_n_steps, n: 10},
     function: {type: postgres, driver: pgx, dsn: "postgres://...", table: results}}
```

`output_condition` / `output_function` are shorthand for one view named `output`, so a config
uses one form or the other, not both. A config that declares no output at all prints every
step to stdout. `outputs:` also applies to a `macros:` config: its results go to the views,
each applying its condition exactly as a live run would, in time order.

With `run: {mode: ensemble}`, each member gets its **own** sinks, the shorthand's included:
write `{member}` (the member's index) or `{seed}` into a view's fields, e.g.
`path: "run-{member}.log"`, and every member writes its own file. Quote templated values in
YAML flow style (`{...}`). Every ensemble view must use a placeholder, or all members would
write to the same place; outside an ensemble, a placeholder is an error. `stdout` needs none:
each member's rows are prefixed `member=<i> seed=<s>` (members run concurrently, so their
rows interleave, each member's in order).

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

To serve the model over a websocket, for a live dashboard:

```yaml
run:
  mode: serve
  websocket: {address: ":2112", handle: /handle}   # handle defaults to /
  pace_ms: 200                                      # delay between steps
  # websocket.allowed_origins: ["https://dash.example.com"]   # or "*"
outputs:
  - {name: stream, function: {type: connection}}    # what each client receives
  - {name: log, function: {type: json_log, path: "session-{connection}.log"}}
```

Every client that connects gets its **own fresh run** of the model, streamed step by step as
protobuf `PartitionState` frames, and the run stops if the client leaves. By default, a browser
page may connect only from the server's own host or from a loopback host.

What a client receives is an output like any other. It is the `outputs:` view whose function is
`{type: connection}`, and that view's `condition` filters it. A serve config declares exactly
one such view and uses `outputs:` rather than the `output_condition` / `output_function` pair.
Each connection also writes its own copy of the other views. Put `{connection}` (the
connection's index, from 0) in each view's fields, just as ensembles use `{member}`.

Serve mode does not apply to `macros:` configs yet. The old `--socket socket.yaml` flag still
works as an alias for a batch config with the shorthand pair: it streams the run filtered by
`output_condition`, and doesn't write `output_function`. It is deprecated.

### Varying a run without editing the file

One config can run as many jobs. `--set path=value` replaces one value for this run, and can
be repeated:

```bash
stochadex --config model.yaml \
  --set 'main.partitions[name=price].seed=7' \
  --set main.simulation.termination_condition.max_steps=8000 \
  --set 'run.seeds=[1, 2, 3]'
```

A path joins keys with dots, and selects a list entry by its `name`, never by its position.
The value is read as YAML. The path must already exist in the file, and the new value must
have the old one's shape: a number for a number, a list for a list. So a typo is an error
naming the `--set`, never a run that quietly ignores it. Text takes the value exactly as
written.

A config can also leave a value to the environment with `${VAR}`:

```yaml
    seed: ${SEED}
    params: {rate: [${RATE}]}
outputs:
  - {name: log, function: {type: json_log, path: "runs/${RUN_ID}.log"}}
```

Unquoted, the variable's text reads as YAML, exactly as if it were written there. So
`${RATE}` can be a number, and a whole value can be a list or a `{type: ...}`. Quoted, it is
text. An unset or empty variable is a config error, as is a placeholder in a key. Write
`$${` for a literal `${`.

Overrides are applied before the config is checked, so every check applies to the result.
Ensemble members and served connections all run the overridden config. A server keeps the
config it validated at startup, so editing the file while it runs changes nothing.

### Splitting an ensemble across machines

A member's run depends only on its seed, so an ensemble can be split into shards, each run
on its own machine, and the shards together are the whole ensemble:

```bash
stochadex --config ensemble.yaml --seed-range 1000:1499   # machine 1
stochadex --config ensemble.yaml --seed-range 1500:1999   # machine 2
```

`--seed-range FROM:TO` runs seeds FROM to TO inclusive, in place of `run.seeds`, exactly
as `--set 'run.seeds=[...]'` would. Each shard writes its own outputs and provenance, and
gathering them is left to the workflow engine.

Name members' outputs by `{seed}`, as in `path: "runs/member-{seed}.log"`. `{member}` is a
member's position within its own shard, so it restarts at 0 in every shard, and shards
writing to the same place would overwrite each other. A shard warns when an output uses
`{member}` without `{seed}`.

### Checking a config without running it

`--check` validates a config and exits without running it: every load check, the run mode's
requirements, the partitions' wiring, the deadlock pre-flight, and each iteration's setup. It
reads no inputs and writes nothing, so it works in a pipeline before an earlier step has
produced this one's inputs. The few checks that need an input's contents (which partitions
it has, and their widths where the config doesn't declare them) happen when the run reads
it.

```bash
stochadex --config model.yaml --check     # exit 0 when valid; 78 naming the problem when not
```

`stochadex inspect --io` prints, as JSON, what a run would read and write, without running
it:
- its inputs: kind, location, and what reads them;
- every place it writes: each `outputs:` view, an embedded run's own output, and a stream
  input's `record:` file, with one path per member for an ensemble;
- the run mode, seeds, clock and any overrides.

```bash
stochadex inspect --io -c model.yaml --set 'run.seeds=[1, 2]'
```

The manifest never copies a sink's or source's fields. It gives a location, with any
credentials removed from URLs and connection strings, and names the `${VAR}`s it filled
without their values. From Go, use `api.Check(config)` and `api.Manifest(config)`.

### Skipping runs whose outputs are up to date

`--provenance` writes `<output>.provenance.json` beside each `json_log` and `arrow` file a
run writes. It records what produced the file:
- the config as resolved, after `--set` and `${VAR}`;
- a fingerprint of each input's contents (an S3 object by its version);
- any model file an iteration reads through `model_path`;
- the build that ran it.

Its `key` hashes all of that, so the same key means the same outputs. A sidecar is written
only after the outputs are published.

```bash
stochadex --config model.yaml --skip-if-unchanged   # implies --provenance
```

With `--skip-if-unchanged`, a run whose file outputs all exist and already carry its key is
skipped: it exits 0 and says so on stderr. Change an input, a value, the model file or the
binary, and it runs. `stochadex inspect --provenance -c model.yaml` prints the provenance
and key without running, to use as an idempotency key in a workflow engine. It reads the
inputs, to fingerprint them.

A run has a key only when everything that decides its outputs can be fingerprinted. It
has none, and is never skipped, when:
- an input is a Postgres table (its rows can change under the same query) or a live
  stream;
- it is served;
- the binary isn't a release, a stamped image, or a `go install ...@vX`. A local build
  isn't cached: Go's own commit stamp can't be trusted (a build may have uncommitted
  changes, and inside a git worktree Go stamps the main checkout's commit). To cache
  with a local build, stamp it: `go build -ldflags "-X main.revision=$(git rev-parse HEAD)"`.

The provenance says which applies. Reformatting the config, its comments or its key order
doesn't change the key.

### Running a config from Go

`api.RunWith(config, options...)` runs a config and hands results back in memory.
`api.CaptureView(name, condition)` attaches an in-memory view, which comes back in
`RunResult.Views[name]`. By default the config's own outputs are **suppressed**: nothing is
written outside the process, including by nested runs' sinks. That suits tests and tooling.
`api.WithConfigOutputs()` writes them as well, which suits an orchestrated step that wants
both. `api.RunToStorage(config)` is the shorthand for one view that mirrors the config's own
condition.

```go
config, err := api.LoadConfig("model.yaml",
    api.WithSet("main.partitions[name=price].seed", "7")) // optional overrides, as --set
result, err := api.RunWith(config,
    api.CaptureView("prices", &simulator.OnlyGivenPartitionsOutputCondition{
        Partitions: map[string]bool{"price": true}}),
    api.WithConfigOutputs()) // also write the config's own outputs
prices := result.Views["prices"]
```

### Exit codes

When a run fails, the CLI prints one `stochadex: ...` line to stderr and exits with a code
that says whether retrying could help. The codes follow BSD `sysexits.h`, so a scheduler or
workflow engine can act on them without knowing anything about stochadex:

| Exit code | Meaning | Retry? |
|---|---|---|
| 0 | the run succeeded | — |
| 64 | the command line was wrong | no |
| 65 | an input was reached, but its contents could not be used | no |
| 70 | the simulation failed while running | no |
| 75 | an input or output could not be reached: a missing file, a refused connection | **yes** |
| 78 | the config is invalid: syntax, unknown key or type, wiring, deadlock, run mode | no |

A panic inside a partition's worker goroutine cannot be intercepted, and exits with Go's own
status 2. Treat it like 70. From Go, `api.Execute(os.Args)` returns the classified error, and
`api.ExitCode(err)` maps it to these codes.

## Analysis, inference and optimisation

A `data` block produces a dataset (a sub-simulation, or a `csv` / `json_log` / `postgres` source). Each `macros` entry expands a framework [`macros`](https://stochadex.github.io/pkg/macros.html) constructor into a *set* of partitions against it. All data, all in-process.

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
