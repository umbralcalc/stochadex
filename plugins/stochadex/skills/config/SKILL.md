---
name: config
description: Write, check, run and debug a stochadex config, a single YAML file that states a stochastic simulation as data and runs with the stochadex CLI, with no Go and no compilation. Use when the user wants to simulate or model a stochastic process or complex system (diffusions, jump and point processes, epidemics, populations, queues, markets, multi-component systems), asks for a stochadex config, or has one that fails to load, deadlocks or gives surprising numbers. Covers partitions, expressions, same-step and one-step-behind reads, the catalogue of built-in processes, inputs, outputs, ensembles and the analysis macros.
---

# Write and run a stochadex config

A stochadex config is one YAML file that is one simulation. The `stochadex` CLI loads it and
runs it in-process. Nothing is compiled, and every part of the file is data.

The engine documents itself, and what it prints is the truth for the version you are driving.
This skill gives the model of a config that you need to write one. For anything it does not
cover, ask the binary rather than guessing:

```bash
stochadex catalogue                 # JSON: every {type: ...} this binary runs, with its fields, params and update rule
stochadex --config model.yaml --check   # validate without running; exit 0 or 78 with the problem named
stochadex graph -c model.yaml       # the wiring as a Mermaid flowchart
stochadex expand -c model.yaml      # what a macros: config runs as
stochadex inspect --io -c model.yaml       # JSON: what a run reads and writes
stochadex inspect --suggest -c model.yaml  # JSON: built-ins that an expression restates
```

The written semantics are at <https://stochadex.github.io/pkg/reference.html>, and worked
configs are in <https://github.com/umbralcalc/stochadex/tree/main/cfg>.

## The CLI

Run `stochadex --version` first. It prints the version and a `features:` line. If the command
is missing, tell the user and offer to install it. Do not download anything before they agree.

```bash
curl -L "https://github.com/umbralcalc/stochadex/releases/latest/download/stochadex-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')" -o stochadex
chmod +x stochadex
```

Each release carries a `SHA256SUMS` file to check the download against. A container
(`ghcr.io/umbralcalc/stochadex:latest`, working directory `/work`) and
`go build` in a checkout's `cmd/stochadex` give the same CLI.

## A config is partitions on one clock

A **partition** holds a vector state and advances it every step. A config is a list of
partitions, the clock they share, and where the results go.

```yaml
# growth.yaml
partitions:
- name: growth
  iteration:
    type: expression
    fields: [{name: size}]                 # names the state's slots, in order
    bindings:                              # optional intermediates, evaluated in order
    - {name: drift, expr: "rate * size * (1 - size / capacity)"}
    outputs:                               # one expression per field: the next state
    - "size + drift * dt + noise * size * shared(normal(0, 1)) * sqrt(dt)"
  params: {rate: [0.05], capacity: [100.0], noise: [0.1]}
  init_state_values: [10.0]                # its length is the state's width
  state_history_depth: 1                   # how many past states it keeps
  seed: 42                                 # its own random stream
simulation:
  termination_condition: {type: number_of_steps, max_steps: 3}
  timestep_function: {type: constant, stepsize: 1.0}
  init_time_value: 0.0
```

```text
0 growth [10]
1 growth [10.177172108511419]
2 growth [9.550670564567534]
3 growth [8.826107025832622]
```

With no `outputs:`, every row prints as `<time> <partition> [<state>]`. The first row is the
initial state, and `number_of_steps: 3` is three steps after it.

- **Every `params` value is a list**: a scalar is `[0.5]`, never `0.5`.
- **`simulation`** takes a `termination_condition` (`number_of_steps` with `max_steps`,
  `time_elapsed` with `max_time_elapsed`, `input_exhausted`), a `timestep_function`
  (`constant` with `stepsize`, `exponential_distribution`, `from_input`) and an
  `init_time_value`.
- **A partition's update** is an `iteration: {type: ...}`: a built-in process, or
  `{type: expression}` for maths of your own. (A top-level `expressions:` list, each entry
  naming its `partition:`, is the same thing written apart from the partitions.)

