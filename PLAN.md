# Plan: config as inputs → one runtime → outputs

Status: **accepted** (2026-10-04).
- **Where this lives:** the long-lived working branch `claude/config-runtime-plan`. It is
  deliberately **never merged to `main`**; implementation PRs branch from `main` and
  link here. Update this file on this branch as items land.
- **Progress:**
  - **Merged:**
    - 0.3 `RunToStorage` (#95);
    - sinks open when a run starts (#96, rule 12);
    - 0.6 websocket push output (#97);
    - O.3 structured exit codes (#99);
    - fix: the CSV loader rejects non-numeric time values (#100, found while doing O.3);
    - 0.4a `outputs:` views (#101).
    - 0.4b, macro results through views (#102). It also rejects `outputs:` with an
      ensemble, which 0.4a had left silently ignored.
    - 0.4c `RunWith` / `CaptureView` / `WithConfigOutputs` (#103). Suppression now
      covers nested sinks, fixing the #96 finding for `RunToStorage`.
    - 0.4d, per-member ensemble views with `{member}` / `{seed}` (#104).
      **Phase 0 is complete.**
    - Phase 1 items 1.1 `inputs:` and 1.2 `from_input` for the `main:` path (#105).
      Acceptance test: solar-fleet driven from a CSV input reproduces exactly.
    - The rest of 1.1 (#106): macros read `inputs:`, and `data:` is shorthand for a
      single input. All 9 shipped `data:` configs give byte-identical results.
    - 1.3 `run: {mode: serve}` (#107). Each connection gets its own fresh run;
      `outputs:` views are written per connection with `{connection}`; `--socket` is a
      deprecated alias.
  - **Decided (2026-10-07): one place in, one place out (rule 14).** A review of the
    work so far against this rule found that outputs are still declared in five places,
    and that #107's served stream is one of them: it lives in `run:` and takes its
    filter from the shorthand `output_condition`. The new **Phase 1b** (IO.1–IO.6)
    closes these gaps. It also settles the open follow-up about the shorthand
    `output_function` going unwritten under ensemble and serve. The answer is not to
    reject it but to desugar it, with a `stdout` sink that knows which run instance it
    belongs to (IO.2).
    - IO.1, the served stream as an `outputs:` view `{type: connection}` (#108).
  - **Merged:** IO.2, every output an `outputs:` view in every mode (#109).
    - The shorthand pair is one view named `output`; no output declared means a
      default stdout view, which fixes a CLI crash on batch configs with no output.
    - `stdout` is prefixed per member or connection.
    - **Acceptance, restated:** "byte-identical stdout" was not achievable, and
      shouldn't be the bar. Batch configs with several partitions already print in a
      different order on every run, because partitions output concurrently. Macros
      now print in time order (as Phase 2 will anyway), and ensemble members
      interleave. All 20 shipped configs print the **same lines** as the previous
      binary.
  - **Merged:** 1.4, `PartitionCoordinator.InjectParams` (#110).
    - Guards: unknown partition, undeclared key, wrong width, and an upstream-fed key
      are errors.
    - Writes in place (decided 2026-10-07): 72 ns and one allocation, the copy of
      the values. A partition's params map is the `Settings`' own map, so, like
      `params_from_upstream`, an injection lands in the `Settings`. Copying the
      map per call to avoid that cost about as much as an inline step, and
      guarded a case nothing in the engine hits; build from the config again to
      start fresh.
  - **Decided (2026-10-07): engine performance is protected.** The plan's changes
    must not slow the step loop; measure every per-step change against v0.19.0
    with interleaved `benchstat` runs. An audit found the output refactor
    (#101, #109) had cost 2–3 ns per output, up to 21% on small inline runs.
    **Merged (#111):** that cost is removed (level with v0.19.0), and
    `json_log` is buffered, which makes log-writing runs 70–85% faster. A
    `Stepper`'s `Close` now finalizes output. The PR adds committed benchmarks
    (`BenchmarkConfigRun*`) and allocation guards. It also fixes per-run
    allocations: since #95, a run generated its configs twice, once to check
    and once to run, which cost 76 allocations for 16 partitions. A config run
    now allocates exactly what v0.19.0 did.
  - **Merged:** 1.5a and 1.6, stream inputs and `params_from_input` (#112).
    - **Q2 decided:** `hold_last` only, for now.
    - **Backpressure:** one "latest wins" slot per stream partition, so there's no
      queue to bound.
    - **Performance:** `params_from_input` from stored data is 60% faster than a
      `from_input` partition read through `params_from_upstream`, with 84% fewer
      allocations, via a new `simulator.ParamsInjector` that copies in place.
  - **Merged:** 1.5b, two-way serving (#113).
  - **Clarified (2026-10-08):** rules 15 (embedded runs are model, not inputs) and 16
    (`main:` and `macros:` compose in one runtime, with unambiguous names).
  - **Merged:** #114, embedded runs.
  - **In review:** v0.20.0 release (#115), then dexetera onto the engine's primitives
    (umbralcalc/dexetera#1, a draft until the tag exists).
    - The dexetera change uses `ActionState`, `ParamsInjector` and inline stepping, which
      runs 12–15× faster per step in WebAssembly.
    - Not serve: dexetera runs in the browser, and all 5 downstream dashboards use its
      in-browser driver.
    - energy-balancer needs a one-line action-width fix when it upgrades.
  - **Previously in review:** #114, embedded runs.
    - Parses the host's forwarding keys once: 32% faster per outer step, 67% less
      memory.
    - Checks inner partition names at load.
    - Tests `params_from_input` reaching an inner run through its host (rule 15).
    - Still open: building the inner run's coordinator every outer step (~39
      allocations per step).
    - `stream: {connection: {}}` reads the served client's own messages.
    - `decode: protobuf_action_state` reads dexetera-compatible `ActionState` (a new
      `cmd/messages/action_state.proto`).
    - Serve runs `params_from_input` and stream inputs per connection; batch runs and
      served connections share one stepping loop.
  - **Next:** a choice. 1.7 (keyboard as a stream transport, which needs a
    `RegisterStream` hook), or the dexetera migration onto two-way serve (it can drop
    `simio`'s own stepping), or the parallel tracks: IO.4, IO.5, O.1, O.4. IO.3 still
    waits on Q7. IO.3 needs Q7 decided
    first; IO.4 and IO.5 can go any time. O.1 and O.4 can run alongside.

## 0. Summary

A stochadex YAML config is general and powerful, but it is hard to tell what runtime a
given file actually corresponds to. One file can trigger anywhere from one to N+k
coordinator runs. Inputs, outputs and run modes are each wired into only one of the two
config paths (`main:` vs `macros:`). Until #92, several keys were also silently ignored
on the other path; they are now load errors.

The target is a single mental model that every config obeys:

```
inputs (fixed or live)  →  ONE runtime (one coordinator, one clock)  →  outputs (sinks)
                           × run: mode (batch | ensemble | serve)
```

- **Inputs** are named and read-only. A fixed input exists before the run is built (a
  file, a database, or a labelled pre-pass simulation). A live input arrives once per
  step and is injected at the step boundary.
- **The runtime** is exactly one coordinator with exactly one clock, written in the
  **core language**: partitions, `{type: ...}` iterations, `expressions:`, wiring
  (`params_from_upstream` / `params_as_partitions`), `embedded:` and `simulation:`.
  That core language is the **only** domain language.
- **Macros are shortcuts, not semantics.** A macro is a pure config → config rewrite: it
  expands into core-language partitions (and, if needed, `simulation:` fields), and
  nothing else. Domain concepts such as "posterior", "SMC" or "planner" exist only as
  macro *names*. The core never knows about them. `stochadex expand` prints the expanded
  config, and the expanded config must run identically (rule 9).
- **Outputs are views**: each is a (name, condition, function) built on the engine's
  existing `OutputFunction` / `OutputCondition` abstraction, available to every
  runtime and composable as a tee. A programmatic caller's result (`RunResult`) is
  just the in-memory views it attached, not a second output mechanism (§2.5).
- **`run:`** decides how many runtimes exist and how they are paced: batch, an ensemble
  over seeds, or one runtime per websocket connection.
- **Chaining runtimes happens outside the engine** (DBOS, Make, scripts, downstream Go),
  through files. The engine does not grow a workflow language. This is the same
  boundary as Invariant A: the engine owns the forward and inferential model, and
  downstream owns calibration loops and orchestration.

Delivery is in three phases. Each phase is safe on its own, and no current capability
is lost. Phase 0 is a pure tidy-up. Phase 1 adds inputs and live I/O. Phase 2 folds
macros into the single runtime behind an exact-equality oracle. A parallel **Track O**
makes each config a well-behaved cloud-orchestrator task: per-invocation overrides, an
I/O manifest, retry-aware exit codes, all-or-nothing outputs, provenance hashes for
caching, and ensembles split across machines.

### 0.1 Why: what this achieves, and what is proven vs assumed

The end state **formalises a simulation DSL embedded in YAML**. It is consistent with
the 2026-07 decision not to build a bespoke language: YAML stays the syntax, and this
plan fixes the *semantics*.
- **A small core:** inputs, partitions, the `{type: ...}` iteration catalogue, the
  `params_*` wiring, recursive embedded runs, `simulation:`, `run:` and outputs.
- **A standard library of macros:** pure config → config rewrites over that core.

| Claim | Status | Evidence / caveat |
|---|---|---|
| A proper DSL, each construct with one meaning | **Supported** | 11/11 macro and structural twins exact (§4.2); macros become library, not semantics. Still to do: a written core semantics reference (execution order, within-step vs lag-1, nesting, clock) and a machine-readable schema. The meaning of each `{type: ...}` is still defined by its Go implementation and its docs. |
| New behaviours by composition, without new macros | **Supported in principle, shown structurally** | Recursive nesting, replicated runs, and MCTS over any data-defined model compose freely: an ES over a posterior, a planner inside particles, a posterior per scenario. Composition also allows expensive or meaningless combinations, so validation (deadlock, widths, clock conflicts) and cost visibility matter more. |
| More integrable and flexible | **Supported** | Declared inputs and outputs, one runtime per config, a lossless file chain (§4.2 finding 15), Track O, and environments as registered components. |
| No performance cost | **Plausible, not yet measured** | Likely neutral or faster: one coordinator replaces a data run plus one replay pass per macro, and no storage replay copies. Possible costs: three environment instances instead of one for MCTS; one larger step loop instead of several small ones under spawn-per-step execution; and composition makes multiplicative nesting cost easy to write. **Gate: benchmark (item 2.12) before retiring the old path.** |

---

## 1. What we learned

### 1.1 Every runtime is already one coordinator run

| Config shape | Coordinator runs it triggers | Clock comes from |
|---|---|---|
| `main:`, `run: batch` | 1 | `main.simulation` |
| `main:`, `run: ensemble` | N, one per seed | `main.simulation` |
| `data:` sub-simulation | +1 run up front | `data.steps` / `data.timestep` |
| each against-storage macro | +1: `AddPartitionsToStateTimeStorage` replays every stored column as a `FromStorageIteration` and runs the new partitions beside them (`pkg/analysis/partitions.go:40`) | the storage's timestamps (`FromStorageTimestepFunction`) |
| each live macro | +1, and the result **replaces** the storage (`pkg/api/macros.go:247`) | the macro's own `steps` / `timestep` |

The confusion is not really "macros vs main". It is that one file triggers a variable,
invisible number of runtimes.

### 1.2 Inputs, outputs and modes are split across the two paths

| | `main:` path | `macros:` path |
|---|---|---|
| External inputs (csv / json_log / postgres / arrow / s3) | **inline only**: since #86 `{type: from_storage, data: [[...]]}` replays a series written into the config itself; there is still no way to bind a *file or database* source to a main partition | `data.source` |
| Output sinks (stdout / json_log / postgres / arrow / duckdb / s3) | `output_function` | **stdout only** (`printStorage`, `pkg/api/run.go`) |
| `run: ensemble` | yes | **not supported**: rejected at load since #92 (`validateMacroContext`, `macros.go:174`); was silently ignored |
| `embedded:`, `main.*` | yes | **not supported**: rejected at load since #92; was silently ignored |
| `data:` | ignored → rejected at load since #92 (`validateMainContext`, `run.go:219`) | read |
| Deadlock pre-flight, `cmd/stochadex-graph` | yes | only on the `data:` sub-simulation |
| Websocket serving | yes (batch only, separate `-s` socket file; one fresh model per connection since #91) | no |
| Go entry point returning storage | none (cryptobook swaps in its own `StateTimeStorageOutputFunction`) | `RunMacros` |
| Go entry point for ensembles | `RunEnsembleToStorage` | n/a |

The silently ignored keys were the same class of problem `dead_keys.go` was written to
stop: keys that look load-bearing and do nothing. #92 turned them into load errors. The
underlying asymmetry remains: those features *exist* on only one path. Phases 1 and 2
remove it.

### 1.3 The 12 macros fall into four groups

Classified by what they read from storage at build time and whether they need their own
clock:

| Group | Macros | Reads at build | Could run inside a host simulation? | Clock |
|---|---|---|---|---|
| **A. Causal expanders** | `vector_mean`, `vector_variance`, `vector_covariance`, `scalar_regression_stats`, `likelihood_comparison`, `posterior_estimation`, `likelihood_mean_function_fit` | shape (`DataRef.GetValueIndices`) and row 0 (`GetTimeIndexFromStorage(storage, 0)`) only | **Yes.** Per step they read current values via `params_from_upstream` and past values via history (`params_as_partitions`, or an inner window replayed through `FromHistoryIteration`). The `windows` map just says "give upstream partition X a history depth of N". | host's |
| **B. Expanders whose shape depends on the data** | `grouped_aggregation` | **whole series**: scans every row to discover groups (`pkg/analysis/grouping.go:77-100`) | Per step yes, but the group set must be known up front, so it needs a **fixed input** | host's |
| **C. Self-contained, own clock** | `evolution_strategy_optimisation`, `mcts_self_play`, `mcts_planning` (without `samples_from` / `weights_from`) | nothing | They *are* a whole simulation | own `steps` / `timestep` |
| **D. Own clock, consume a whole dataset** | `smc_inference` (every particle replays the full observed series in an embedded sim, `pkg/macros/smc.go:132-147`), `mcts_planning` with `samples_from` (every row after burn-in) / `weights_from` (final row) | **full contents** | No: needs a **fixed input** | own `steps` (rounds) |

So we arrive at two definitions:

- **Input**: a fixed dataset that must exist before the runtime is built. Groups B and D
  require one. Group A can use one, or can read live partitions of the same run.
- **Runtime**: one coordinator with one clock. The clock comes from an explicit
  `simulation:` block, from an input's timestamps, or from a single group C/D macro.

Other facts that matter:

- Two in-config chains are in real use:
  - **Against-storage → against-storage**: `cfg/example_macro_config.yaml`, where
    `rolling_var` reads `rolling_mean` through `params_from_upstream["mean"]` with a
    150-step window.
  - **Against-storage → live (calibrate, then plan)**: `posterior_estimation` followed
    by `mcts_planning` with `samples_from: {partition_name: post_sampler}`, in one file
    (`test/mcts_planning_test.go`, `pkg/api/macros_planning_test.go`). The live macro
    reads the earlier macro's output, then **replaces** the storage, so the posterior
    columns are absent from the final output. That is intended and documented in those
    tests.

  Every other config and skill recipe uses one macro. (An early survey claimed only the
  first chain existed. Making live macros run alone broke the second chain's tests, and
  the change was reverted.)
- Each live spec's `resolve` stub says it "must not be combined with against-storage
  macros" (`macros_optimisation.go:63` and similar). That code is **unreachable**: the
  type switch at `macros.go:234` runs first. The wording also contradicts the supported
  calibrate → plan chain. It is harmless but misleading.

### 1.4 How the engine is actually used downstream

Fourteen repos were surveyed.

- **About 12 of 14 drive the engine from Go**, not YAML. They use `ConfigGenerator` or
  `LoadSettingsFromYaml`, then `NewPartitionCoordinator` / `RunWithHarnesses`, then
  `StateTimeStorageOutputFunction`.
- **The most common hand-written Go pattern is feeding real data into a forward run via
  `general.FromStorageIteration`**: homark, floodrisk, trywizard, anglersim,
  bathing-water-forecaster and AMR all do it. This is exactly what `main:` cannot do in
  YAML today.
- **YAML in practice** is `main:` with stdout or json_log, plus `data:` + `macros:` for
  inference (`posterior_estimation`, `smc_inference`, `likelihood_comparison`).
  - cryptobook uses about 24 configs through `pkg/cfgrun`.
  - AMR uses `cfg/amr_inference.yaml`, a csv source with `posterior_estimation`.
  - card-game-studio calls `api.RunMacros` and registers `cardgame_rulesvm` via
    `RegisterEnvironment`.
- **Chaining across runtimes is almost always external, through files** (the
  in-config calibrate → plan chain in §1.3 is the exception). Examples:
  - floodrisk: `ingest` → CSV → `calibrate`.
  - homark: `fetchspine` → CSV → `calibratespine` → `posteriors/*.json` →
    `forwardspine` / `policyscenario`.
  - cryptobook: `record-feed` (exchange websocket) → json_log → `lob_calibrate_from_log.yaml`,
    with the path substituted per window.
  - business-survival and anglersim follow the same pattern with CSV/JSON.

- Nobody downstream uses `embedded:`, `run:`, socket configs or postgres from YAML.
- cryptobook's `pkg/cfgrun/cfgrun.go` (around line 264) has a stale comment saying no
  storage-returning ensemble exists. `api.RunEnsembleToStorage` now exists.

### 1.5 Live I/O today

| Mechanism | Where | Shape |
|---|---|---|
| `StepAndServeWebsocket` / `NewWebsocketHandler` (`pkg/api/run.go:27`, `:47`) | engine; socket file passed with `-s` | An HTTP server on a private mux. Each connecting client gets its own freshly built run (since #91), with `WebsocketOutputFunction` streaming every step, paced by a sleep, and stopped when the client disconnects. Origins are restricted (same-origin, loopback, `allowed_origins`). Batch mode only. |
| `UserInputIteration` (`pkg/keyboard/user_input.go:93`) | engine | A partition that **blocks inside `Iterate`** on a keystroke channel with a timeout. |
| `ApplyActionState` (dexetera `pkg/simio/dispatch.go`) | downstream | Between steps, the driver writes incoming protobuf actions into named partitions' `action_state_values` params, then calls `coordinator.Step`. |
| `cmd/record-feed` (cryptobook) | downstream | Exchange websocket → json_log, later replayed as `data.source.json_log`. |

The keyboard pattern breaks the two essential rules in CLAUDE.md. The harness runs a
simulation twice and compares the runs, and `ReentrantSimulation` re-evaluates a model
by reseeding it. A partition whose output depends on socket or wall-clock timing
satisfies neither. dexetera's step-boundary injection is the right pattern, and it is
the same idea as `mcts_planning`'s "an action is a params injection".

### 1.6 Bugs found along the way — all fixed and merged

| # | Bug | Fix | PR |
|---|---|---|---|
| 1 | **Concurrent websocket clients shared one model.** `StepAndServeWebsocket` reused one generator, and `GenerateConfigs` hands out the same iteration instances each time (`pkg/simulator/configs.go:321`). Reproduced: 14 data races and streams of 164 / 0 / 82 messages across three clients. Every handler also mutated the shared generator, which was registered on the global `http.DefaultServeMux`. | A fresh build per connection, re-loading the source file. A private `ServeMux`. Stepping stops on client disconnect. `NewWebsocketHandler` is exported. | umbralcalc/stochadex#91 |
| 5 | **The websocket server accepted any origin.** | `allowed_origins` in the socket file. The default admits non-browser clients, same-origin pages and loopback origins on any port; `"*"` restores the old behaviour. | umbralcalc/stochadex#91 |
| 4 | **Keys silently ignored**: `main.expressions`, `main.simulation`, `embedded:` and `run:` (non-batch) alongside `macros:`; `data:` without `macros:`. | `validateMacroContext` / `validateMainContext`: each is a load error naming the key. | umbralcalc/stochadex#92 |
| 2 | **Covariance init indexing.** `NewVectorCovariancePartition` wrote `[i+j]`, not `[i*num+j]`. This was user-visible through the YAML macro's `default_value`, because burn-in emits the initial state. The old test missed it because its default was 0. | Row-major index. | umbralcalc/stochadex#93 |
| 3 | ~~A live macro after other macros silently drops their output.~~ | **Not a bug**: this is the supported calibrate → plan chain (§1.3). | — |

Every fix has tests that compare against an independent reference (an offline run, or
an independently built matrix), go through the public path (a YAML file into the public
API), and were mutation-checked (each test fails with its fix reverted).

---

## 2. Target model

### 2.1 Definitions

| Term | Meaning | Lives in |
|---|---|---|
| **Fixed input** | A read-only `StateTimeStorage` that exists before the runtime is built: a file, a database, or a labelled pre-pass simulation | `inputs: {name: {source: ...}}` or `{simulation: ...}` |
| **Live input** | A per-step stream from outside, injected into partition params **between** steps; optionally recorded | `inputs: {name: {stream: ...}}` |
| **Runtime** | One coordinator, one clock, built from partitions, expressions, embedded runs and macro expansions | `main:` (+ `macros:`) |
| **Output view** | A (name, condition, function): a sink plus the filter selecting what it receives; several views form a tee; nested views carry scope | `outputs:` only. The `simulation.output_*` pair is shorthand that becomes one `outputs:` entry at load (IO.2). Caller-attached in-memory views for `RunResult` are added by the Go API, not the config (§2.5) |
| **Run mode** | How many runtimes exist and how they are paced | `run: {mode: batch \| ensemble \| serve}` |

### 2.2 Rules

1. **One runtime per config.** Anything multi-stage is orchestrated outside the engine,
   through files.
2. **One clock per runtime.** It comes from the `simulation:` block. That is either
   written by hand (a constant timestep, or `timestep_function: {type: from_input, input: x}`)
   or *emitted* by a macro's expansion (ES, SMC and MCTS emit `simulation:` fields). An
   emitted field that conflicts with one already present is an ordinary
   duplicate-definition error. Macros have no clock privilege of their own.
3. **Every macro is a config → config rewrite.** It emits core-language partitions,
   plus `state_history_depth` raises on the partitions it reads, into the one runtime.
   Chained macros become ordinary `params_from_upstream` edges, covered by the deadlock
   pre-flight and the graph tool. A macro's output is plain config: `{type: ...}`
   iterations and data values only, never live Go objects.
4. **Inputs may be computed, but they are still inputs.** A `{simulation: ...}` input is
   a labelled pre-pass. This keeps self-contained demos to one file and keeps seeded
   draws identical.
5. **Expansion never reads data.** Today `grouped_aggregation` scans the dataset for its
   groups, SMC copies the observed series into each particle's embedded run, and
   `mcts_planning` bakes posterior samples into params, all at build time. That makes
   their output depend on data values, not just on the config, so they are not pure
   shortcuts. Instead:
   - `grouped_aggregation` takes its groups declared in config. Discovering groups is a
     separate analysis whose output is an input.
   - SMC and planning emit partitions that read their data **at run time**, through
     `from_input` partitions bound to a named input, including inside embedded runs.

   An input may itself be a whole run (`inputs: {post: {run: <nested config>}}`). That
   is how the calibrate → plan chain (§1.3) stays a single file.
6. **Live I/O happens only at the step boundary.** Transports live in the driver loop
   that wraps the coordinator, never inside `Iterate`. Partitions stay pure, so the
   harness and reentrancy guarantees hold for everything except the live session
   itself.
7. **Recording turns live input into fixed input.** A recorded live run replays exactly
   offline from the record, using the same binding.
8. **Transports and sources are registries, not engine code** (`RegisterDataSource`,
   `RegisterComponent`, and a new `RegisterStream`). The engine's dependencies stay lean.
9. **Every macro has a core-language twin.** `stochadex expand -c cfg.yaml` prints the
   fully expanded config (no `macros:` key), and that config must produce
   **byte-identical** output to the macro form. This is the same test as the
   domain-models catalogue's declarative twin, applied to macros:
   - If a macro's expansion can be written in the core language, the macro is
     convenience only.
   - If it can't, because an emitted iteration has no `{type: ...}` form, the core
     language has a real gap. Close it by adding the spec, never by giving the macro
     special powers.
10. **Specs added for a macro must be generic.** When a macro's expansion needs a new
    `{type: ...}` spec, that spec must be named and documented in generic simulator
    terms and be useful outside that macro. "Sample rows from an input" passes;
    "posterior sampler" does not. This keeps the core vocabulary from accreting macro
    concepts.
11. **Expanded partitions carry their origin.** Every partition a macro emits records
    which macro produced it (a `from_macro: rolling_var` annotation, surfaced as a
    comment by `stochadex expand`). Load, validation and runtime errors report it, so a
    user is pointed at the macro they wrote, not a generated partition they didn't.
12. **Outputs are views, and there is one output mechanism.** Every output (config
    sinks, websocket streams, a caller's in-memory result, an ensemble member's log, a
    nested run's records) is an `OutputFunction` gated by an `OutputCondition`. No
    path may bypass it with a parallel capture mechanism. Views open their resources in
    `Configure` and commit in `Finalize`, never at load. A nested view's records carry
    their scope: path, outer step and time (§2.5).
13. **Named Go components remain data.** A config may *name* a Go component that a
    downstream repo registered (an `mcts_self_play` environment, an `onnx_inference`
    iteration). That is still core language. A macro may expand into such names, but it
    may not carry decision rules or bespoke maths that the core cannot express.

14. **One place in, one place out.** `inputs:` is the only place external data enters a
    run. `outputs:` is the only place results leave it. `run:` decides how many runtimes
    there are (one, one per seed, one per connection). Everything else (partitions,
    params, expressions, wiring, `embedded:`) describes the model.
    - **Shorthand is allowed only if it becomes these blocks at load.** Examples:
      `data:`, the `simulation.output_*` pair, and the default "print to stdout" when
      nothing is declared. `stochadex expand` shows the result.
    - **Why:** a config's I/O contract can then be read from two blocks. That is what
      makes a config easy to reason about, and what makes configs chainable by an
      orchestrator. Track O's `inspect --io` manifest (O.2) and provenance hashes (O.5)
      need a *complete* list of reads and writes, and they get one only if nothing
      reads or writes from anywhere else.
    - **Params are model, not inputs:** a literal in `params:` is part of the model's
      definition. Inputs are external data read when a run starts.
    - **A bidirectional transport appears in both blocks.** For example, a served
      connection is an `outputs:` view, and its live actions (1.5) are an `inputs:`
      stream that refers to the same connection. The rule is about where things are
      declared, not about each transport doing only one job.

15. **Embedded runs are model, not inputs** (clarified 2026-10-08). An input exists
    before the runtime starts: read or computed once, read-only, and independent of the
    run's state. That covers `{source:}`, a `{simulation:}` pre-pass, and Phase 2's
    nested-run input `{run:}`. An `embedded:` run is re-run inside a partition every
    outer step, so it can depend on the run's state, and it stays under `embedded:`.
    - **How it gets params:** its host partition forwards every params key of the form
      `<inner_partition>/<param>` into the inner run at each outer step. So
      `params_from_upstream`, `params_from_input` and literal params all reach an inner
      run through its host.
    - **Lag-1 state reads:** use the `/initial_state_from_partition_history` and
      `/update_from_partition_history` suffixes.
    - **What the plan changes:** only the edges. Its outputs move to top-level scoped
      views (IO.3), and in Phase 2 its partitions may read inputs (`from_input`, rule 5).

16. **`main:` and `macros:` compose in one runtime** (clarified 2026-10-08). Today they
    are mutually exclusive, because macros run in a separate context and #92 rejects
    the ignored `main:`. That is a Phase 0 guard, lifted in Phase 2. In the target, a
    macro expands into the same runtime as the hand-written `main:` (rule 3), so a macro
    can analyse a live main partition, plan over a hand-written model, or sit beside
    hand-written extras. Four rules keep that unambiguous:
    1. **The core language only has `main:`.** `stochadex expand` turns every macro into
       plain partitions with byte-identical output (rule 9). There is always exactly one
       simulation to reason about.
    2. **One clock** (rule 2). A clock-bearing macro and a hand-written clock are a
       duplicate-definition error.
    3. **Names are unambiguous.** A partition name may be defined once across `main:`,
       every input and every macro's outputs. A macro reference (`partition_name: y`)
       that could resolve to more than one is a config error naming each, never a
       precedence rule.
    4. **Expansion never reads data** (rule 5), so what a macro adds depends only on the
       config.

    A macro reads three sources: main partitions (live), input partitions (through a
    `from_input` replay its expansion emits), and earlier macros' outputs (ordinary
    `params_from_upstream` edges). The calibrate → plan chain is the exception: its
    earlier macros become a nested-run input. A macro generates config; what it
    generates is often analysis, inference, optimisation or search stepped forward as
    partitions, not a generative domain model.

### 2.3 Target YAML (end of Phase 2)

```yaml
inputs:                                   # named, read-only
  obs:    {source: {csv: {path: obs.csv, time_column: 0, state_columns: {y: [1]}}}}
  synth:  {simulation: {steps: 500, partitions: [...]}}        # today's data: sub-sim
  post:   {run: {inputs: {...}, macros: [{type: posterior_estimation, ...}]}}  # computed by a nested run
  orders: {stream: {websocket: {url: "wss://..."}}, decode: json,
           record: dat/orders.log, on_empty: hold_last}        # live

main:
  partitions:
    - name: flow
      iteration: {type: param_values}
      params_from_input: {param_values: orders}      # live injection
    - name: y_replay
      iteration: {type: from_input, input: obs}      # fixed replay as a partition
  expressions: [...]
  simulation:
    timestep_function: {type: from_input, input: obs}   # or constant, or macro-supplied
    termination_condition: {type: input_exhausted}      # or number_of_steps, ...
outputs:                                  # views: each sink gated by its own condition
  - {name: log,  condition: {type: every_step}, function: {type: json_log, path: run.log}}
  - {name: dash, condition: {type: only_given_partitions, partitions: [flow]},
     function: {type: websocket, url: "ws://..."}}
macros:                                   # each expands into THIS runtime
  - type: vector_mean
    name: rolling_mean
    data: {partition_name: y}             # resolves to an input replay or a main partition
    ...
run: {mode: batch | ensemble | serve, seeds: [...], websocket: {...}, pace_ms: 200}
```

`macros:` is optional. Every config above has an equivalent with no `macros:` key at
all, printed by `stochadex expand` (rule 9), and that equivalent is the authoritative
form. The core vocabulary is `inputs`, `partitions`, `iteration: {type: ...}`,
`expressions`, the `params_*` wiring, `embedded`, `simulation` and `run`. Words like
`posterior_estimation` or `vector_mean` appear only as macro type names.

Whether to flatten `main:` to the top level is a separate cosmetic decision for Phase 3
(see §6). Phases 0–2 keep `main:`.

### 2.4 Explicitly out of scope

- **A pipeline or workflow language inside the config.** No multi-stage `stages:`, no
  piping one `main:` into another. Use an external orchestrator.
- **Blocking I/O inside `Iterate`.** New live inputs use step-boundary injection. The
  keyboard iteration stays for back-compat, but it is not the pattern to copy.
- **Live inputs under `ensemble`, or inside reentrant / MCTS rollouts.** A live run is
  one real trajectory. `mcts_planning` can still search the forward model within it; the
  live input only drives the real trajectory, not the search.
- **Decision rules as data.** Unchanged: environments stay registered Go.

---

### 2.5 Outputs are views; `RunResult` is just an in-memory view

`simulator.OutputFunction` (`Configure` / `Output` / optional `Finalize`), gated by an
`OutputCondition` (`IsOutputStep(partition, state, timesteps)`), is already the engine's
general output abstraction. Stdout, json_log, Postgres, Arrow, DuckDB, S3, websocket and
in-memory storage are all implementations, and the condition makes each one a filtered
view. **The refactor should not introduce a second output concept beside it.** Today
`RunResult` (#95) is one: `RunToStorage` *replaces* the configured sink with an
in-memory one. The unifying formulation:

- **An output view = (name, condition, function).** A config declares zero or more
  views:
  ```yaml
  outputs:
    - {name: log,  condition: {type: every_step}, function: {type: json_log, path: run.log}}
    - {name: dash, condition: {type: only_given_partitions, partitions: [plan_apply]},
       function: {type: websocket, url: "ws://..."}}
  ```
  Today's single `simulation.output_condition` / `output_function` pair is shorthand
  for one unnamed view. Several views mean a tee, with each sink gated by its own
  condition. This is item 0.4 generalised.
- **The caller can attach views too.** The programmatic entry point takes options that
  add in-memory views (storage) and say whether the config's own views also run. That
  choice decides whether running for a result also writes the config's sinks, so it is
  an explicit option: suppressing them is the side-effect-free default for library use
  and checks; teeing is what an orchestrated step wants. **`RunResult` is then the set
  of in-memory views the caller asked for.** Its `Storage` field is the default view,
  and named views can be added later as a map. `RunToStorage` (#95) stays as the
  convenience form "one in-memory view mirroring the config's condition, config sinks
  suppressed". Adding named views to `RunResult` later is purely additive, so #95's
  shape does not need to change.
- **Ensembles: views are per member.** Today ensemble members drop the configured sink
  entirely (`runSeededMember` swaps in storage), so an ensemble config cannot write a
  json_log per member. Under views, each member gets fresh sink instances, with paths
  templated by member and seed (`path: "run-{member}.log"`), or a member-tagging
  wrapper for shared sinks. `RunResult.Members[i]` holds each member's in-memory views.
- **Nested views need scope.** A nested run's own sink *is* called, once per outer
  step. A probe confirmed a json_log inside the posterior's likelihood window wrote
  1608 correct lines (4 inner runs × 201 rows × 2 partitions). But each line carries
  only the inner partition name and inner time: nothing says which outer step it
  belongs to, and the inner `test_data` has the same name as the outer one. Rule:
  **a nested view's records are scoped**, with a path prefix
  (`test_likelihood/test_data`) plus the outer step and time alongside the inner time.
  This needs a small scoped-record extension to the sink interface or a wrapper.
  Streaming sinks take it naturally. A single `StateTimeStorage` has one time axis, so
  an in-memory *nested* view needs either one storage per outer step, or a flat
  storage with an outer-step column. That choice is open (§6 Q7).
- **Sinks that open files when the config is *loaded* break all of this.** json_log
  calls `os.Create` at load (found in #95), so a suppressed view still truncates its
  file, every ensemble member's reload truncates the same file, and a `--check` would
  clobber outputs. Views must open their resources in `Configure` and commit them in
  `Finalize`, which is also Track O's all-or-nothing outputs (O.4). Fixing json_log is
  the next PR.
- **Websocket serving is a view.** `run: {mode: serve}` says *where to listen* and that
  each connection is its own runtime. What each connection receives is an ordinary
  `outputs:` view, `{name: stream, condition: ..., function: {type: connection}}`
  (IO.1). #107 shipped a first form in which the stream lived in `run:` and borrowed
  the shorthand `output_condition`; IO.1 moves it into `outputs:` per rule 14. Live
  stream *inputs* (1.5) are the mirror image: sources bound at the step boundary, and
  declared in `inputs:`.

## 3. Capability preservation

| Capability today | Where it lands | Phase |
|---|---|---|
| `main:` batch / ensemble | unchanged | — |
| `embedded:` (hierarchical / nested sims) | unchanged; composition inside one runtime | — |
| Websocket serving (`-s` socket file) | `run: {mode: serve}` (#107), with the stream as an `outputs:` view `{type: connection}` (IO.1); `-s` kept as a deprecated alias | 1, 1b |
| A nested (embedded) run's own `output_function` | top-level `outputs:` views addressing nested partitions by scoped path (IO.3); the nested form becomes deprecated shorthand | 1b |
| Inline data in a partition (`from_storage` values, #86) | an `{inline: ...}` input source; the inline form becomes shorthand (IO.4) | 1b |
| `data.source` csv / json_log / postgres / arrow / s3 | `inputs: {x: {source: ...}}`; `data:` desugars to it | 1 |
| `data:` sub-simulation (6 configs/recipes) | `inputs: {x: {simulation: ...}}`, same draws | 1 |
| Against-storage macro chain (`example_macro_config`) | expanders in one runtime; **exact oracle** | 2 |
| Group C live macros (ES, MCTS ×2) | clock-bearing expanders | 2 |
| Group D (SMC, planning with `samples_from`) | clock-bearing expanders reading a fixed input | 2 |
| `RunMacros`, `RunEnsembleToStorage` | wrappers over `RunToStorage` | 0 |
| Calibrate → plan in one file (against-storage macro, then a live macro reading its output) | the earlier macros become a nested-run input (`inputs: {post: {run: ...}}`); the live macro reads it; same output, which also excludes the posterior columns as today | 2 |

New capabilities:

- `main:` reads real data from YAML (`from_input`). This covers the dominant downstream
  Go pattern.
- Macro results can go to any sink, so two-stage file pipelines work from YAML alone.
- Inference and optimisation can run under `ensemble`.
- Macro runs can be served over the websocket and drawn with `cmd/stochadex-graph`.
- Live inputs with record → replay.

---

## 4. Phases

**Performance bar for every item** (decided 2026-10-07): no slowdown in the step
loop. A change that touches anything run per step is benchmarked against v0.19.0
(the commit before #91), interleaving the two binaries under `benchstat`. The PR
states the numbers, and allocations per step are pinned by tests.

**Test bar for every item** (set by #91–#93):
- Assert against an independent reference, not the implementation's own logic.
- Drive the test through the public path (a YAML file into the public API).
- Assert the specific promised content, with positive controls against over-rejection.
- Mutation-check: revert the change and confirm its test fails.

Each phase ends with `go build ./...`, `go test ./...`, the CLI binary tests
(`test/binary_configs_test.go`), the skill-recipe drift test, and a CHANGELOG entry.

### 4.1 Versioning

**Decided (2026-10-04): the whole plan ships on v0.x.** Every phase, including Phase 3's
removal of the deprecated spellings, is a **minor** bump. That follows the CHANGELOG
policy: pre-1.0, breaking changes go in minors, with a "Changed" entry and migration
notes.

v1.0.0 is not tied to this plan. It is a separate decision: a stability promise over a
named public surface (YAML schema, exported `pkg/api`, the `Iteration` interface,
registry names), and v1 is Go's last major that keeps the import path. It should wait
until domain-model promotion into the core has settled into additive changes.

### 4.2 Expansion spike (Phase 2 entry gate)

Hand-write the core-language twin of each of three representative macros, run both
forms, and compare. The three:
- `vector_mean` / `vector_variance` (easy);
- `posterior_estimation` (nested sub-simulations);
- `smc_inference` (reads the full dataset while expanding; size risk).

For each macro, record:
- (a) whether the twin is exact;
- (b) the expansion size;
- (c) which `{type: ...}` specs were missing, and whether the needed ones are generic.

**Results (run 2026-10-04 on `main` @ `448d5cf`; artifacts in `spike/` and
`pkg/api/zz_spike_*_test.go` on branch `claude/spike-macro-expansion`, commit
`96c9fb7`; that branch is evidence only and is never merged). Verdict: GATE PASSED for the representative set.**

| Macro config | Twin exact? | Rows × partitions compared | Twin size (macro form) | New specs needed |
|---|---|---|---|---|
| `example_macro_config` (`vector_mean` → `vector_variance` chain) | **Yes, max abs diff 0** | 501 × 3 | 43 lines (36) | none |
| `example_posterior_macro_config` (`posterior_estimation`, nested likelihood window) | **Yes, max abs diff 0** | 2001 × 6 | 87 lines (~50) | none: the existing top-level `embedded:` block expresses the nested window |
| `example_smc_config` (`smc_inference`, 100 particles) | **Yes, max abs diff 0** | 4 × 3 | 72 lines (~45); **does not grow with particle count** | one, and it is generic (below) |
| `example_evolution_strategy_config` (`evolution_strategy_optimisation`, own clock) | **Yes, max abs diff 0** | 1001 × 5 | 80 lines (44) | none; the macro's `steps: 1000` became an ordinary `simulation:` block (rule 2 confirmed) |
| `example_planning_config` (`mcts_planning`, own clock, `pkg/agents`) | **Yes, max abs diff 0** | 9 × 2 (the search row includes the full tree and rollout state) | 96 lines with a YAML anchor (186 without); macro form 78 incl. comments | three `pkg/agents` iteration specs: `mcts_apply`, `mcts_tree`, `mcts_rollout` (below) |

**Harness validated.** Deliberately broken twins were all caught, including (for the
second round) the ES sampler seed, learning rate and reward target, and the MCTS tree
seed, search budget and rollout-environment reward. Earlier mutants caught: a different sampler
seed, a different function (`data_values` instead of `data_values_variance`), the SMC
proposal seed, the model variance, and an observed data point. Two mutants were
*correctly* not caught: the SMC evaluation seed (this example's particle model is
deterministic) and observed row 0 (the initial state comes from `init_state_values`).

**What the spike established:**
1. **The single-runtime design is numerically sound.** Every twin runs as **one**
   coordinator, with the data partition stepping live, and reproduces the macro path's
   separate data run plus replay passes exactly. This is direct evidence for Phase 2's
   core claim.
2. **The core language already covers most of it.** The aggregation and posterior
   expansions needed no new vocabulary. The only gap was SMC's particle evaluator.
3. **The gap is generic, and the code already exists.** `SMCParticleEvaluationIteration`
   lives in `pkg/macros`, but nothing in it is SMC-specific: "state a model once, run
   it N times per step, forward a slice of a parameter vector into each copy, derive
   seeds per (step, copy), output one value per copy." The spike registered it as
   `replicated_simulation_run` and the twin was exact.
   **Action:** promote it to `pkg/general` under that generic name, with a `{type: ...}`
   spec. It is reusable for ES reward evaluation, sensitivity sweeps and in-partition
   ensembles. This meets rule 10.
4. **Two couplings found, both already anticipated by rules 3 and 5:**
   - **Expansion reads storage for shapes.** `vector_variance` cannot even *expand*
     until `rolling_mean` has been run, because it looks up the mean's width in
     storage. In a config → config rewrite the width comes from the upstream
     partition's `init_state_values`. The likelihood window's initial state (data row
     0) is likewise just the data partition's `init_state_values`.
   - **Expansion reads data contents.** The SMC twin had to inline the observed series
     (61 values) through `from_storage`. That works exactly, but it embeds data in the
     config, so size grows with the data. This confirms rule 5's design: models nested
     inside `embedded:` / `replicated_simulation_run` need `from_input` bound to a named
     input, so the expansion is data-free.

**Round 2 findings (ES and MCTS planning):**
5. **Macros with their own clock are not special.** Both ES and MCTS expanded to a plain
   `simulation:` block, and the twins were exact.
6. **The MCTS iterations need `{type: ...}` specs, and the spike shows they can have
   them.** On the data path (`[]float64` states, `int` actions) the encoder and decoder
   are identity functions, and the `SimulationEnvironment` is fully determined by data:
   the model, actions, action partition and param, horizon, reward partition, return
   range and discount. The spike registered `mcts_apply` / `mcts_tree` / `mcts_rollout`
   taking that environment as a sub-spec, and the twin was exact. These are algorithm
   iterations in the catalogue, like `posterior_mean` or `smc_proposal`, not macro
   concepts. `mcts_self_play` uses the same three, so rule 10 is met. A production spec
   should split `environment:` (data-defined `{type: simulation, ...}`, or a registered
   name under rule 13) from the search settings (simulations, exploration, depths, seed).
   The spike reused the whole planning field set for both.
7. **The macro's shared Go environment instance is not needed.** The macro hands one
   environment object to all three partitions. In the twin each partition builds its
   own from the same data, and the output is identical.
8. **Shared sub-specs: use YAML anchors, not new vocabulary.** Repeating the environment
   in three places costs size (186 lines) and introduces a new risk: the copies can
   drift apart, and a twin whose rollout environment differed was caught as a mismatch.
   A plain YAML anchor (`planning: &env` … `planning: *env`) defines it once. It loads
   through the existing loader and dead-key check unchanged, stays exact, and halves the
   size to 96 lines. **`stochadex expand` should emit anchors for any sub-spec shared by
   several partitions.**
9. **Phase 2 needs a refactor split.** The spike had to split the planning spec's
   environment and search construction (`buildSelfPlay`) from its partition
   construction. That is exactly the expander shape of 2.1: build the components from
   config, and let the wiring become plain partitions.

**Round 3: the remaining cases (2026-10-04). Every case the plan depends on is now
covered. All 11 twins are exact.**

| Case | Twin exact? | Compared | Twin size | New specs needed |
|---|---|---|---|---|
| `mcts_self_play` with registered `tictactoe` | **Yes, max abs diff 0** | 11 plies × 2 | 54 lines | the `mcts_*` specs, taking `environment: {type: tictactoe, ...}` as a **named component** |
| `likelihood_mean_function_fit` | **Yes, max abs diff 0** | 101 × 2 | 50 lines | `data_comparison_gradient`: likelihood plus a named gradient function |
| `grouped_aggregation`, groups appearing over time | **Yes, max abs diff 0** | 41 × 3 | 32 lines | none; groups declared as the existing `accepted_value_group_tupindex_*` param |
| 3-level nesting (outer run → posterior → likelihood window), in one file | **Yes, max abs diff 0**; mutants at level 2 and level 3 both caught | 5 × 14 | 102 lines | `embedded_simulation_run` taking a recursive `run: {main, embedded}` |
| calibrate → plan chain as **two configs plus a json_log file** | **Yes, max abs diff 0** for both stages | stage 1: 2501 × 6; stage 2: 3 × 2 | 88 + 86 lines | a samples input on the planning environment (stand-in for a named input) |

**Round 3 findings:**
10. **Registered environments should be components, not macros.** Today
    `RegisterEnvironment` takes a builder that returns *partitions*, so it is a macro
    shipped from downstream. The spike registered the environment as a typed component
    behind a small wrapper with three methods, building the apply, tree and rollout
    iterations for any downstream state and action types. The core `mcts_*` specs
    reference it by name. The twin was exact.
    **API change:** `RegisterEnvironment` switches from a partition builder to a
    component builder; card-game-studio's `cardgame_rulesvm` must migrate. This is the
    one downstream-visible change the spike found.
11. **Component specs must not carry wiring knobs.** The self-play spec's search seed,
    `sims_per_ply` and `init_grid` turned out to be ignored by the iterations. The
    behaviour comes from the partition seeds, the inner run's step count and
    `init_state_values`, which the expansion sets. A production component spec carries
    only what defines the environment, or these become dead config the dead-key check
    can't see.
12. **`DataComparisonGradientIteration` is not a live object.** Its `Batch` is filled
    at run time by the nested-run machinery, as `from_history`'s data is. The coverage
    test's exclusion reason is wrong; a data spec is straightforward.
13. **Declared groups are already the iteration's interface.** The macro only
    *discovers* them and writes `accepted_value_group_tupindex_*`. Hazard found: a group
    that appears at run time but wasn't declared is **silently dropped** (its output is
    0). The production spec needs a policy: an error, a warning, or an "other" bucket.
    Discovery remains available as a computed input.
14. **Recursive nesting works.** One `embedded_simulation_run` spec whose `run:` is a
    full `main` + `embedded` config expresses any depth. Three levels were exact and
    every level was shown to be live. (A first attempt with the inner run shorter than
    the likelihood's burn-in had level 3 inactive; the level-3 mutant exposed it and the
    test was fixed.)
15. **Chaining through a file is lossless.** json_log round-trips float64 values
    exactly. The two-config calibrate → plan pipeline reproduces the in-memory macro
    chain exactly, and changing sample values in the file is caught. This is direct
    evidence for "one runtime per config, orchestrate outside" (Track O) and for
    `inputs:` (Phase 1).

**Process note:** two earlier "uncaught" mutants (ES window, fit descent steps) were
invalid: a `sed` pattern anchored on `3$` never matched lines ending in `}`. When redone
correctly, the fit mutant is caught. The ES window mutant is still legitimately uncaught,
because with a discount of 0 only the last step's constant reward counts. Mutation
scripts should assert their edit was applied.

**Overall verdict: GATE PASSED for the full macro set.** Every macro type and every
structural case the plan relies on (own clocks, registered components, data-dependent
expansion, nesting of any depth, cross-runtime chaining) has an exact core-language
twin.

The core vocabulary grows by **six generic specs**: `replicated_simulation_run`,
`mcts_apply`, `mcts_tree`, `mcts_rollout`, `data_comparison_gradient` and a recursive
`embedded_simulation_run`. It also needs one environment-component registry and a
named-input binding, which Phase 1 already plans. None of these is a macro concept.
Twins run about 1–2.5× the macro form's length (machine artifacts, and anchors keep
them compact).

The work this adds to Phase 2:
- items 2.0 and 2.0b;
- a `data_comparison_gradient` spec;
- the recursive embedded spec;
- the `RegisterEnvironment` migration;
- an undeclared-group policy;
- assert-applied mutation scripts.

### Phase 0 — tidy-up (no new schema)

The goal: every key that is accepted does something, there is one Go entry point, and
known bugs are fixed. Behaviour changes only where a config currently relies on silent
ignoring or silent discarding.

**Progress:** 0.1, 0.5 and 0.7 are merged (#91–#93); 0.2 was dropped. **Remaining:**
- 0.3: one `RunToStorage` entry point.
- 0.4: macro results through output sinks. This must re-admit `main.simulation.output_*`,
  which #92 currently rejects in macro mode.
- 0.6: a websocket push sink.
- Optionally, reword the unreachable live-macro `resolve` stub messages.

None of these depend on each other, so each can be its own PR.

| # | Item | Detail | Acceptance |
|---|---|---|---|
| 0.1 | **DONE (#92)**: reject ignored keys in macro mode | When `macros:` is present, error on `main.expressions`, `main.simulation` (all fields for now; 0.4 re-admits `output_function` / `output_condition`), `embedded:`, and `run:` other than `{mode: batch}`. Without `macros:`, error on `data:`. | New `pkg/api` tests per key; all `cfg/` and recipes still load |
| 0.2 | ~~Enforce "a live macro runs alone"~~ | **Dropped**: it would break the supported calibrate → plan chain (§1.3). Optionally reword the unreachable `resolve` stub messages, which claim the opposite. | — |
| 0.3 | **DONE (#95)**: one programmatic entry point | `api.RunToStorage(config) (*RunResult, error)` with `RunResult.Storage` (batch, macros) or `.Members` (ensemble). It records what the config's `output_condition` selects into an in-memory view, *in place of* the config's sinks, on a copy of the config. `Run` prints its result for macro and ensemble configs (in name order). `RunMacros` / `RunEnsembleToStorage` are deprecated wrappers. In §2.5 terms, this is the convenience form "one in-memory view, config views suppressed". 0.4 generalises it to caller-attached views with suppress or tee, and grows `RunResult` additively. | Storage matches the config's own json_log sink read back; config unmodified; mutation-checked (see #95) |
| 0.4 | Output views (§2.5), in four slices | **0.4a, DONE (#101):** a top-level `outputs:` list of (name, condition, function) views; `simulation.output_*` as shorthand for one view; a tee with per-view conditions (`simulator.OutputViews`); `RunToStorage` suppresses the views. **0.4b, DONE (#102):** macro results flow through views (interim: replay the final storage through them until Phase 2 removes the replay). **0.4c, DONE (#103):** programmatic options to attach in-memory views and to suppress or write through config views, with suppression covering nested views. **0.4d, DONE (#104):** per-member sink instances for ensembles (templated paths). | Each view receives exactly its condition's rows (checked against an independent single-sink run); a suppressed view writes nothing and creates no file; per-member ensemble logs match the members' storages; mutation-checked |
| 0.5 | **DONE (#91)**: fix the serve race | Fresh generator per connection (reload from `sourcePath`); a private `http.ServeMux`; stop on disconnect; configurable origins (default: same-origin + loopback). | Streams match an offline reference run under `-race`; one build per connection; disconnect, origin and YAML-loading tests |
| 0.6 | **DONE (#97)**: push websocket sink | A websocket *client* view function (`{type: websocket, url: ...}`), usable in `outputs:` like any sink and registered via `RegisterComponent`; it opens its connection in `Configure`, not at load (rule 12) | Test against an in-process `httptest` server; the stream matches an in-memory view of the same run |
| 0.7 | **DONE (#93)**: covariance init indexing | `i*num+j` | Widths 1–4 against an independent matrix, plus the YAML `default_value` path |

**Versioning:** 0.1 turned silent no-ops into errors. That is called out under
"Changed" in the CHANGELOG, as a minor bump (see §4.1).

### Phase 1 — inputs and live I/O (additive)

| # | Item | Detail | Acceptance |
|---|---|---|---|
| 1.1 | **DONE (#105, #106)**: `inputs:` block. The `main:` path is in #105; macros reading `inputs:` (`data:` as an alias) is in #106 | A map from name to one of `{source: ...}` (all registered sources) or `{simulation: {partitions, expressions, steps, timestep, init_time}}`. `data:` desugars to a single unnamed input. Partition names across inputs must be unique, or loading fails. | `data:` configs produce byte-identical output; dead-key check covers `inputs:` |
| 1.2 | **DONE (#105)**: `from_input` iteration | `{type: from_input, input: x, partition: p}` replays an input partition as a main partition. It builds on #86's inline `from_storage`, sourcing the rows from a named input instead of inline data. Plus `timestep_function: {type: from_input, input: x}` and `termination_condition: {type: input_exhausted}`. This promotes `FromStorageIteration` from "live-object, no data form" to a data spec, because the data now has a name. | Re-express one downstream pattern (e.g. a floodrisk forward run) as YAML, matching the Go version exactly; coverage test entry moves from excluded to registered |
| 1.3 | **DONE (#107)**: `run: {mode: serve}`. The stream moves into `outputs:` in IO.1 | Moves the socket file into the config: `websocket: {address, handle, allowed_origins}`, plus `pace_ms`. Each connection attaches a websocket *view* to its own fresh run (§2.5), alongside any config views. `-s` stays as a deprecated alias that fills these fields. | `cfg/socket.yaml` flow still works; new config form works; a served stream matches an in-memory view of the same run |
| 1.4 | **IN REVIEW (#110)**: injection port in the engine, `PartitionCoordinator.InjectParams` | Move dexetera's `ApplyActionState` idea into the engine as `simulator.InjectParams(coordinator, partition, key, values)` (or a `Stepper` hook). It runs only between steps. | Unit test: an injection before step k is visible at step k and not before |
| 1.5 | **DONE (#112, #113)**: stream inputs (1.5a), two-way serving (1.5b) | `inputs: {x: {stream: {<transport>: {...}}, decode: json \| protobuf_action_state, on_empty: hold_last \| default \| block \| step_per_message, record: path}}` bound with `params_from_input`. Add a `RegisterStream` hook. The websocket transport ships in the engine (gorilla is already a dependency); others such as Kafka/MQTT go downstream or in `cmd/`. | Live-then-replay test: run against an in-process websocket server with `record:`, replay from the record as a `source: json_log` input, and get identical storage |
| 1.6 | **DONE (#112)**: guard rails | Reject `stream:` inputs under `ensemble`. Pick a backpressure policy (bounded buffer, drop-oldest default, configurable). | Load-time error tests |
| 1.7 | Keyboard on the new path | Re-express keyboard input as a `stream` transport (`{keyboard: {...}}`) feeding a `param_values` partition. Keep `UserInputIteration` and mark it legacy. | The existing keyboard test still passes; the new form has a test |

### Phase 1b — one place in, one place out (rule 14)

Added 2026-10-07 after a review of the work so far against rule 14. Before this phase,
outputs could be declared in five places:
- `outputs:` views;
- the `simulation.output_*` shorthand, which batch, ensemble and serve each treated
  differently;
- an embedded run's own `output_function`;
- the CLI printing results when nothing was declared (macros, ensembles);
- the served stream in `run:` (#107).

Inputs had two stragglers:
- inline data inside a partition;
- model files read from a path inside a partition spec (an ONNX model).

| # | Item | Detail | Acceptance |
|---|---|---|---|
| IO.1 | **DONE (#108)**: the served stream is an `outputs:` view | A new output function `{type: connection}`, valid only under `run: {mode: serve}`, sends that view's rows to the connection being served. A serve config must declare exactly one, with its own condition. It no longer borrows the shorthand `output_condition`, and the shorthand is rejected under serve until IO.2 makes it an ordinary view. Each connection substitutes its websocket for the `connection` view and builds fresh instances of the others (`{connection}` as in #107). `--socket` keeps its exact old behaviour on a shorthand config (stream gated by `output_condition`, `output_function` not written), and is rejected with `outputs:` views, which it never supported (found in review: with `--socket`, views using `{connection}` were rejected at load, and views without it were rejected by the alias). | The served stream equals an in-memory capture with the view's condition; a serve config with zero or two `connection` views, or one outside serve, is a config error; the `--socket` flow is unchanged; mutation-checked |
| IO.2 | **IN REVIEW (#109)**: the shorthand becomes one view, in every mode | At load, `output_condition` / `output_function` becomes `outputs: [{name: output, ...}]`, so batch, ensemble and serve share one code path, and the run-mode rules (`{member}` / `{connection}`) apply to it as to any view. `stdout` becomes **instance-aware**: under ensemble or serve it prefixes each row with `member=<i> seed=<s>` or `connection=<i>`, as the CLI's ensemble printing already does, so it needs no placeholder. When a config declares no output at all, the default ("print to stdout") becomes an explicit default view. The CLI's separate printing paths for macros and ensembles then go away. **This replaces "reject or make per-member" (the old follow-up):** desugaring makes the silent drop impossible rather than an error. | Every shipped config prints the same lines as before (as a set: order was already non-deterministic for batch configs with several partitions, and changes by design for macros and ensembles); a shorthand `json_log` with `{member}` writes per member; an ensemble with a shorthand `json_log` without a placeholder is a config error naming the shorthand; `expand` shows the desugared view |
| IO.3 | Nested runs write through top-level views | An `outputs:` view can select nested partitions by scoped path (`test_likelihood/test_data`). Its records carry the scope (path, outer step and time), per §2.5. An embedded run's own `output_function` becomes deprecated shorthand for such a view. Needs Q7 decided first. | A scoped view of a nested run equals the nested sink's records, each with its outer step; two nested partitions with the same inner name are told apart |
| IO.4 | Inline data is an input | An `{inline: {times: [...], partitions: {name: [[...], ...]}}}` input source. The inline `from_storage` form becomes shorthand for an inline input plus `from_input`. | `cfg/example_from_storage_config.yaml` is byte-identical through the inline input |
| IO.5 | Model files are inputs | A file an iteration reads (an ONNX model, today) is declared once in `inputs:` and referred to by name, so `inspect --io` and provenance (O.5) see it. Either declared or discovered: decide by Q8. | The manifest lists the model file; changing its contents changes the provenance hash |
| IO.6 | Retire the shorthand forms | Once IO.1–IO.4 land, `data:`, the `simulation.output_*` pair and `--socket` print a deprecation notice. They are removed in a later v0.x minor (§4.1). | Each deprecated form prints its notice once and still gives identical results |

**What this unlocks:** O.2's manifest becomes "`inputs:` and `outputs:` after
desugaring", and its exhaustiveness test (the manifest's paths match what a run actually
reads and writes) becomes a check of rule 14 itself.

### Phase 2 — macros as expanders inside the single runtime

**Entry gate: the expansion spike (see §4.2).** Phase 2 starts only if the spike shows
that representative macros have exact core-language twins, that their expansions are a
manageable size, and that any new specs they need are generic (rules 9–11).

| # | Item | Detail | Acceptance |
|---|---|---|---|
| 2.1 | Expander interface (config → config) | Replace `macroSpec.resolve(storage)` / `liveMacroSpec.resolveLive(storage)` with `expand(ctx) (Expansion, error)`. `ctx` exposes **only config**: input *names* and their declared partition names and widths, plus the host partitions' names and widths. It never exposes storage contents. `Expansion = {partitions []PartitionConfig (data specs only), minHistoryDepth map[name]int, simulation SimulationConfigStrings (optional fields)}`. | All 12 macros implement it; each expansion round-trips through YAML marshal → load unchanged |
| 2.2 | Depth requirements | Translate today's `windows` map into `max(existing, required)` `state_history_depth` on the referenced partitions, whether they are input replays or main partitions | Covered by the oracle (2.6) |
| 2.3 | Data-ref resolution | A `DataRef.partition_name` resolves to a main partition, an earlier expander's partition, or an input replay. An input partition that is referenced but not yet replayed gets an auto-created `from_input` partition. | Unit tests per case |
| 2.4 | Clock as ordinary config | Macros that need a clock (ES, SMC, MCTS) emit `simulation:` fields in their expansion. Merging follows ordinary duplicate-definition rules (§2.2 rule 2). With no clock at all and one fixed input, the default is `{type: from_input}` on that input. | Conflict tests with error messages that name both definitions |
| 2.5 | Remove build-time data reads | `grouped_aggregation` takes declared `groups:`; old configs desugar with a one-off group discovery run as a computed input. `smc_inference` emits embedded particle runs whose observed-data partition is `from_input` instead of a copied series. `mcts_planning`'s `samples_from` / `weights_from` read a named input at run time instead of being baked into params. The old calibrate → plan configs desugar to a nested-run input. | The calibrate → plan tests in `test/` and `pkg/api` pass unchanged; expansion succeeds with the input files absent (proving it reads none) |
| 2.6 | **Exact oracle** | Desugar old `data:` + `macros:` to the new shape at load time. For every `cfg/example_*.yaml`, every skill recipe, and AMR's and cryptobook's macro configs, the new path must produce **byte-identical storage** to the Phase-0 path, with the same seeds. If it doesn't, apply CONVENTIONS' oracle ladder, step down explicitly (exact → claim-level → distributional), and record which level was used. Never widen a tolerance to make it pass. | Golden-output test kept permanently as a regression guard |
| 2.7 | Tooling coverage | The deadlock pre-flight and `cmd/stochadex-graph` operate on the single expanded generator, so macro configs are covered | Graph render test for `example_macro_config` shows the `rolling_mean → rolling_var` edge |
| 2.8 | `run:` everywhere | `ensemble` and `serve` work for macro configs | Ensemble posterior test: per-seed results differ, and the ensemble mean is near the truth |
| 2.9 | Retire the replay coordinator path | `runMacros`' sequential storage-threading loop is deleted; `AddPartitionsToStateTimeStorage` stays in `pkg/analysis` for Go users and notebooks | — |
| 2.0b | `pkg/agents` iteration specs | Register `mcts_apply`, `mcts_tree`, `mcts_rollout` with an `environment:` sub-spec (`{type: simulation, ...}` data-defined, or a registered name) plus search settings; add `pkg/agents` to the registry coverage scan (§4.2 findings 6 and 7) | The spike's exact planning twin, kept as a regression test |
| 2.0c | Remaining core specs from the spike | `data_comparison_gradient` (finding 12; fix the coverage exclusion reason); recursive `embedded_simulation_run` with `run: {main, embedded}` (finding 14), making top-level `embedded:` a special case of it; an undeclared-group policy for `values_grouped_aggregation` (finding 13) | The spike's exact fit, grouped and 3-level nested twins, kept as regression tests |
| 2.0d | Environments as components | Change `RegisterEnvironment` from a partition builder to a component builder (typed wrapper building the apply, tree and rollout iterations); the component spec carries only environment-defining fields (finding 11); migrate card-game-studio's `cardgame_rulesvm` | The spike's exact `tictactoe` self-play twin; card-game-studio's tests pass after migration |
| 2.0 | Promote `replicated_simulation_run` | Move `SMCParticleEvaluationIteration` from `pkg/macros` to `pkg/general` under the generic name, with a `{type: ...}` spec (§4.2 finding 3); `smc_inference` expands to it | The spike's exact SMC twin, kept as a regression test |
| 2.11 | `stochadex expand` and the twin oracle | A CLI subcommand and an `api.Expand(config)` function emit the core-language config. For every shipped macro config, the expanded config runs byte-identically to the macro form. Sub-specs shared by several partitions are emitted once as YAML anchors (§4.2 finding 8). **Known core-language gaps to close by adding `{type: ...}` specs** (from `registry_coverage_test.go` and the macro constructors): (a) `DataComparisonGradientIteration` (`likelihood_mean_function_fit`), excluded today as "live-object built by macros:"; (b) an embedded simulation *inside a partition* (likelihood windows, SMC particles, the planning model), where `embedded:` is top-level only, so nesting needs a recursive spec; (c) the `pkg/agents` MCTS iterations, now item 2.0b; the spike proved their data-path specs give an exact twin. Each gap is either closed or recorded as a known gap; a tolerance is never widened. | The golden twin test passes for every `cfg/` macro config and skill recipe; the coverage test gains the agents package |
| 2.12 | Performance gate | Benchmark each shipped macro config, macro path versus expanded single-runtime path: wall-clock, allocations and peak memory, under the default and `inline` / `persistent_worker` execution strategies | No regression beyond noise on any config before 2.9 retires the old path; results recorded in `docs/performance.md` |
| 2.13 | Core semantics reference and schema | A written reference for the core language (execution order, within-step vs lag-1 reads, embedded-run semantics, clock resolution, input binding, origin annotations), plus a JSON Schema generated from the registries for editor and agent tooling | Schema drift test: every registered `{type: ...}` appears in it; the reference is linked from the skill |
| 2.10 | Docs | Rewrite the CLAUDE.md config section; `pkg/api/doc.go`; `pkg/macros/doc.go` ("macros are config → config shortcuts over the core language"); the `stochadex-model` skill and recipes, keeping `TestSkillRecipesMatchExamples` green; `docs/how_it_works.md`; CHANGELOG | Skill falsifier: a fresh agent given only the skill authors an input-driven config and a chained-macro config correctly |

### Track O — orchestration readiness (runs alongside Phases 1–2)

**Goal:** make a config a well-behaved task for a cloud orchestrator (DBOS steps,
Argo/Airflow tasks, Step Functions states, Kubernetes Jobs). The phases supply the
structure: one runtime per config, declared inputs and outputs, and registry-backed
storage. This track supplies what an orchestrator needs to drive that structure:
- parameterise each invocation;
- see a step's inputs and outputs without running it;
- decide whether to retry a failure;
- trust that a retry or crash leaves no partial outputs;
- skip work whose inputs haven't changed;
- spread one ensemble across machines.

**What already exists:**
- an OCI image;
- `LogRunProvenance`, which stamps the build and features and reads an orchestrator-injected
  `STOCHADEX_IMAGE_DIGEST`;
- the S3 / Arrow / DuckDB sources and sinks;
- seeded, reproducible runs.

**Design rule:** keep orchestration *outside* the engine (§2.4). Nothing in this track
adds stages, dependencies or scheduling to the config. It only makes one invocation
easy to parameterise, inspect, retry and cache from outside.

**Nesting vs splitting:** nested-run inputs (`inputs: {x: {run: ...}}`) keep a small
pipeline in one file, but they hide that stage from the orchestrator. Nest only stages
that are cheap and never worth checkpointing on their own. Split anything you'd want
retried, cached or monitored separately into its own config.

| # | Item | Detail | Depends on | Acceptance |
|---|---|---|---|---|
| O.1 | **Per-invocation overrides** | A CLI `--set path=value` (repeatable), plus `${VAR}` placeholders resolved from the environment at load time. Paths address the config tree (`inputs.obs.source.s3.key`, `main.partitions[name=w].seed`, `run.seeds`). List entries are selected **by name only**; there are no positional indices (§6 Q6). Overrides are applied before the dead-key check and validation, so a bad path is a load error. Replaces cryptobook's YAML text substitution. | Phase 1 (`inputs:` gives stable, named paths); the `main:` / `run:` paths can land earlier | An overridden run matches the same run with the value edited into the file; an unknown path or type mismatch is rejected with the path named; an unset `${VAR}` is an error, not an empty string |
| O.2 | **I/O manifest and dry run** | `stochadex inspect --io -c cfg.yaml` emits JSON describing the run: inputs (kind, location, partitions), output views (name, condition, sink, location, scope for nested views), run mode, seeds, the clock source, and the resolved overrides. `--check` loads, validates and runs the deadlock pre-flight, then exits without running. | Phase 1 for `inputs:`; it can report today's `data:` / `output_function` earlier | The manifest is golden-tested for every `cfg/example_*.yaml`; `--check` exits 0 on every shipped config and non-zero, naming the problem, on each known-bad case; a test checks that the manifest's input/output paths line up with what a real run reads and writes |
| O.3 | **DONE (#99)**: structured exit codes | Distinct codes for: a config or validation error (never retry); input unavailable or a transient I/O failure (retry); a runtime numerical failure (don't retry); success. Replace the `panic` / `log.Fatal` paths in loading and `Run` with typed errors mapped at the CLI edge. Library callers get the typed errors through `RunToStorage` (0.3). | 0.3 | A table test drives one representative failure of each class through the CLI binary and asserts its exit code; no `panic` is reachable from a bad config |
| O.4 | **All-or-nothing outputs** | Every file and object *view* (§2.5, rule 12) opens in `Configure`, writes to a temporary name, and commits in `Finalize` on clean termination: rename for local files, a final put or multipart complete for S3. A failed or killed run leaves no output at the final path. Sinks without an atomic commit (Postgres, websocket) document their behaviour. | 0.4 (sinks for every runtime) | Kill a run mid-way, then check no final-path output exists; let it complete and check the output matches a reference; each sink states its guarantee in its docs |
| O.5 | **Provenance hashes / caching key** | Extend `LogRunProvenance` and write a sidecar `*.provenance.json` alongside the outputs. It holds the config hash (after overrides), each input's content hash or object version (or a recorded-stream path), the seeds, and the existing build/image fields. Optional `--skip-if-unchanged`: exit with a "cached" status when an existing sidecar matches. Also usable directly as a DBOS idempotency key. | O.1, Phase 1 | The same config, inputs and seeds give the same hash, and changing any one changes it (property test); `--skip-if-unchanged` skips only on an exact match |
| O.6 | **Splitting an ensemble across machines** | Ensemble seeds can be overridden per invocation (`--set run.seeds=…` via O.1, or `--seed-range 1000:1999`). Each shard writes its own outputs and provenance. Gathering shards is left to the orchestrator or downstream. | O.1 | Two shards' outputs together equal the single-machine ensemble with the same seeds, member for member |

**Order:**
1. O.3 and the `main:` / `run:` part of O.1 can start now. They don't need `inputs:`.
2. O.2, O.5 and the rest of O.1 follow Phase 1.
3. O.4 follows 0.4.
4. O.6 is a thin layer on O.1.

Every item meets the §4 test bar.

### Phase 3 — optional follow-ups (decide later)

- Flatten `main:` to the top level, and remove the deprecated `data:` / `main:` / `-s` /
  `RunMacros` spellings. This ships as a v0.x minor with a "Changed" entry (see §4.1).
- Add `run: {mode: sweep}` over a params grid. It fits because `run:` now covers every
  runtime. Only worth doing if external orchestration proves clumsy for homark- and
  AMR-style scenario sweeps.
- (Provenance hashing moved to Track O, item O.5.)

---

## 5. Downstream impact

| Repo | Today | Impact |
|---|---|---|
| cryptobook | `pkg/cfgrun` → `RunMacros` / coordinator with a swapped output; json_log handoff | None required. Can adopt `RunToStorage` (0.3), and the stale comment about `RunEnsembleToStorage` can be dropped. `record-feed` could become a `stream:` input with `record:` (1.5). |
| card-game-studio | `RegisterEnvironment` + `RunMacros` | None. `RunMacros` is kept as a wrapper. |
| AMR | `cfg/amr_inference.yaml` (`data.source` csv + `posterior_estimation`) | None (desugars). It could later replay data into forward runs via `from_input`. |
| dexetera | its own `ApplyActionState` + wasm stepper | None. It can adopt the engine's injection port (1.4) and the `protobuf_action_state` decoder. |
| homark, floodrisk, trywizard, anglersim, bathing-water | Go `FromStorageIteration` pipelines | None. They can move forward runs to YAML via `from_input` (1.2). |
| Go-only repos | coordinator / harness | None. |

Downstream repos that use local path-replace pick these changes up immediately; others
pick them up at the next tag. Follow the release-flow ritual.

---

## 6. Open questions

**Policy (2026-10-05):** don't resolve questions ahead of the evidence. Each one is
closed once the work since has answered it, or given a "decide by" point and the
evidence to collect before then. None blocks the next PRs.

**Closed:**
1. ~~Interim output in macro mode (0.4).~~ **Closed by §2.5:** `outputs:` (views) is the
   permanent form, with `simulation.output_*` as shorthand for one view. No temporary
   `output:` key.
3. ~~Group C macros alongside hand-written partitions.~~ **Closed by the spike (§4.2
   findings 5 and 9):** a clock-bearing macro expands to ordinary partitions plus
   `simulation:` fields, so sharing a runtime is allowed and a clash is an ordinary
   duplicate-definition error (rule 2).
4. ~~`grouped_aggregation` against a live partition.~~ **Closed by spike finding 13:**
   declared groups are already the iteration's interface, so yes. The remaining
   undeclared-group policy (error, warning, or "other" bucket) is part of item 2.0c.
6. *Override path syntax:* list entries by **name only**, decided 2026-10-04. The
   `${VAR}` part is still open (below).

**Open, with "decide by" points:**

| Q | Question | Decide by | Evidence to gather first |
|---|---|---|---|
| 2 | ~~Stream clock: `hold_last` only, or also `step_per_message` (an event clock)?~~ **Decided (2026-10-08): `hold_last` only, for now** (#112). Add an event clock when a real feed needs one. **Evidence (2026-10-08):** dexact's protocol (dexetera's websocket driver) is lock-step, one step per inbound `ActionState`. So it is the first concrete user, if dexact ever drives a server-side stochadex serve. Nothing downstream uses that path today; cryptobook's feed is the other candidate | Phase 1.5 | — |
| 5 | Flatten `main:` to the top level? | Phase 3 | Agent test A.1: does the `main:` level cause agent authoring errors? Plus the migration cost across downstream configs and recipes |
| 6 | May `${VAR}` placeholders appear anywhere, or only in string values? | O.1 | What cryptobook's `cfgrun` substitutes today (paths only, or numbers too), and whether non-string placeholders break the dead-key check or the type errors |
| 7 | In-memory view of a nested run: one storage per outer step, or a flat storage with an outer-step column? | IO.3 (rule 14 now gives the direction: nested runs write through top-level views, so their records must carry scope) | Who reads nested views (debugging likelihood windows, inspecting MCTS trees) and what shape they want; streaming sinks just carry the scope fields either way |
| 8 | Model files (e.g. ONNX): declared in `inputs:` and referred to by name, or discovered by `inspect --io` from known spec fields? | IO.5 | How many registered iterations read files, and whether downstream registrations (`RegisterIteration`) can declare which of their fields are file paths. Declaring keeps rule 14 exact; discovering keeps configs shorter |

## 7. Risks

| Risk | Mitigation |
|---|---|
| The Phase-2 desugar changes numerical results (replay clock, history depth, init rows, RNG stream order) | Byte-identical golden oracle across every shipped config (2.6); explicit oracle step-down if it is ever needed, never widened tolerances |
| Live I/O leaks non-determinism into the harness / reentrant guarantees | Rule 6: injection only at step boundaries, transports outside `Iterate`; recording plus a replay-equivalence test (1.5) |
| Serve mode is exposed beyond localhost | Configurable origins (0.5); document it as a local/dev surface |
| Making silent ignores into errors breaks someone's config | CHANGELOG "Changed" entry; error messages say exactly what to move where |
| Scope creep into a workflow language | §2.4 out-of-scope list; one runtime per config is a load-time invariant, not a convention |

## 8. Use-case walkthroughs (target state)

- **Forecast from real data:** `inputs: {obs: {source: csv}}`, a `main:` model with
  `from_input` drivers, the input's clock, and a `json_log` output. One file, one run.
- **Calibrate, then forecast (DBOS/Make):** config 1 has an `obs` input and
  `posterior_estimation`, and writes posterior samples to arrow. Config 2 reads them as
  an input with `mcts_planning samples_from`, or as forward-run params. Two steps,
  joined by a file. With Track O:
  - each step is parameterised with `--set` (O.1);
  - the graph is built from `inspect --io` (O.2);
  - retries follow the exit codes (O.3);
  - a crash leaves no half-written posterior (O.4);
  - an unchanged calibration is skipped through its provenance hash (O.5, also the DBOS
    idempotency key).
- **A large ensemble on a cluster:** one config, N orchestrator tasks each running a
  seed range (O.6), with per-shard outputs and provenance, gathered downstream.
- **Robustness of an inference:** the same posterior config with
  `run: {mode: ensemble, seeds: [...]}`.
- **Live LOB model:** `inputs: {book: {stream: websocket, record: dat/book.log}}`,
  injected into `param_values` partitions, an online `posterior_estimation` expander,
  and a websocket push sink to a dashboard. Afterwards, replay `dat/book.log` as a
  `source:` input to reproduce the session exactly.
- **Interactive game / dashboard (dexetera-style):** `run: {mode: serve}` with
  `protobuf_action_state` stream input; each client gets an isolated runtime.
- **Self-contained demo / skill recipe:** `inputs: {synth: {simulation: ...}}` with one
  macro. Still one file, and the draws are identical to today.

## 9. How agents will find this change

**Net expectation: better, provided the skill teaches macros first, core as the
reference, and `expand` to inspect. Measure it rather than assume it (item A.1).**

**What should help:**
- **One fixed vocabulary.** No keys that are silently ignored (#92), no
  macro-versus-main runtime split to reason about, and one meaning per construct.
  Agents fail most on hidden semantics, and this removes the main source.
- **Fast checking without running.** `--check`, `inspect --io` and `expand` (Track O,
  2.11) let an agent validate its wiring, see what a macro produces, and confirm its
  inputs and outputs before spending a run. Errors name the macro that produced a
  partition (rule 11).
- **Twins as worked examples.** Every macro has a readable expansion, so an agent that
  needs to go beyond a macro can start from its expansion instead of from scratch.
- **Overrides (`--set`, by name).** Variations such as seeds, paths and parameters are
  one flag, not a YAML rewrite. That is fewer edits and fewer chances to corrupt a
  config.
- **A schema (2.13).** Lets agent tooling validate or constrain generation directly.

**What could hurt, and the mitigations:**
- **Core-language configs are wiring-heavy.** Correct widths, flattened initial vectors,
  index params such as `grouping_partition_tupindex_0`, and slot offsets such as
  `best_idx`. The spike's own twins needed one correction caught by the oracle (the fit
  window), and some were generated programmatically for this reason. *Mitigation:* the
  skill says to use a macro whenever one fits and to drop to core only for bespoke
  parts. Widths should be derived wherever possible, so expansion and validation
  compute them rather than authors.
- **More catalogue to learn** (six new specs, components). *Mitigation:* the schema plus
  generic naming (rule 10) keeps them guessable.
- **Transition confusion.** Old shapes (`data:` / `macros:` chains, `-s`) will live in
  downstream repos and in model training data for a while. *Mitigation:* desugaring
  keeps them working through the v0.x minors, and deprecation warnings point to the new
  spelling.
- **Composition invites over-engineering.** Deep nesting is easy to write and expensive
  to run. *Mitigation:* `inspect` reports nesting depth and estimated inner-step counts.

| # | Item | Detail | Acceptance |
|---|---|---|---|
| A.1 | Agent falsifier, before vs after | Reuse the skill-validation method: fresh agents, given only the skill, author a fixed task set (a forward model from data, a calibration, a calibrate → plan pipeline, a bespoke composition such as ES over a posterior). Run it on today's skill and again after Phase 2 plus the skill rewrite (2.10). | Same or better task success, fewer iterations to a converging config, and no new failure class. Results recorded in PLAN.md |

