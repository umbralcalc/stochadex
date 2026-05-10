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

## Example analysis notebooks

- [Examples with CSV files](https://github.com/umbralcalc/stochadex/blob/main/nbs/csv.ipynb)
- [Examples with Dataframes](https://github.com/umbralcalc/stochadex/blob/main/nbs/dataframe.ipynb)
- [Examples with JSON Log Entries](https://github.com/umbralcalc/stochadex/blob/main/nbs/logs.ipynb)
- [Examples with Partitions](https://github.com/umbralcalc/stochadex/blob/main/nbs/partitions.ipynb)
- [Examples with a Postgres DB](https://github.com/umbralcalc/stochadex/blob/main/nbs/postgres.ipynb)
