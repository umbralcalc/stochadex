---
name: stochadex-model
description: Author, run, and analyse a stochastic simulation as a single YAML config for the stochadex engine — no Go, no compilation, no toolchain. Use when the user wants to model or simulate a stochastic process or complex system (SDEs, diffusions, point/jump processes, epidemic/population/queueing/financial dynamics, agent-based or multi-component systems), build or debug a stochadex config file, run an ensemble, or do online inference / aggregation / optimisation over a simulation. Covers the expressions DSL for bespoke maths, the {type: ...} iteration / kernel / likelihood / macro registries, partition wiring and the deadlock rule, run modes, and the data:/macros: analysis tier. This file is self-contained — everything needed to write a working config is here.
---

# Author a stochadex model as a config

Build, run, and analyse a stochastic simulation by writing **one YAML file** — no Go, no
compilation, no toolchain. You describe a system; `stochadex --config file.yaml` resolves and
runs it in-process. Everything you need to author a working config is here.

## Prerequisite: the `stochadex` CLI

Authoring needs the `stochadex` binary on PATH to run configs. It is a single prebuilt
executable — no Go toolchain required to run a data-spec config (the kind this skill writes).

```bash
# Prebuilt binary (no Go needed) — pick your platform's asset:
curl -L https://github.com/umbralcalc/stochadex/releases/latest/download/stochadex-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/') -o stochadex
chmod +x stochadex && ./stochadex --config model.yaml

# Or, with a Go toolchain, build from a checkout (the CLI is its own module):
git clone https://github.com/umbralcalc/stochadex && cd stochadex/cmd/stochadex && go build -o stochadex .
```

If `stochadex` is missing, stop and tell the user to install it (above) rather than guessing —
authoring is fine, but you cannot *run* a config without it.

**Check what you have before writing a config that needs it:**

```bash
stochadex --version
# stochadex v0.5.3 darwin/arm64
# features: arrow postgres s3          <- portable build
# features: arrow cblas duckdb postgres s3   <- accelerated build
```

**Two builds exist.** The default asset above is pure Go. An `stochadex-accel-<os>-<arch>` asset
(same URL with `stochadex-accel-`) adds `cblas` (an optimised system BLAS — worth it for
BLAS-heavy workloads, see the performance page) and `duckdb` output. Both accept identical
configs; only the compiled-in features differ, and `--version` is how you tell them apart.

## The 60-second mental model

A simulation is a set of **partitions**. Each partition advances a vector **state** every step,
computed from its **params** (named `[]float64` inputs) and, optionally, other partitions' states.
Partitions run concurrently and are wired together only through the config.

- **`params:`** — every value is a list of float64 (a scalar is a one-element list: `[0.5]`).
- **`init_state_values:`** — the state vector at t=0; its length is the state width.
- **`state_history_depth:`** — how many past steps to keep (≥1).
- **`seed:`** — per-partition RNG seed.

Output is one row per step: `<time> <partition> [<state values>]`.

## A partition's update: two ways

### (A) `expressions:` — bespoke maths (use this for custom models)

You write the per-step update as string expressions and define the params, so nothing is implicit.
This is the primary path for a model that isn't a textbook process.

```yaml
main:
  partitions:
  - name: growth                 # declares params + initial state
    params: {rate: [0.05], capacity: [100.0], noise: [0.1]}
    init_state_values: [10.0]
    state_history_depth: 1
    seed: 42
  expressions:                   # gives that partition's update as data
  - partition: growth
    fields:                      # names the state slots, in order (length = state width)
    - {name: x}
    bindings:                    # optional named intermediates, evaluated in order
    - {name: drift, expr: "rate * x * (1 - x / capacity)"}
    outputs:                     # one expression per field = the next state vector
    - "x + drift * dt + noise * x * shared(normal(0, 1)) * sqrt(dt)"
  simulation:
    output_condition: {type: every_step}
    output_function: {type: stdout}
    termination_condition: {type: number_of_steps, max_steps: 50}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
```

