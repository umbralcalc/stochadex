---
title: "Quickstart"
logo: true
---

# Quickstart
<div style="height:0.75em;"></div>

## Cloning the repository

```bash
git clone git@github.com:umbralcalc/stochadex.git
cd stochadex
```

## Building and running the binary

```bash
# Update the go modules
go mod tidy

# Build the binary
go build -o bin/ ./cmd/stochadex

# Run your config
./bin/stochadex --config ./cfg/example_config.yaml
```

## Running over websocket

```bash
# Run the stochadex with a socket config
./bin/stochadex --config ./cfg/example_config.yaml \
    --socket ./cfg/socket.yaml
```

## Building and running the containerised version (may need sudo)

```bash
# Build the stochadex container
docker build -t stochadex -f Dockerfile.stochadex .

# Run the binary in the container with your configs
docker run -p 2112:2112 stochadex --config ./cfg/example_config.yaml \
    --socket ./cfg/socket.yaml
```

## Developing the code

You can add any new simulation partition you like by following the patterns for other processes given, e.g., in the [pkg/continuous](https://umbralcalc.github.io/stochadex/pkg/continuous.html) package.

- The main step is to create a new struct for your partition iterator which implements the [simulator.Iteration](https://umbralcalc.github.io/stochadex/pkg/simulator.html#Iteration) interface.
- It is then strongly recommended that a test function for this new iterator is written, which should include a test that calls [simulator.RunWithHarnesses](https://umbralcalc.github.io/stochadex/pkg/simulator.html#RunWithHarnesses).

## Using the analysis package

The [pkg/analysis](https://umbralcalc.github.io/stochadex/pkg/analysis.html) package provides tools for analysing simulation outputs and building new simulations.

The plots work well within GoNB notebooks (notebooks with a Go-friendly Jupyter Kernel) and there are some simple examples of what you can do provided the [nbs/](https://github.com/umbralcalc/stochadex/tree/main/nbs) folder.

In order to use the GoNB Jupyter Kernel, please install GoNB from here: [https://github.com/janpfeifer/gonb](https://github.com/janpfeifer/gonb).
