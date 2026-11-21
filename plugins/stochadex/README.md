# stochadex for Claude

Skills that let Claude build and experiment on [stochadex](https://stochadex.github.io)
simulations, starting from a diagram.

You give Claude a diagram of a system: Mermaid, a draw.io or Excalidraw file, boxes and
arrows typed into the chat, or a photo of a whiteboard. Claude reads it as a stochadex
config, checks and runs it, tests what you expect of it, and writes what the model did back
onto the diagram. You change the diagram or ask what would happen if, and it goes round
again.

## Install

```bash
claude plugin marketplace add umbralcalc/stochadex
```

```bash
claude plugin install stochadex@stochadex
```

Inside a session the same commands are `/plugin marketplace add umbralcalc/stochadex` and
`/plugin install stochadex@stochadex`. The plugin states no version, so an install follows
this repository: `claude plugin update stochadex@stochadex` brings it up to date.

The skills drive the `stochadex` CLI, a single prebuilt binary. If it is not installed,
Claude offers to fetch it from the
[latest release](https://github.com/umbralcalc/stochadex/releases/latest). The run report
script needs `python3`, with no packages.

## Skills

| Skill | What it does |
|---|---|
| `/stochadex:experiment` | The loop. Give it a diagram, or a folder from an earlier session. |
| `/stochadex:diagram` | Reads a diagram as a model, and draws a config and its results as a diagram. |
| `/stochadex:config` | Writes, checks, runs and debugs a stochadex config. |

Claude also picks these up without the slash command: "simulate this diagram" is enough.

## What a session leaves behind

One folder per system, all plain text:

```text
diagram.md        the living diagram, with claims and assumptions
model.yaml        the stochadex config the diagram currently is
claims.json       what you expect of the model, as tests
experiments.md    one entry per experiment: question, change, result, reading
original.*        your diagram as you first gave it
runs/NNN/         each experiment's config, logs and summary
```

`model.yaml` is an ordinary stochadex config. It runs with `stochadex --config model.yaml`
with or without Claude, and anywhere the engine does.

## How the skills stay true to the engine

Every complete config in the skills is run by the engine's own test suite
(`TestDocsExamples`), against the output the skill shows. Beyond those examples the skills
send Claude to the binary's own account of itself (`stochadex catalogue`, `--check`,
`graph`, `inspect`) rather than to a copy of it.
