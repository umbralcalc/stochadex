![](./logo.png)

# So what is the 'stochadex' project?

It's a simulation engine which can be used to generate samples from and statistically infer a whole 'Pokédex' of possible systems. 'Pokédex' here is just a fanciful metaphor for the large range of simulations that might come in useful when taming the complex descriptions of real world systems... and _kind of_ gives us the name 'stochadex'. The hope for this project is that it can become the basis upon which to build generalised software solutions for a whole lot of different and interesting problems!

From a software engineering perspective, the stochadex simulation framework abstracts away many of the common features that sampling algorithms have for performing these computations behind a highly-configurable interface. This isn't particularly new as a concept (see, e.g., [SimPy](https://gitlab.com/team-simpy/simpy/), [StoSpa](https://github.com/BartoszBartmanski/StoSpa), [FLAME GPU](https://github.com/FLAMEGPU/FLAMEGPU2/) and loads more), however the software can be leveraged in future projects, and, to be honest, writing the code from scratch has just been a lot of fun in Go!

## Need more context and documentation?

- Simulation engine motivation and design article: [https://umbralcalc.github.io/posts/stochadexI.html](https://umbralcalc.github.io/posts/stochadexI.html).
- A probabilistic description of simulations: [https://umbralcalc.github.io/posts/stochadexII.html](https://umbralcalc.github.io/posts/stochadexII.html).
- Estimation-based inference of simulations: [https://umbralcalc.github.io/posts/stochadexIII.html](https://umbralcalc.github.io/posts/stochadexIII.html).
- Optimising action-taking within simulations:  [https://umbralcalc.github.io/posts/stochadexIV.html](https://umbralcalc.github.io/posts/stochadexIV.html).
- Structuring simulation partitions for real-world use cases: [https://umbralcalc.github.io/posts/stochadexV.html](https://umbralcalc.github.io/posts/stochadexV.html).

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

You can add any new simulation partition you like by following the patterns for other processes given, e.g., in the `pkg/continuous` package.

- The main step is to create a new struct for your partition iterator which implements the `simulator.Iteration` interface.
- It is then strongly recommended that a test function for this new iterator is written, which should include a test that calls `simulator.RunWithHarnesses`.

## Using the analysis package

The `pkg/analysis` package provides tools for analysing simulation outputs and building new simulations on top of them. The plots work well within GoNB notebooks (notebooks with a Go-friendly Jupyter Kernel) and there are some simple examples of what you can do provided the `nbs/` folder. So take a look!

> In order to use the GoNB Jupyter Kernel, please install GoNB from here: [https://github.com/janpfeifer/gonb](https://github.com/janpfeifer/gonb).
