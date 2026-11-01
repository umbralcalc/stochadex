---
title: "Config reference"
logo: true
---

# Config reference
<div style="height:0.75em;"></div>

What a stochadex config means, section by section: how a run steps, how partitions read one
another, and what inputs, outputs, run modes and macros do. The [quickstart](quickstart.html)
gets a first config running; this page is the reference behind it.

## A config is one run

Every config describes exactly one run: **inputs → one runtime → outputs**.

- **`inputs:`** are named, read-only data the run reads: a file, a database, an inline series,
  a pre-pass simulation, or a live stream. They are read when the run starts, never when the
  config loads.
- **The runtime** is one coordinator stepping one set of partitions on one clock. It is
  written in the core language: `main:` partitions, each advanced by an `iteration:` or by
  `expressions:`, wired together with `params_from_upstream` and `params_as_partitions`, and a
  `simulation:` block that sets the clock. `embedded:` and the `embedded_simulation_run`
  iteration put whole runs inside a partition.
- **`macros:`** are shortcuts. Each expands into ordinary partitions in the same run, so a
  `macros:` config is its expansion, which `stochadex expand` prints.
- **`outputs:`** are views of the run, each a condition and a destination.
- **`run:`** says how many runtimes there are and how they are paced: one batch run, one per
  seed in an ensemble, or one per client served.

Chaining runs happens outside the engine, through files: one config's output is the next one's
input. Nothing in a config describes stages.

The examples on this page are complete configs. Each is followed by exactly what
`stochadex --config` prints for it, and a test runs every one, so the page stays true to the
engine. With no `outputs:`, a run prints every row to stdout as `<time> <partition> [<state>]`.

## Partitions

A **partition** advances a vector state each step from its **params** and, optionally, other partitions' states.

