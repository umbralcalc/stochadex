# Stochadex Project Template

A starting template for building simulations with the [stochadex](https://github.com/umbralcalc/stochadex) SDK.

## Setup

1. Copy this template into your new project directory:

```bash
cp -r template/ /path/to/my-project
cd /path/to/my-project
```

2. Update the Go module name in `go.mod`:

```bash
# Replace the module path with your own
sed -i '' 's|github.com/example/my-stochadex-project|github.com/youruser/yourproject|g' go.mod
```

3. Install dependencies:

```bash
go mod tidy
```

4. Verify everything works:

```bash
go build ./...
go test -count=1 ./...
```

## Project structure

```
.claude/commands/       Claude Code skills for development
  new-iteration.md      /new-iteration — scaffold a new Iteration implementation
  new-config.md         /new-config — scaffold a new simulation config
CLAUDE.md               Project conventions for Claude Code (loaded automatically)
pkg/custom/             Example custom iteration (MovingAverageIteration)
cfg/                    Simulation configs (YAML, for the API code-generation path)
```

## Running simulations

Run a config using the stochadex CLI:

```bash
go run github.com/umbralcalc/stochadex/cmd/stochadex --config cfg/builtin_example.yaml
```

## Using Claude Code skills

With [Claude Code](https://claude.com/claude-code) installed:

- `/new-iteration exponential decay process` — scaffolds a new Iteration with tests
- `/new-config two coupled oscillators` — generates a simulation config YAML

## Examples included

- **`pkg/custom/moving_average.go`** — A custom `Iteration` computing an exponential moving average of an upstream partition's state.
- **`cfg/builtin_example.yaml`** — Config using only built-in iterations (Wiener process + Ornstein-Uhlenbeck).
- **`cfg/custom_example.yaml`** — Config combining a built-in Wiener process with the custom `MovingAverageIteration`.