**Names an expression may use:** the partition's own **field names** (current committed value),
its **params keys**, `dt` (timestep), `t` (time), `step` (step number), any earlier **binding**,
and an **upstream alias** (see wiring below).

**Functions:** `sqrt pow exp log abs min max clamp(x,lo,hi) where(cond,a,b) floor sin cos erf erfc`,
`slice(v,i,n)`, `concat(a,b)`, `width(v)`, `lag(name,n)` (a value n steps back), plus `+ - * /`
and comparisons `< > <= >= ==`. Everything is elementwise over vectors with length-1 broadcasting,
including `where` — under a vector condition each branch must be the condition's width or a scalar,
and a scalar condition is lazy, so only the taken branch is evaluated (and only it draws).

**Not elementwise:** `each(n, i, expr)` builds a width-n value whose element `i` is `expr` with
the lane index bound — an index shift such as `each(5, i, where(i == 0, births, pop[i-1] * s))`,
and a per-lane `where` that is lazy, so a switched-off lane draws nothing. `scan(n, i, acc, init,
expr)` is the same loop with an accumulator threaded through it: lane `i` sees the previous lane's
value (`init` at lane 0) and the call is the *last* lane's value, so it is a fold, not a map. Use
it when a lane must see what earlier lanes did — allocation, running maxima, prefix sums:
`slice(scan(6, i, acc, 0, concat(acc, acc[i] + q[i])), 0, 6)` is an exclusive prefix sum, and
`each(6, i, sum(slice(q, 0, i)))` is the same thing more simply (a zero-width `slice` is empty,
and `sum` of it is 0) at O(n²).