| Field | Meaning |
|---|---|
| `iteration` | A library process named as data, *or* omit it and supply `expressions`. |
| `params` | Named inputs; every value is a list of float64 (a scalar is `[0.5]`). |
| `params_from_upstream` | Params set from another partition's state within the step (see [how a step runs](#how-a-step-runs)). |
| `params_as_partitions` | Params that name other partitions, passed as their indices, for an iteration that reads their history. |
| `params_from_input` | Params set from an input, row by row or from a live stream (see [inputs](#inputs)). |
| `init_state_values` | The state vector at `init_time_value`. Its length is the state width. |
| `init_state_from_upstream` | Start from an upstream's initial state instead (see [inputs](#inputs)). |
| `state_history_depth` | How many past states to keep (≥1). |
| `seed` | The partition's own random seed. |

The `simulation` block is all data too: a `termination_condition` (`number_of_steps`,
`time_elapsed`, `input_exhausted`), a `timestep_function` (`constant`,
`exponential_distribution`, `from_input`), an `init_time_value`, and optionally an
`execution_strategy` (how partitions are scheduled within a step, which never changes the
result).

### Two ways to write an update

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


## How a step runs

```yaml
# step.yaml
main:
  partitions:
  - name: counter
    iteration: {type: expression, fields: [{name: count}], outputs: ["count + 1"]}
    init_state_values: [0]
    state_history_depth: 1
    seed: 0
  - name: same_step
    iteration: {type: expression, fields: [{name: x}], outputs: ["10 * c"]}
    params: {c: [0]}
    params_from_upstream: {c: {upstream: counter}}
    init_state_values: [-1]
    state_history_depth: 1
    seed: 0
  - name: last_step
    iteration: {type: expression, fields: [{name: x}], upstreams: {m: counter}, outputs: ["10 * m[0]"]}
    init_state_values: [-1]
    state_history_depth: 1
    seed: 0
  - name: clock
    iteration: {type: expression, fields: [{name: time}, {name: number}, {name: size}], outputs: ["t", "step", "dt"]}
    init_state_values: [-1, -1, -1]
    state_history_depth: 1
    seed: 0
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 3}
    timestep_function: {type: constant, stepsize: 0.5}
    init_time_value: 10
```

```text
10 counter [0]
10 same_step [-1]
10 last_step [-1]
10 clock [-1 -1 -1]
10.5 counter [1]
10.5 same_step [10]
10.5 last_step [0]
10.5 clock [10 1 0.5]
11 counter [2]
11 same_step [20]
11 last_step [10]
11 clock [10.5 2 0.5]
11.5 counter [3]
11.5 same_step [30]
11.5 last_step [20]
11.5 clock [11 3 0.5]
```

- **The first row is the initial state**, at `init_time_value`: each partition's
  `init_state_values`. No iteration has run.
- **A step moves the clock by `dt`** (from `timestep_function`) and computes every partition's
  next state. The row is labelled with the new time. Inside the step, `t` is the time the step
  starts from, `step` counts from 1, and `dt` is this step's increment.
- **`number_of_steps: 3` means three steps** after the initial row. `time_elapsed` stops once
  the time has advanced by that much, and `input_exhausted` after an input's last row.
- **Two ways to read another partition, differing only in timing:**
  - **`params_from_upstream` is within the step.** `same_step` sees `counter`'s value from
    *this* step (1 at the first step). It sets a param of the reader to the upstream's
    state, or to the elements `indices:` picks, so the upstream is computed first.
  - **A history read is one step behind.** `last_step` reads `counter` through `upstreams:`,
    which gives its last committed state (0 at the first step). `params_as_partitions` passes
    another partition's index to an iteration that reads its history the same way.
- **Rows come in partition order** within a step, whatever order the partitions were computed
  in, so the same config and seeds give the same output.

### History

A partition keeps its last `state_history_depth` states, and before the run starts that
history holds its initial state. `lag(c, k)` in an expression reads `k` rows further back than
the upstream's last committed state:

```yaml
# history.yaml
main:
  partitions:
  - name: counter
    iteration: {type: expression, fields: [{name: count}], outputs: ["count + 1"]}
    init_state_values: [0]
    state_history_depth: 3
    seed: 0
  - name: delayed
    iteration: {type: expression, fields: [{name: x}], upstreams: {c: counter}, outputs: ["lag(c, 2)[0]"]}
    init_state_values: [-1]
    state_history_depth: 1
    seed: 0
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 4}
    timestep_function: {type: constant, stepsize: 1}
    init_time_value: 0
```

```text
0 counter [0]
0 delayed [-1]
1 counter [1]
1 delayed [0]
2 counter [2]
2 delayed [0]
3 counter [3]
3 delayed [0]
4 counter [4]
4 delayed [1]
```

Reading further back than a partition keeps fails the run, naming the depth to raise. The
check happens as the run reads it, so `--check` passes such a config.

### The one rule: no cycle within a step

Two partitions that each read the other within the step can never run, so the config is
refused before anything runs:

```yaml
# cycle.yaml
main:
  partitions:
  - name: prey
    iteration: {type: expression, fields: [{name: x}], outputs: ["x + p"]}
    params: {p: [0]}
    params_from_upstream: {p: {upstream: predator}}
    init_state_values: [1]
    state_history_depth: 1
    seed: 0
  - name: predator
    iteration: {type: expression, fields: [{name: x}], outputs: ["x + p"]}
    params: {p: [0]}
    params_from_upstream: {p: {upstream: prey}}
    init_state_values: [1]
    state_history_depth: 1
    seed: 0
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 3}
    timestep_function: {type: constant, stepsize: 1}
    init_time_value: 0
```

```text
stochadex: api: simulation wiring will deadlock — params_from_upstream forms a within-step dependency cycle among partitions [prey predator]. Break each cycle by making at least one direction a lag-1 read (a state-history read via params_as_partitions) instead of params_from_upstream
```

Break the cycle by making at least one direction a history read. For mutually coupled models
(predator and prey, say) reading each other one step behind in both directions is the faithful
explicit-Euler step. `stochadex graph` draws the wiring and marks any cycle.

### Seeds

Each partition draws from its own random stream, seeded by its `seed:`, so a run is a function
of its config and inputs: the same ones give the same rows. Under `run: {mode: ensemble}`, each
member's seed replaces every partition's `seed:` with one derived from it, in partition order,
so members are independent runs and each is reproducible from its seed alone.

## Nested runs

An `embedded_simulation_run` partition runs a whole simulation inside itself at every step,
from its `init_state_values`, to its own termination. The partition's state is then the nested
partitions' final states, joined in their order. A params key written
`<nested partition>/<param>` sets that param of the nested run, here from this step's `rate`:

```yaml
# nested.yaml
main:
  partitions:
  - name: rate
    iteration: {type: expression, fields: [{name: r}], outputs: ["r + 1"]}
    init_state_values: [1]
    state_history_depth: 1
    seed: 0
  - name: three_steps
    iteration:
      type: embedded_simulation_run
      run:
        partitions:
        - name: grow
          iteration: {type: expression, fields: [{name: g}], outputs: ["g + k"]}
          params: {k: [0]}
          init_state_values: [0]
          state_history_depth: 1
          seed: 0
        simulation:
          termination_condition: {type: number_of_steps, max_steps: 3}
          timestep_function: {type: constant, stepsize: 1}
          init_time_value: 0
    params: {burn_in_steps: [0]}
    params_from_upstream: {grow/k: {upstream: rate}}
    init_state_values: [0]
    state_history_depth: 1
    seed: 0
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 3}
    timestep_function: {type: constant, stepsize: 1}
    init_time_value: 0
```

```text
0 rate [1]
0 three_steps [0]
1 rate [2]
1 three_steps [6]
2 rate [3]
2 three_steps [9]
3 rate [4]
3 three_steps [12]
```

The nested run's state restarts every step, but its random streams carry on from where the last
step left them, as a simulation advanced alongside the outer one wants. Keys of the same form
can also start the nested partitions from the outer run's history
(`<nested>/initial_state_from_partition_history`) or from a slice of the host's own previous
state (`<nested>/init_state_values_from_outer`). A top-level `embedded:` run is the same thing,
named and hosted by a partition of that name.

### Embedded runs are black boxes

An embedded run (`embedded:`) runs a whole inner simulation inside one partition, from the
start, at every outer step. The outer run sees only the result. At each outer step the host
partition's row is its inner partitions' final states, concatenated in inner partition
order, and outputs record that row like any other. The inner steps themselves are written
nowhere: an embedded run with thousands of inner steps per outer step costs no output and
fills no log.

`stochadex inspect --io` gives each host partition's column layout, so you don't have to
work out which columns hold which inner partition:

```json
"embedded": [{"partition": "forecast", "columns": [
  {"partition": "inner", "offset": 0, "width": 1},
  {"partition": "other", "offset": 1, "width": 2}]}]
```

To debug an embedded run, write every inner step of every outer step to a log, with no
edit to the config:

```bash
stochadex --config model.yaml --debug-embedded forecast=forecast-debug.log
```

A debug log is for debugging, not a result. `inspect --io` lists it under `debug`, it gets
no provenance sidecar, and `--skip-if-unchanged` ignores it. It is verbose by nature: each
inner run follows the last, so inner times repeat once per outer step.


## Inputs

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

An input's first row is the replaying partition's initial state, and each step reads the next
row, so a run over an input of N rows takes N - 1 steps. With the input as the clock
(`timestep_function: {type: from_input}`), the steps land on the input's own times:

```yaml
# replay.yaml
inputs:
  demand:
    source:
      inline:
        times: [0, 1, 3, 6]
        partitions:
          orders: [[5], [7], [2], [9]]
main:
  partitions:
  - name: orders
    iteration: {type: from_input, input: demand}
    state_history_depth: 1
    seed: 0
  - name: backlog
    iteration: {type: expression, fields: [{name: b}], outputs: ["b + o - 4"]}
    params: {o: [0]}
    params_from_upstream: {o: {upstream: orders}}
    init_state_values: [0]
    state_history_depth: 1
    seed: 0
  simulation:
    termination_condition: {type: input_exhausted, input: demand}
    timestep_function: {type: from_input, input: demand}
    init_time_value: 0
```

```text
0 orders [5]
0 backlog [0]
1 orders [7]
1 backlog [3]
3 orders [2]
3 backlog [1]
6 orders [9]
6 backlog [6]
```

The run never steps *to* the first row, but it is data like any other, and the macros count it
(see [Estimating from every row](#estimating-from-every-row)). A `main:` partition that folds
in its upstream's value each step (a running sum or mean) sees only the rows after the first, so
start it from its upstream's initial state with `init_state_from_upstream`, naming one of its
`params_from_upstream` keys:

```yaml
  - name: mean                      # the running mean of every row of flow
    iteration:
      type: expression
      fields: [{name: m}]
      outputs: ["m + (v[0] - m) / (step + 1)"]
    params: {v: [0.0]}
    params_from_upstream: {v: {upstream: flow}}
    init_state_values: [0.0]        # the width; the values come from flow's first row
    init_state_from_upstream: v
    state_history_depth: 1
    seed: 0
```

The partition starts from the upstream's initial state at that key's indices, however that state
was set, here from the input when the run starts.

An input is any `source:` (`csv`, `json_log`, `postgres`, plus `arrow` and `s3` in the
distributed CLI), or a pre-pass simulation (`{simulation: {steps, timestep, partitions: ...}}`).
A run's `json_log` output can be the next run's input, which is how separate configs chain.

A short series can be carried in the config itself, as an `inline` source. It needs a row
per time, and an input with only `times:` can drive a clock:

```yaml
inputs:
  sun:
    source:
      inline:
        times: [0.0, 1.0, 2.0, 3.0]
        partitions: {clear_sky: [[0.0], [5.0], [30.0], [120.0]]}
```

A `from_storage` iteration or timestep function with inline `data:` is shorthand for an
inline input read by `from_input`, so `inspect --io` and provenance see that data as an
input too.

A file an iteration reads, such as an ONNX model, is an input too. Declare it with `file:`
and name it in the iteration field that takes its path:

```yaml
inputs:
  brain: {file: {path: models/brain.onnx}}
main:
  partitions:
  - name: policy
    iteration: {type: onnx_inference, model_path: {input: brain}}
    # ...
```

Any iteration field written `{input: NAME}` is given that file's path when the config
loads. A plain `model_path: models/brain.onnx` is shorthand for a file input of that path.
`--check` doesn't read model files, and a run reads them when it starts.

Inputs are read when the run starts, never when the config is loaded:
- a missing input exits as unavailable (75);
- an input lacking the named partition exits as a data error (65);
- an undeclared or unused input is a config error (78).

A `macros:` config reads `inputs:` too: its macros analyse every input's partitions. When
there are several inputs they must share one time axis, and a partition name may come from
only one of them. (A top-level `data:` block, the older spelling of a single input named
`data`, still works but is deprecated: it prints a notice naming its replacement.)

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

#### Steering a run from the keyboard

A stream can be the keyboard. Each mapped key sets a stream partition's value, which holds
until the next mapped key. Unmapped keys are ignored, and Esc stops listening; the run
carries on with the last value. Pace the run so there's time to press anything:

```yaml
inputs:
  keys:
    stream:
      keyboard: {partition: steer, keys: {a: [-1.0], d: [1.0], s: [0.0]}}
    record: keys.log                # what the keyboard gave each step, for replay
main:
  partitions:
  - name: steering
    iteration: {type: param_values}
    params: {param_values: [0.0]}
    params_from_input: {param_values: {input: keys, partition: steer}}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 0
  # ... partitions that read steering via params_from_upstream ...
run: {pace_ms: 200}                 # a step every 200ms
```

Replaying `keys.log` as `{source: {json_log: {path: keys.log}}}` repeats the run exactly.
The keyboard is available in the `stochadex` CLI; from Go, import
`github.com/umbralcalc/stochadex/pkg/keyboard` to register it.

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


## Outputs

Results leave a run through a top-level `outputs:` list of *views*. Each view has a name, an
optional `condition` (default `every_step`), and a `function`:

```yaml
outputs:
  - {name: log, function: {type: json_log, path: run.log}}
  - {name: sparse, condition: {type: every_n_steps, n: 10}, function: {type: stdout}}
```

Conditions are `every_step`, `every_n_steps`, `only_given_partitions`, `final_step` and `nil`.
`final_step` writes each partition once, as it stands when the run ends (its last step, whatever
the termination condition), and `{type: final_step, partitions: [a, b]}` writes only those
partitions; it is the one to use for an estimation run, which wants the final estimate and not
a row per step. Functions are
`stdout`, `json_log`, `websocket`, `arrow`, `duckdb`, `postgres`, `s3` and `nil`. A config with
no `outputs:` prints every step to stdout. (The older `simulation.output_condition` /
`output_function` pair is deprecated shorthand for one view named `output`; a config that
uses it prints a notice giving the exact `outputs:` line to replace it with.)

Each view applies its own condition as the run goes. Here `every_n_steps` writes steps 0, 2
and 4, and `final_step` writes `double` once, as the run ends:

```yaml
# views.yaml
main:
  partitions:
  - name: counter
    iteration: {type: expression, fields: [{name: count}], outputs: ["count + 1"]}
    init_state_values: [0]
    state_history_depth: 1
    seed: 0
  - name: double
    iteration: {type: expression, fields: [{name: x}], outputs: ["2 * c"]}
    params: {c: [0]}
    params_from_upstream: {c: {upstream: counter}}
    init_state_values: [0]
    state_history_depth: 1
    seed: 0
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 5}
    timestep_function: {type: constant, stepsize: 1}
    init_time_value: 0
outputs:
- {name: sparse, condition: {type: every_n_steps, n: 2}, function: {type: stdout}}
- {name: final, condition: {type: final_step, partitions: [double]}, function: {type: stdout}}
```

```text
0 counter [0]
0 double [0]
2 counter [2]
2 double [4]
4 counter [4]
4 double [8]
5 double [10]
```

### Columnar, streaming and database outputs

Beyond `stdout` and `json_log`, write columnar output directly:

```yaml
outputs:
  - {name: columns, function: {type: arrow, path: run.arrow}}                     # Arrow IPC file
  - {name: table, function: {type: duckdb, path: run.duckdb, table: results}}   # DuckDB table
```

To stream a run live to another service (a dashboard, a recorder), push it to a websocket
server. The run connects as a client when it starts, sends each output as a protobuf
`PartitionState` frame, and closes the connection when it finishes:

```yaml
outputs:
  - {name: push, function: {type: websocket, url: "ws://localhost:8080/ingest"}}
```

`postgres` takes local credentials, or `driver`/`dsn` through `database/sql` to reach **any Postgres-wire database** (TimescaleDB, CockroachDB, a managed instance):

```yaml
outputs:
  - {name: db, function: {type: postgres, driver: pgx, dsn: "postgres://...", table: results}}
```

`s3` is a **transport, not a format**: give it a `format:` and it reuses the normal sink, so anything writable locally is writable to object storage. Credentials come from the standard AWS chain, never the config file. Set `endpoint:` for any S3-compatible store (MinIO, R2, Ceph):

```yaml
outputs:
  - {name: lake, function: {type: s3, bucket: my-bucket, key: runs/out.arrow, format: arrow}}
```

The same formats work as input sources to read a run back in: `{arrow: {path: run.arrow}}`, `{postgres: {...}}`, `{s3: {bucket, key, format}}`. Name a source the binary lacks and the error lists the ones it has.

`arrow` writes one IPC file (a `time` column plus a fixed-size list column per partition), read natively by Polars, pandas and DuckDB:

```python
import pyarrow.ipc as ipc
table = ipc.open_file("run.arrow").read_all()
```

`duckdb` lands the same data in a DuckDB table (zero-copy). Both write once at the end, so they need a view whose condition emits every partition every step.

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

An embedded run's debug log is published with its outer run. A served connection's
outputs are published when its session ends, and the client leaving counts as a clean end.
A killed run can leave a `.partial` file behind, which the next run replaces. Publishing
costs one rename per file, once per run and never per step.


### Several outputs from one run

To send one run to several places, each with its own filter, list several views under
`outputs:`:

```yaml
outputs:
  - {name: log,  function: {type: json_log, path: run.log}}
  - {name: dash, condition: {type: only_given_partitions, partitions: [price]},
     function: {type: websocket, url: "ws://localhost:8080/ingest"}}
  - {name: db,   condition: {type: every_n_steps, n: 10},
     function: {type: postgres, driver: pgx, dsn: "postgres://...", table: results}}
```

`outputs:` also applies to a `macros:` config: its results go to the views, each applying its
condition exactly as a live run would, in time order.

With `run: {mode: ensemble}`, each member gets its **own** sinks, the shorthand's included:
write `{member}` (the member's index) or `{seed}` into a view's fields, e.g.
`path: "run-{member}.log"`, and every member writes its own file. Quote templated values in
YAML flow style (`{...}`). Every ensemble view must use a placeholder, or all members would
write to the same place; outside an ensemble, a placeholder is an error. `stdout` needs none:
each member's rows are prefixed `member=<i> seed=<s>` (members run concurrently, so their
rows interleave, each member's in order).


## Run modes

```yaml
run:
  mode: ensemble           # one member per seed, run concurrently
  seeds: [11, 22, 33, 44]  # output rows are prefixed member=<i> seed=<s>
  # concurrency: 4         # optional; defaults to GOMAXPROCS
```

Omit `run` for a single batch run. Every mode applies to a `macros:` config too: an ensemble
of a posterior estimate, say, gives one estimate per seed. `run: {pace_ms: 200}` paces a batch run from the CLI in
real time, a step every 200ms, for watching it or steering it from the keyboard. Results
taken from Go with `RunWith` or `RunToStorage` are not paced.

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
one such view.
Each connection also writes its own copy of the other views. Put `{connection}` (the
connection's index, from 0) in each view's fields, just as ensembles use `{member}`.

The old `--socket socket.yaml` flag still
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


## Macros: analysis, inference and optimisation

The config's `inputs:` produce a dataset (a sub-simulation, or a `csv` / `json_log` / `postgres` source). Each `macros` entry expands a framework [`macros`](https://stochadex.github.io/pkg/macros.html) constructor into a *set* of partitions against it. All data, all in-process.

```yaml
inputs:
  data:
    simulation:
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

The macros are the aggregations (`vector_mean`, `vector_variance`, `vector_covariance`,
`grouped_aggregation`), `scalar_regression_stats`, `likelihood_comparison`,
`likelihood_mean_function_fit` and `posterior_estimation`, which analyse the inputs and the
run's partitions; and four with a clock of their own, `evolution_strategy_optimisation`,
`smc_inference`, `mcts_self_play` and `mcts_planning`. `stochadex catalogue` lists each one's
fields.

### The run a macros config makes

A `macros:` config runs as its expansion, one ordinary run:
- **Its partitions:** each input's partitions, replayed under their own names, then the
  `main:` partitions, then each macro's partitions, in order. A macro reads the inputs, the
  `main:` partitions and earlier macros' outputs live, and a `main:` partition can read a
  macro's output. A partition name may be defined only once across all of them.
- **Its clock:** `main.simulation` when the config states one; otherwise the clock of the one
  macro that has its own (steps of a timestep, or an SMC run's rounds); otherwise the inputs',
  stopping when they run out. A run has one clock, so a second macro with its own is refused,
  and several inputs must share one time axis.
- **Its histories:** a macro reading a partition over a window keeps that partition's history
  as deep as the window. Two macros reading one partition over different windows are refused.
- **Calibrate, then plan:** macros that calibrate before a planner with its own clock run first,
  as a `run:` input the planner reads.

### Seeing what macros expand to

A `macros:` config runs as its expansion: the same run written out in the core config
language, with the inputs replayed as partitions and each macro's partitions spelled out.
`stochadex expand` prints it without running it, reading only the inputs' shapes:

```bash
stochadex expand -c model.yaml                     # YAML, to stdout
stochadex expand -c model.yaml > expanded.yaml     # a config of its own, which runs the same
```

The output is laid out to be read: a component that fits on a line stays on it, a longer one is
written field by field, and one that appears more than once (a planning environment the search
reads in several places) is written once as a YAML anchor and referred to by alias after. It takes
`--set` like a run does, and exits as a run would on a config error (78) or an input it cannot
reach (75). From Go, `api.Expand(config)` returns it as a config and `api.ExpandToYAML(config)`
as text.

### Estimating from every row

Every row of a macro's data counts, exactly once, the first included: an input's first row is
its replay's initial state, which the windowed aggregations read from history and
`scalar_regression_stats` and a cumulative `vector_mean` start from. (A grouped aggregation's
window counts only the rows the run has reached, so early steps are not padded with zeros.)
Two things decide whether you get an estimate:

- **A windowed aggregation returns `default_value` until its window has filled.**
  `vector_mean`, `vector_variance` and `vector_covariance` read `window:` rows of history
  (default 1) and output `default_value` (default 0) for the
  first `window` steps. A window longer than the run would output that every step, so a run of a
  file input fails when the run starts (as it reads its inputs; `--check` reads none), naming
  the fix, instead.
- **For the mean over every row, use `cumulative: true`.** A `vector_mean` with
  `cumulative: true` (and no `window:`, `kernel:`, `params:` or `default_value:`) is the running
  mean of all rows so far, each weighted equally. A step costs the same however long the data,
  where a window as long as the data would cost a pass over it every step.

So a closed-form rate estimate from a CSV of counts and exposures — the Poisson maximum
likelihood rate is total count over total exposure, which is the ratio of their means — is a
cumulative mean written once, at the end:

```yaml
macros:
- type: vector_mean
  name: means                    # [mean count, mean exposure] over every row
  data: {partition_name: obs}
  cumulative: true
outputs:
- {name: estimate, condition: {type: final_step, partitions: [means]},
   function: {type: json_log, path: estimate.log}}
```

[`cfg/example_poisson_rate_config.yaml`](https://github.com/umbralcalc/stochadex/blob/main/cfg/example_poisson_rate_config.yaml)
does this per group, with the rates themselves as a `main:` partition.

### The learning macros have levers

Four macros *converge* or merely *run* depending on hyperparameters. Each ships as a converging example under [`cfg/`](https://github.com/umbralcalc/stochadex/tree/main/cfg), pinned by a test that asserts it recovers a known answer:

| Macro | Recovers | Levers that decide convergence |
|---|---|---|
| `evolution_strategy_optimisation` | a reward's optimum | Keep covariance `learning_rate` slow (≈0.1); a fast rate collapses the search width before the mean arrives. `discount_factor: 0.0` for a static objective. |
| `posterior_estimation` | the data-generating parameters | The `comparison` **must** read the sampler (the posterior weights *sampled* params by loglike). Proposal covariance wide enough to explore prior→truth; `past_discount` near 1. |
| `smc_inference` | the observed stream's mean | `num_particles`, `num_rounds`, `priors` ranges. |
| `scalar_regression_stats` | slope and intercept | None; OLS is closed-form. With an intercept, cumulative mode is width 9: `[n, Sx, Sy, Sxx, Sxy, Syy, alpha, beta, sigma2]`. |


## Checking, inspecting and running from Go

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
- every place it writes its results: each `outputs:` view and a stream input's `record:`
  file, with one path per member for an ensemble;
- each embedded run's column layout, and any debug logs, listed apart (see "Embedded runs
  are black boxes");
- the run mode, seeds, clock and any overrides.

```bash
stochadex inspect --io -c model.yaml --set 'run.seeds=[1, 2]'
```

The manifest never copies a sink's or source's fields. It gives a location, with any
credentials removed from URLs and connection strings, and names the `${VAR}`s it filled
without their values. From Go, use `api.Check(config)` and `api.Manifest(config)`.

### Drawing a config's wiring

`stochadex graph` prints the run's partition dependency graph without running it:
one node per partition, a solid edge for each `params_from_upstream` read (exact
within-step wiring), and a dashed edge from a past copy of the producer for each
`params_as_partitions` reference (a hint that the partition may read the other's history,
one step behind). It prints Mermaid by default, or Graphviz DOT with `--format dot`, and
takes `--set` like a run does. A `macros:` config's graph is its expansion's, the
partitions it runs as; it reads the config's inputs, as the run would.

```bash
stochadex graph -c model.yaml                                # Mermaid, to stdout
stochadex graph -c model.yaml --format dot | dot -Tsvg > model.svg
```

A within-step cycle, which would deadlock the run, is marked in the graph. The command
still prints the graph, then names the cycle on stderr and exits 78, so it works as a CI
guard. (`stochadex-graph --config ...`, the old separate binary, still works from a
checkout but is deprecated, and releases no longer ship it.)

### What a build can run

`stochadex catalogue` prints, as JSON, what this binary can run: its version and compiled-in
features, and every component a config can name by type, read from the registries the config
resolves through. For each one the engine ships, it also says what the component is, the
fields its `{type: ...}` spec takes, and the `params:` it reads from its partition. A
downstream registration, such as `onnx_inference` in a build with ONNX, appears by name in
the build that links it.

```bash
stochadex catalogue | jq '.iterations[].name'
```

Every list is sorted, so the output is identical for identical builds:

```json
{
  "catalogue_version": 2,
  "version": "v0.26.0",
  "features": ["arrow", "postgres", "s3"],
  "iterations": [{"name": "bernoulli_process"}, ...],
  "data_sources": [{"name": "arrow"}, {"name": "csv"}, ...],
  "stream_transports": [{"name": "connection"}, {"name": "keyboard"}, {"name": "websocket"}],
  "output_functions": [...],
  "output_conditions": [...],
  "termination_conditions": [...],
  "timestep_functions": [...],
  "execution_strategies": [...],
  "macros": [...],
  "kernels": [...],
  "likelihoods": [...],
  "priors": [...],
  "jumps": [...],
  "environments": [{"name": "simulation"}, {"name": "tictactoe"}]
}
```

An entry the engine ships says what it takes:

```json
{
  "name": "values_function_vector_mean",
  "description": "ValuesFunctionVectorMeanIteration computes a kernel-weighted rolling mean of a function over historical values and times.",
  "fields": [
    {"name": "function", "required": true, "choices": [
      {"name": "data_values", "description": "...",
       "params": [{"name": "data_values_indices", "optional": true},
                  {"name": "data_values_partition"}, {"name": "latest_data_values"}]},
      ...]},
    {"name": "kernel", "required": true}
  ],
  "params": [{"name": "subtract_from_normalisation", "optional": true}, ...]
}
```

- **`fields`** are the keys of its spec; `required` marks those it cannot do without. A field
  that names one of a set (`function: data_values`) lists its `choices`, each with the params
  it reads. A macro's, a source's or an environment's fields nest as its YAML does.
- **`params`** are the names it reads from its partition's `params:`, which
  `params_from_upstream` and `params_as_partitions` also supply. A name ending in `*` is a
  family (`transitions_from_*` is `transitions_from_0`, `_1`, ...); `optional` marks one it
  reads only if present. A component a partition's iteration takes, its kernel or likelihood,
  reads its own params from the same partition, so look in its entry too.
  `params_incomplete` marks a type that also reads params whose names a config chooses, as an
  `expression` reads the ones its formulas name.

All of this is read from the engine's source, and checked against every shipped config.
`catalogue_version` changes only if a key is removed or changes meaning; new keys may
appear within a version. From Go, use `api.BuildCatalogue()`.

### Skipping runs whose outputs are up to date

`--provenance` writes `<output>.provenance.json` beside each `json_log` and `arrow` file a
run writes. It records what produced the file:
- the config as resolved, after `--set` and `${VAR}`;
- a fingerprint of each input's contents, model files included (an S3 object by its
  version);
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


## Where to look next

- [How it works](how_it_works.html): the engine behind partitions and histories, and how to
  write an iteration in Go.
- [`cfg/`](https://github.com/umbralcalc/stochadex/tree/main/cfg): worked example configs.
- [robert](https://github.com/umbralcalc/robert): authoring and running stochadex configs with
  an AI agent.
