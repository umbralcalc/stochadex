# So what is the 'stochadex'?

A generalised simulation engine to generate samples from and statistically infer a 'Pokédex' of possible stochastic processes. A 'Pokédex' here is just a fanciful description for a very general class of multidimensional stochastic processes that pop up everywhere in taming the mathematical wilds of real-world phenomena, and which also leads to a name for the software: the _stochadex_. With such a thing pre-built and self-contained, it can become the basis upon which to build generalised software solutions for a lot of different interesting problems - happy days!

The point, from a software engineering perspective, is to design something which abstracts away many of the common features that sampling algorithms have for performing these computations behind a highly-configurable interface. This isn't particularly new as a concept (see, e.g., [SimPy](https://gitlab.com/team-simpy/simpy/), [StoSpa](https://github.com/BartoszBartmanski/StoSpa), [FLAME GPU](https://github.com/FLAMEGPU/FLAMEGPU2/) and loads more), however the design provides a mathematical formalism to reference in future projects, and, to be honest, writing the code from scratch has just been a lot of fun in Go!

## Need more context and documentation?

- The original simulation engine motivation and design article: [https://umbralcalc.github.io/posts/stochadexI.html](https://umbralcalc.github.io/posts/stochadexI.html).
- An article demonstrating how to implement probabilistic learning methods with the stochadex: [https://umbralcalc.github.io/posts/stochadexII.html](https://umbralcalc.github.io/posts/stochadexII.html).
- An article demonstrating how to adaptively infer simulation parameters with the stochadex: [https://umbralcalc.github.io/posts/stochadexIII.html](https://umbralcalc.github.io/posts/stochadexIII.html).
- An article demonstrating how to structure simulations for real-world use cases: [https://umbralcalc.github.io/posts/stochadexIV.html](https://umbralcalc.github.io/posts/stochadexIV.html).
- An article demonstrating how to optimise agent interactions with the stochadex: [https://umbralcalc.github.io/posts/stochadexV.html](https://umbralcalc.github.io/posts/stochadexV.html).

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

You can add any new stochastic phenomena you like by following the patterns for other processes given, e.g., in the `pkg/continuous` package. The key step is to create a new struct for your process which implements the `simulator.Iteration` interface.