**Random draws:** `normal(mean,sd)`, `poisson(rate)`, `uniform(lo,hi)`, `gamma(shape,scale)`,
`binomial(n,p)`. **Rule (the #1 mistake):** if a draw's parameters are all scalars its width is
ambiguous and the run panics — wrap it: `shared(normal(0,1))` for one sample, or
`iid(n, normal(0,1))` for n independent samples. A draw whose parameter is already a vector needs
no wrapper.

### (B) `iteration: {type: ...}` — a library process (catalogue at the end)

For a standard process, name it and put its parameters in `params:`. No expressions entry.

```yaml
main:
  partitions:
  - name: walk
    iteration: {type: wiener_process}     # params it reads go in params:
    params: {variances: [1.0, 4.0]}       # a 2-D Wiener process
    init_state_values: [0.0, 0.0]
    state_history_depth: 1
    seed: 7
  simulation: { ... as above ... }
```

## Coupling partitions (and the one rule that matters)

A partition reads another partition's state through one of two mechanisms, which differ in
**timing** — and getting the timing wrong is the only way to hang a simulation:

1. **`upstreams:` (in the expressions block)** — `alias[i]` reads the other partition's value
   from the **PREVIOUS step** (a one-step lag).
   ```yaml
   expressions:
   - partition: predator
     fields: [{name: y}]
     upstreams: {prey: prey_partition}      # prey[0] = prey's previous-step value
     outputs: ["y + (delta*prey[0]*y - gamma*y) * dt"]
   ```
2. **`params_from_upstream:` (in the partitions block)** — injects another partition's value into
   a params key, read as `key[i]`. This is a **WITHIN-step** read (you see the other partition's
   value as computed *this* step), so it imposes a computation order.
   ```yaml
   - name: consumer
     params_from_upstream: {other: {upstream: producer}}   # read as other[0] this step
   ```

**Deadlock rule.** `params_from_upstream` is within-step and **deadlocks** if two partitions each
depend on the other within the same step (neither can go first). Break any such cycle by making at
least one direction a lag-1 `upstreams` read. For most mutually-coupled models (predator–prey, etc.)
use lag-1 in both directions — it's the faithful choice for an explicit Euler step. If you do
deadlock, the run tells you exactly which partitions form the cycle.

## The simulation block (all data specs)

```yaml
  simulation:
    output_condition:      {type: every_step}            # or nil, every_n_steps{n}, only_given_partitions{partitions:[...]}
    output_function:       {type: stdout}                # or nil, json_log{path},
                                                         # arrow{path}, duckdb{path,table}
    termination_condition: {type: number_of_steps, max_steps: 100}   # or time_elapsed{max_time_elapsed}
    timestep_function:     {type: constant, stepsize: 1.0}           # or exponential_distribution{mean, seed}
    init_time_value:       0.0
```

## Run modes — `run:` (optional; default is one batch run)

```yaml
run:
  mode: ensemble           # run one member per seed, concurrently
  seeds: [11, 22, 33, 44]  # output rows are prefixed member=<i> seed=<s>
  # concurrency: 4         # optional; defaults to GOMAXPROCS
```

## Analysis & inference — `data:` + `macros:`

A macro expands one of the built-in analysis constructors into a *set* of partitions. `data:`
produces the dataset they analyse — a sub-simulation run for `steps`, or a file source — and each
macro runs against it. This whole block is data and runs in-process.

```yaml
# Generate a Normal data stream, then estimate its rolling mean and variance.
data:
  steps: 500
  timestep: 1.0
  partitions:
  - name: data_stream
    iteration: {type: data_generation, likelihood: {type: normal, allow_default_covariance_fallback: true}}
    params: {mean: [1.8, 5.0], covariance_matrix: [2.5, 0.0, 0.0, 9.0]}
    init_state_values: [1.3, 8.3]
    state_history_depth: 200          # must be >= the macros' window (they aggregate this much history)
    seed: 291
macros:
- type: vector_mean
  name: rolling_mean
  data: {partition_name: data_stream}
  kernel: {type: exponential}
  params: {exponential_weighting_timescale: [100.0]}   # the exponential kernel's timescale
  window: 100                       # how much history of the data source to aggregate
- type: vector_variance
  name: rolling_var
  mean: {partition_name: rolling_mean}   # covariance/variance reference the rolling mean
  data: {partition_name: data_stream}
  kernel: {type: exponential}
  params: {exponential_weighting_timescale: [100.0]}
  window: 100
```

A macro takes an optional **`params:`** map for the kernel (or iteration) it wraps — the
`exponential` kernel *requires* `exponential_weighting_timescale`; the `instantaneous` kernel (the
default when you omit `kernel:`) needs none. `vector_covariance` takes the same fields as
`vector_variance` (both need a `mean:` reference). Macro results are written to stdout
automatically — an analysis run needs no `simulation:` block. A later macro may reference an earlier
macro's output partition by name (they run in order).
### Reading and writing data (I/O)

`data.source` loads a dataset instead of running a sub-simulation; `output_function` writes a
run out. The same formats work in both directions, so a run can be written and read back:

| Source (`data.source:`) | Sink (`output_function:`) |
|---|---|
| `{csv: {path: x.csv, time_column: 0, state_columns: {series: [1,2]}}}` | `{type: stdout}` / `{type: json_log, path: run.log}` |
| `{json_log: {path: run.log}}` | `{type: arrow, path: run.arrow}` |
| `{arrow: {path: run.arrow}}` | `{type: duckdb, path: db.duckdb, table: results}` *(accel build)* |
| `{postgres: {user, password, dbname, table, partition_names, start_time, end_time}}` | `{type: postgres, table: results, user/password/dbname}` — or `{type: postgres, driver: pgx, dsn: "…", table: t}` for **any Postgres-wire database** |
| `{s3: {bucket, key, format: arrow\|csv\|json_log}}` | `{type: s3, bucket, key, format: arrow\|json_log}` |

**S3 is a transport, not a format** — give it the `format:` of the object and it reuses the
normal loader/sink, so anything above works over S3. Credentials come from the standard AWS
chain (env, shared config, IAM role) — never put them in the config. Optional `region:` and
`endpoint:` (set `endpoint` for MinIO / R2 / any S3-compatible store).

If you name a source this binary doesn't have, the error lists the ones it does.

Notable macros (all take a `data:` block): `vector_mean` / `vector_variance` / `vector_covariance`,
`grouped_aggregation`, `scalar_regression_stats`, `likelihood_comparison`, and
`posterior_estimation` (online Bayesian estimation of a simulation's parameters — its spec nests a
`comparison:` with a windowed embedded model). `evolution_strategy_optimisation` and `smc_inference`
run live (no `data:` needed; give them `steps:`).

Two decision-making macros also run live, and the distinction between them matters:

- **`mcts_planning`** — tree search over *your model*. The dynamics are already partitions, so an
  action is a params injection (`actions:` + `action_partition` + `action_param`), a transition is
  one step of the model, and the reward is just another partition named by `reward_partition`
  (write it as an `{type: expression}` partition). Nothing has to come from Go. Use this whenever
  the user wants "what should I do, given this simulation" — dispatch, dosing, intervention
  timing. See `cfg/example_planning_config.yaml`. It needs `horizon:` (lookahead), `steps:`
  (decisions to take) and `return_range: [min, max]` (UCB1 needs a bounded value scale — widen it
  rather than letting returns clamp). **When `horizon` exceeds `rollout_max_steps`, set
  `progress_partition:`** — a partition whose state[0] scores the position in [0,1] (a win
  probability, a normalised margin). Rollouts that run out of steps are scored by it instead of
  discarded; without one the search falls back to reward banked so far, and on a long horizon
  that is much weaker. **To plan under parameter uncertainty rather than a point estimate**, add
  `parameters:` with `samples_from: {partition_name: <recorded draws>}` (or inline `samples:`) plus
  `targets:` routing each draw into the model. `posterior_estimation`'s sampler partition already
  emits posterior draws, so `samples_from: {partition_name: <sampler>, burn_in: <converged from>}`
  chains inference straight into planning — the search then averages over the posterior, which
  changes the recommendation whenever the payoff is nonlinear in the uncertain parameter. Adding
  `belief: {observation_partition: <what is seen>, variance: <noise>}` under `parameters:` lets the
  planner *learn* within an episode, so it will pay for an informative action; it costs a model
  step per sample per transition, so keep the sample set to a handful. Use `weights:` /
  `weights_from:` when the samples are not themselves posterior draws (SMC particles carry their
  posterior in their weights) — that sets the starting belief and steers which samples get drawn. Every model partition must have `state_history_depth: 1`,
  and the model must be Markov in its own rows: the planner restarts it each transition, so
  nothing may rely on the step counter — carry a phase/counter in the state instead.
  On stochastic models it uses **chance nodes** by default, averaging each action's value over
  sampled successors. `pinned_noise: true` opts out (faster, and free when the model is
  deterministic) — but only do that knowingly: a pinned search exploits the noise draw it is
  going to receive, and its over-promise *grows* with `sims_per_decision`. Either way, quote a
  planned return as one scenario's outcome, not a forecast; vary `scenario_seed` and aggregate.
- **`mcts_self_play`** — search over *decision rules written in Go*, and the one macro you cannot
  write from config alone: its `env:` names an `agents.Environment` (a game's legal moves and win
  condition), registered by a downstream module via `api.RegisterEnvironment`, and the binary must
  link that module. The engine ships a single `tictactoe` fixture, so `{type: mcts_self_play,
  name: ttt, steps: 10, sims_per_ply: 120, env: {type: tictactoe}}` runs out of the box and
  nothing else will unless you built the binary.

For optimising a *policy parameterisation* globally rather than an action sequence from the
current state, `evolution_strategy_optimisation` is the right tool instead of either.

## Worked recipes (start here for the inference/optimisation macros)

The three learning macros have levers that decide whether they *converge* or merely *run* — so
don't write one from the catalogue alone. Copy the matching recipe in `recipes/` and adapt it.
Each is a complete, in-process config, verified by an engine test that pins it to a known answer.
**The recipes are the schema** for these macros' sub-fields (`sorting`, `sampler.distribution`,
`comparison.window`, `window_data_history_depth`, `memory_depth`, …) — the catalogue below lists
macro *names* only, so change the numbers and the objective/model, but keep the field shapes.