## Built-in first, expression second

A built-in is compiled and runs the same maths several times faster than an expression, so
look for one before writing an expression. Search the catalogue by what a process computes:

```bash
stochadex catalogue | jq -r '.iterations[] | "\(.name): \(.update // .description)"'
stochadex catalogue | jq '.iterations[] | select(.name == "ornstein_uhlenbeck")'
```

An entry's `params` are the keys it reads from the partition's `params:`, its `fields` are
the keys of its `{type: ...}` spec, and its `update` is the rule it computes, in a notation
where `x` is the state, `x@1` the last committed state and `Z ~ N(0, 1)` a draw. Common ones:
`wiener_process`, `ornstein_uhlenbeck`, `geometric_brownian_motion`, `drift_diffusion`,
`poisson_process`, `compound_poisson_process`, `hawkes_process`, `bernoulli_process`,
`categorical_state_transition`, `cumulative`, `constant_values`, `param_values`.

Read an entry's `update` before using it; the name is not enough. `poisson_process`, for
one, is a running count that adds at most one event a step, right for rare events on a fine
clock. For the number of arrivals in a coarse step (customers in a day), write the draw as an
expression: `shared(poisson(rate * dt))`.

When you have written an expression anyway, `stochadex inspect --suggest -c model.yaml` finds
each one that restates a built-in, checks the replacement gives the same numbers, and prints
the YAML to use.

## The expression language

An expression may read:

- the partition's own **fields** (its last committed state);
- its **params** keys, including ones set by `params_from_upstream`;
- an **upstream alias** from `upstreams:` (another partition's last committed state, indexed
  as `alias[0]`);
- `dt` (this step's increment), `t` (the time the step starts from), `step` (counting from
  1) and `pi`;
- any earlier **binding**.

These share one namespace, so no two may have the same name and none may be called `dt`, `t`,
`step` or `pi`. Everything is elementwise over vectors, and a length-1 value broadcasts.

- **Functions:** `where(cond, a, b)`, `clamp(x, lo, hi)`, `min`, `max`, `abs`, `floor`, `exp`,
  `log`, `sqrt`, `pow`, `sin`, `cos`, `erf`, `erfc`, `fill`, `width`, `slice(v, start, n)`,
  `concat`, `sum`, `dot`, `lag(name, k)`, arithmetic, the comparisons, and `&&` and `||`.
- **Draws:** `normal(mean, sd)`, `uniform(lo, hi)`, `exponential(rate)`, `poisson(mean)`,
  `gamma(shape, rate)`, `beta(a, b)`, `binomial(n, p)`.
- **Not elementwise:** `each(n, i, expr)` builds a width-`n` value whose element `i` is
  `expr`, for an index shift such as an ageing cohort. `scan(n, i, acc, init, expr)` is the
  same loop with an accumulator carried from lane to lane.

Three mistakes account for most failed loads, and the error names each:

1. **A draw with scalar parameters has no width.** A one-element param counts as a scalar.
   Wrap the draw: `shared(normal(0, 1))` is one sample used across the state, and
   `iid(n, normal(0, 1))` is `n` independent ones. A draw whose parameter is already a vector
   needs no wrapper.
2. **YAML reads `y`, `n`, `yes`, `no`, `on`, `off` as booleans.** Quote a field named so:
   `{name: "y"}`.
3. **`lag(name, k)` needs history.** It reads only a field or an upstream alias, and the
   partition read must keep `state_history_depth` of at least `k + 1`.

The catalogue does not list these functions. When unsure whether the language can say
something, write it in a three-step config and run `--check`.

## Reading another partition: this step, or one step behind

Partitions never call each other. One reads another in one of two ways, which differ only in
timing. Choosing between them is the main modelling decision in a config.

| Read | Written as | Sees | Use it when |
|---|---|---|---|
| **This step** | `params_from_upstream: {key: {upstream: other}}` on the reader, read as `key` | the other's value computed *this* step, so the other runs first | a quantity derived from another (a flow from a level, an observation of a state) |
| **One step behind** | `upstreams: {alias: other}` in the expression, read as `alias[0]` | the other's last committed state | feedback and mutual coupling |

