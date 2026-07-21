# stochadex

A Go SDK — and a no-toolchain YAML engine — for building, configuring, and running simulations
of stochastic processes and complex systems. A simulation is a set of independent **partitions**,
each advancing its own state each step, run concurrently and wired together through a small set of
well-defined channels.

You can drive stochadex three ways, from easiest to most flexible:

1. **As a Claude Code plugin** — an agent authors and runs a simulation as YAML for you.
2. **As a CLI** — write one YAML config, run a prebuilt binary. No Go toolchain required.
3. **As a Go library** — build `Settings` + `Implementations` and embed the engine.

---

## 1. Use it with an agent (Claude Code plugin)

The `stochadex-model` skill teaches an agent to author, run, and analyse a simulation as a single
YAML config — the expressions DSL, the `{type: ...}` registries, partition wiring, run modes, and
the inference/optimisation macros — with four validated, converging worked recipes.

```bash
# Add this repo as a plugin marketplace, then install the plugin:
claude plugin marketplace add umbralcalc/stochadex
claude plugin install stochadex@stochadex
```

Then just describe a system ("simulate a noisy logistic-growth population", "recover the mean of
this data stream") — the skill activates automatically. It drives the `stochadex` CLI below, so
install that too.

## 2. Run a config from the CLI (no Go toolchain)

Install the prebuilt binary:

```bash
# Prebuilt (no Go needed) — picks your platform's asset from the latest release:
curl -L "https://github.com/umbralcalc/stochadex/releases/latest/download/stochadex-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')" -o stochadex
chmod +x stochadex

# Or, if you have Go:
go install github.com/umbralcalc/stochadex/cmd/stochadex@latest
```

Write a config — a 1-D random walk, recorded each step:

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

A config that names no Go anywhere resolves and runs **in-process** — no codegen, no `go run`.
See [`cfg/`](cfg/) for worked examples (ensembles, online inference, evolution-strategy
optimisation, regression) and [`.claude/skills/stochadex-model/`](.claude/skills/stochadex-model/)
for the full authoring guide and recipes.

## 3. Embed it as a Go library

```bash
go get github.com/umbralcalc/stochadex
```

Build a run with `simulator.ConfigGenerator` (`SetPartition` / `SetSimulation` →
`GenerateConfigs`) and execute it. The [quickstart](https://umbralcalc.github.io/stochadex/) walks
through a complete program.

---

## Documentation

- **Docs site:** https://umbralcalc.github.io/stochadex/ — quickstart, how-it-works, package reference.
- **Changelog:** [`CHANGELOG.md`](CHANGELOG.md).
- **Contributing conventions:** [`CLAUDE.md`](CLAUDE.md) (framework), [`models/CONVENTIONS.md`](models/CONVENTIONS.md) (domain-models catalogue).

## License

[MIT](LICENSE).