- **`recipes/evolution_strategy_optimisation.yaml`** — maximise a reward (here the negative
  squared distance from a target) by adapting a sampling mean + covariance; converges to the
  target `[3, -2]`. Write your objective as the `reward` partition's `{type: expression}`.
  **Levers:** keep the covariance `learning_rate` *slow* (≈0.1) — a fast rate collapses the search
  width before the mean arrives and freezes it short; use `discount_factor: 0.0` for a static
  objective (the sample is fixed across the window, so a discount only rescales a constant reward).
- **`recipes/posterior_estimation.yaml`** — online Bayesian recovery of a data stream's parameters;
  recovers the mean `[1.8, 5.0]` from an off-truth prior `[0, 0]`. **The one thing you must not
  omit:** the `comparison.model` has to read the sampler via `params_from_upstream`
  (`mean: {upstream: <sampler_name>}`) — the posterior is a loglike-weighted average of the
  *sampled* params, so if the loglike doesn't depend on the sample the mean just drifts. (The macro
  now panics if you forget, naming the fix.) **The second thing:** `window_data_history_depth` must
  *equal* `comparison.window.depth` — too large only warns, and the window then scores the
  zero-filled tail of the replay buffer instead of your data, freezing the posterior at its prior.
  **Levers:** proposal covariance wide enough to explore prior→truth (diag ≈9); `past_discount`
  near 1 (0.999) so evidence accumulates instead of being forgotten; enough `steps` to concentrate.
