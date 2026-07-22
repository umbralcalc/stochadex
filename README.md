# stochadex

A Go SDK — and a no-toolchain YAML engine — for building, configuring, and running simulations of
stochastic processes and complex systems, with online inference and simulation-based
decision-making over one composable primitive.

**📖 Full documentation: [stochadex.github.io](https://stochadex.github.io/)**

## Install

**As a Claude Code plugin** — an agent authors and runs simulations as YAML for you:

```bash
claude plugin marketplace add umbralcalc/stochadex
claude plugin install stochadex@stochadex
```

**As a CLI** — one YAML config, a prebuilt binary, no Go toolchain:

```bash
curl -L "https://github.com/umbralcalc/stochadex/releases/latest/download/stochadex-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')" -o stochadex
chmod +x stochadex
```

**As a Go library**:

```bash
go get github.com/umbralcalc/stochadex
```

## Where to start

| | |
|---|---|
| [Quickstart](https://stochadex.github.io/pkg/quickstart.html) | Your first simulation, in Go. |
| [Running with configs](https://stochadex.github.io/pkg/configs.html) | The YAML/CLI path — models, analysis and inference without writing Go. |
| [How it works](https://stochadex.github.io/pkg/how_it_works.html) | The execution model behind partitions and histories. |
| [Domain model index](https://stochadex.github.io/pkg/model_index.html) | The catalogue of real-world model stubs. |
| [Package reference](https://stochadex.github.io/pkg/simulator.html) | Generated API docs. |

## License

[MIT](LICENSE). Changelog: [`CHANGELOG.md`](CHANGELOG.md).