`params_from_upstream` takes `indices: [0, 2]` to pick elements of the upstream's state. A
built-in that reads another partition's history takes its name through
`params_as_partitions: {key: [other]}`; the catalogue entry says which keys.

**The one rule: no cycle of this-step reads.** If A reads B this step and B reads A this
step, neither can go first, and the config is refused with the cycle named. Break it by
making at least one direction one step behind. For two things that drive each other
(predator and prey, stock and price) reading one step behind in both directions is the
faithful explicit Euler step.

```yaml
# reservoir.yaml
partitions:
- name: inflow                      # a built-in: mean-reverting river flow
  iteration: {type: ornstein_uhlenbeck}
  params: {mus: [4.0], thetas: [0.5], sigmas: [1.0]}
  init_state_values: [4.0]
  state_history_depth: 1
  seed: 11
- name: reservoir                   # a water balance
  iteration:
    type: expression
    fields: [{name: level}]
    upstreams: {out: release}       # release, one step behind: breaks the loop
    outputs: ["max(level + (q - out[0]) * dt, 0)"]
  params: {q: [0.0]}
  params_from_upstream: {q: {upstream: inflow}}       # inflow, this step
  init_state_values: [50.0]
  state_history_depth: 1
  seed: 0
- name: release                     # the operating rule: spill above the target
  iteration:
    type: expression
    fields: [{name: flow}]
    outputs: ["where(lvl > target, gain * (lvl - target), 0)"]
  params: {lvl: [0.0], target: [40.0], gain: [0.5]}
  params_from_upstream: {lvl: {upstream: reservoir}}  # reservoir, this step
  init_state_values: [0.0]
  state_history_depth: 1
  seed: 0
simulation:
  termination_condition: {type: number_of_steps, max_steps: 2}
  timestep_function: {type: constant, stepsize: 0.5}
  init_time_value: 0.0
```

```text
0 inflow [4]
0 reservoir [50]
0 release [0]
0.5 inflow [4.343721410213715]
0.5 reservoir [52.17186070510686]
0.5 release [6.085930352553429]
1 inflow [4.320595898388047]
1 reservoir [51.28919347802417]
1 release [5.6445967390120835]
```

`stochadex graph -c reservoir.yaml` draws exactly this: a solid edge for each this-step read
and a dashed edge, from a shaded past copy of the partition read, for each read one step
behind.

### A delay

`lag(name, k)` reads a field or an upstream alias `k` rows further back than its last
committed state, so `lag(a, 0)` is `a`. An upstream alias is already one step behind, which
makes **a delay of D steps `lag(alias, D - 1)`**, with the partition read keeping
`state_history_depth: D`. Index the result as usual (`lag(o, 2)[0]`), and `k` may be a param,
so `--set` can change the delay.

```yaml
# delay.yaml
partitions:
- name: orders                      # an order of 5 placed at step 2, and nothing else
  iteration: {type: expression, fields: [{name: placed}], outputs: ["where(step == 2, 5, 0)"]}
  init_state_values: [0]
  state_history_depth: 3            # a delay of 3 steps reads 2 rows back: keep 3
  seed: 0
- name: shelf                       # receives each order 3 steps after it is placed
  iteration:
    type: expression
    fields: [{name: stock}]
    upstreams: {o: orders}
    outputs: ["stock + lag(o, delay - 1)[0]"]
  params: {delay: [3]}
  init_state_values: [0]
  state_history_depth: 1
  seed: 0
simulation:
  termination_condition: {type: number_of_steps, max_steps: 6}
  timestep_function: {type: constant, stepsize: 1}
  init_time_value: 0
```

```text
0 orders [0]
0 shelf [0]
1 orders [0]
1 shelf [0]
2 orders [5]
2 shelf [0]
3 orders [0]
3 shelf [0]
4 orders [0]
4 shelf [0]
5 orders [0]
5 shelf [5]
6 orders [0]
6 shelf [5]
```

