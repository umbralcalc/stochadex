---
title: "Quickstart"
logo: true
---

# Quickstart
<div style="height:0.75em;"></div>

## Cloning the repository

```shell
git clone git@github.com:umbralcalc/stochadex.git
```

## Building and running the binary

```shell
# update the go modules
go mod tidy

# build the binary
go build -o bin/ ./cmd/stochadex

# run your config
./bin/stochadex --config ./cfg/example_config.yaml
```

## Running over websocket

```shell
# run the stochadex with a socket config
./bin/stochadex --config ./cfg/example_config.yaml \
--socket ./cfg/socket.yaml
```

## Building and running the containerised version (may need sudo)

```shell
# build the stochadex container
docker build -t stochadex -f Dockerfile.stochadex .

# run the binary in the container with your configs
docker run -p 2112:2112 stochadex --config ./cfg/example_config.yaml \
--socket ./cfg/socket.yaml
```

## Developing the code

You can add any new simulation partition you like by following the patterns for other processes given, e.g., in the **pkg/continuous** package.

- The main step is to create a new struct for your partition iterator which implements the **simulator.Iteration** interface.
- It is then strongly recommended that a test function for this new iterator is written, which should include a test that calls **simulator.RunWithHarnesses**.

## Using the analysis package

The **pkg/analysis** package provides tools for analysing simulation outputs and building new simulations on top of them. The plots work well within GoNB notebooks (notebooks with a Go-friendly Jupyter Kernel) and there are some simple examples of what you can do provided the **nbs/** folder. So take a look!

In order to use the GoNB Jupyter Kernel, please install GoNB from here: [https://github.com/janpfeifer/gonb](https://github.com/janpfeifer/gonb).