- **`recipes/smc_inference.yaml`** — particle-filter inference; write the model once under
  `model.partitions` and it is run once per particle (each with its own random streams), routing
  each particle's parameters in via `param_forwarding`. Recovers the observed stream's mean.
  **Levers:** `num_particles` (more = tighter posterior), `num_rounds`, and the `priors` ranges.
- **`recipes/scalar_regression_stats.yaml`** — closed-form OLS of scalar `y` on scalar `x`;
  recovers slope 2.5 and intercept 1.0 of a noisy line. No convergence levers (it's closed-form —
  more `steps` just tighten the estimate). **The gotcha is the output layout:** with `intercept:
  true`, `mode: cumulative` the state is width 9 `[n, Sx, Sy, Sxx, Sxy, Syy, alpha, beta, sigma2]`,
  so index 6 is the intercept, 7 the slope, 8 the residual variance — read the slot, don't hunt
  for the number.

## Running and debugging

```
stochadex --config model.yaml
```

If every iteration and simulation component is a `{type: ...}` data spec (no Go), it runs
in-process with no toolchain. Errors are located and actionable — an unknown type names the field,
a mistyped param key is rejected, and a within-step cycle names the partitions to break. Read the
error; it tells you what to fix.

## Gotchas

- **Scalar draws** need `shared(...)` or `iid(n, ...)` (see the draw rule above).
- **Mutual coupling** must break the within-step cycle with a lag-1 `upstreams` read.
- **A partition named `y`, `n`, `on`, `off`** is fine — names are read as strings.
- **Params are lists.** A scalar is `[0.5]`, not `0.5`.
- Prefer `expressions:` for anything custom; reach for a `{type: ...}` iteration only for a
  standard process you can name in the catalogue.

## Catalogue (valid `{type: ...}` names)