Only committed state can be lagged. To delay a value a partition reads this step (a param
set by `params_from_upstream`), have a reader lag the *producer* through `upstreams:`, as
here. A lag counts steps, not time: a 3-day delay is 3 steps only while a step is a day, so
state the delay in steps as a param and change it whenever the step size changes.

## Where results go, and running many times

```yaml
run: {mode: ensemble, seeds: [1, 2, 3, 4]}       # one independent run per seed
outputs:
- {name: log, function: {type: json_log, path: "runs/member-{seed}.log"}}
- {name: ends, condition: {type: final_step}, function: {type: stdout}}
```

- **`outputs:`** is a list of views, each a `name`, an optional `condition` (`every_step`,
  `every_n_steps` with `n`, `only_given_partitions` with `partitions`, `final_step`) and a
  `function` (`stdout`, `json_log`, `arrow`, and others the catalogue lists).
- **A `json_log`** has one line per row: `{"partition_name": ..., "state": [...], "time": ...}`.
  Its folder must exist before the run. A run's files appear only when it ends cleanly.
- **`run: {mode: ensemble}`** runs once per seed, concurrently. A member's seed replaces
  every partition's `seed:`, so a member is reproducible from its seed alone. Every file view
  needs `{seed}` or `{member}` in its path, quoted.
- **Vary a run without editing the file:** `--set path=value`, repeatable, as in
  `--set 'partitions[name=release].params.gain=[0.1]' --set 'run.seeds=[1, 2]'`. A list
  entry is chosen by its `name`, the path must already exist in the file, and the value must
  have the old one's shape. `${VAR}` in a value is filled from the environment.

## Data in, and analysis on top

- **`inputs:`** are named, read-only data: `{source: {csv: {path, time_column, state_columns}}}`,
  `json_log`, `arrow`, a short series written `inline`, or a pre-pass `simulation`. A
  partition replays one with `iteration: {type: from_input, input: NAME}`, a param follows
  one with `params_from_input`, and the clock can follow one with `timestep_function:
  {type: from_input, input: NAME}` and `termination_condition: {type: input_exhausted, input:
  NAME}`. One run's `json_log` can be the next run's input.
- **`macros:`** are shortcuts that expand into ordinary partitions in the same run: rolling
  and cumulative statistics (`vector_mean`, `vector_variance`, `vector_covariance`,
  `grouped_aggregation`, `scalar_regression_stats`), inference (`posterior_estimation`,
  `smc_inference`, `likelihood_comparison`), optimisation (`evolution_strategy_optimisation`)
  and planning over the model (`mcts_planning`). Take each one's fields from
  `stochadex catalogue | jq '.macros[] | select(.name == "vector_mean")'`, start from the
  matching `example_*_config.yaml` in the repository's `cfg/` folder, which a test pins to a
  known answer, and read `stochadex expand` to see what you built. The inference and
  optimisation macros converge or merely run depending on a few settings, which the config
  reference lists under "The learning macros have levers".
- **`embedded_simulation_run`** runs a whole inner simulation inside one partition at every
  outer step, for a forecast or a nested ensemble. `inspect --io` gives its column layout.

## When a run fails

The CLI prints one `stochadex: ...` line that names the partition and the fix, and exits
with a code that says what kind of problem it is:

| Exit | Meaning | What to do |
|---|---|---|
| 78 | the config is invalid: syntax, unknown key or type, wiring, a cycle | fix the config as the message says |
| 65 | an input was read but its contents can't be used | fix the data or the columns named |
| 75 | an input or output could not be reached | check the path; retrying can help |
| 70 | the simulation failed while running | read the message; often a width or a NaN |
| 64 | the command line was wrong | fix the flags |

Always run `--check` before a run, and read the error before changing anything: it says what
to change. Every command also prints a `stochadex-run version=...` line to stderr, so stdout
stays clean for `jq` and for redirecting into a file.