**Iterations — standard processes:** `wiener_process` (params: `variances`),
`ornstein_uhlenbeck` (`mus`,`sigmas`,`thetas`), `geometric_brownian_motion` (`variances`),
`drift_diffusion` (`drift_coefficients`,`diffusion_coefficients`), `poisson_process` (`rates`),
`cox_process` (`rates`), `bernoulli_process` (`state_value_observation_probs`),
`compound_poisson_process` (`rates`,`gamma_alphas`,`gamma_betas`; field `jump_dist: {type: gamma_jump}`),
`drift_jump_diffusion`, `hawkes_process`, `binomial_observation_process`,
`categorical_state_transition`, `cumulative_time`, `gradient_descent`,
`ornstein_uhlenbeck_exact_gaussian`,
`hawkes_process_intensity` (params: `background_rates`; field `kernel: {...}`; wire the counting
partition it is excited by with `params_as_partitions: {hawkes_partition_index: [<name>]}`).

**Iterations — utility / composable:** `constant_values`, `copy_values`, `param_values`,
`cumulative` (field `iteration: {...}`), `discounted_cumulative` (`iteration`),
`data_generation` (`likelihood: {...}`), `data_comparison` (`likelihood`),
`values_function_vector_mean` / `values_function_vector_covariance` (`function: <name>`, `kernel: {...}`),
`values_grouped_aggregation` (`aggregation: <name>`, `kernel`),
`values_function` (either `function: <name>` or `transform: <name>` + `reduce: <name>`),
`values_changing_events` (fields `event_iteration: {...}` and `iteration_by_event:` — a *list* of
`{event: <number>, iteration: {...}}` pairs; falls back to `default_values` params, else the
previous state, when no event matches),
`values_weighted_resampling` (params: `log_weight_partitions`, `data_values_partitions`,
optional `log_weight_indices`, `past_discounting_factor` — wire the partition lists by name with
`params_as_partitions`),
`posterior_mean` (`transform: mean|variance`), `posterior_covariance`, `posterior_log_normalisation`,
`ensemble_kalman_filter` (stochastic EnKF data assimilation; params: `ensemble_size`, `state_dimension`,
`observation_noise_variance`, optional `observation_indices` and `inflation`; wire `forecast_ensemble`
and `latest_data_values` with `params_from_upstream`; state is the analysis ensemble flattened
member-major — see `cfg/example_enkf_config.yaml`).

**Kernels:** `exponential` `periodic` `gaussian_state` `t_distribution_state` `binned`
`instantaneous` `constant` `product` (fields `kernel_a`,`kernel_b`).
**Likelihoods:** `normal` (field `allow_default_covariance_fallback: bool`) `t_distribution`
`wishart` `beta` `poisson` `gamma` `negative_binomial`.
**Priors:** `uniform` (`lo`,`hi`) `truncated_normal` (`mu`,`sigma`,`lo`,`hi`) `half_normal` (`sigma`)
`log_normal` (`mu`,`sigma`).
**Value functions** (`function:` on the vector mean/covariance iterations): `data_values`
`data_values_variance` `other_values` `unit_value` `past_discounted_data_values`
`past_discounted_other_values`.
**Whole value functions** (`function:` on `values_function`): `params_event` (reads the `event`
params) `partition_event` (reads `event_partition_index` / `event_state_value_index`).
**Aggregations:** `count` `sum` `mean` `max` `min`.
**Macros:** `vector_mean` `vector_variance` `vector_covariance` `grouped_aggregation`
`scalar_regression_stats` `likelihood_comparison` `posterior_estimation`
`likelihood_mean_function_fit` `evolution_strategy_optimisation` `smc_inference`
`mcts_self_play` (needs a registered `env:`) `mcts_planning` (tree search over your own model).
**Environments** (`mcts_self_play`'s `env:`): `tictactoe` only, unless the binary links a module
that called `api.RegisterEnvironment`.
**Simulation components:** output_condition `nil|every_step|every_n_steps|only_given_partitions`;
output_function `nil|stdout|json_log`; termination `number_of_steps|time_elapsed`;
timestep `constant|exponential_distribution|from_history`.
