# So what is the 'stochadex'?

A generalised simulation engine to generate samples from and statistically infer a 'Pokédex' of possible stochastic processes. A 'Pokédex' here is just a fanciful description for a very general class of multidimensional stochastic processes that pop up everywhere in taming the mathematical wilds of real-world phenomena, and which also leads to a name for the software: the _stochadex_. With such a thing pre-built and self-contained, it can become the basis upon which to build generalised software solutions for a lot of different interesting problems - happy days!

The point, from a software engineering perspective, is to design something which abstracts away many of the common features that sampling algorithms have for performing these computations behind a highly-configurable interface. This isn't particularly new as a concept (see, e.g., [SimPy](https://gitlab.com/team-simpy/simpy/), [StoSpa](https://github.com/BartoszBartmanski/StoSpa), [FLAME GPU](https://github.com/FLAMEGPU/FLAMEGPU2/) and loads more), however the design provides a mathematical formalism to reference in future projects, and, to be honest, writing the code from scratch has just been a lot of fun in Go!

## Need more context and documentation?

- The original simulation engine motivation and design article: [https://umbralcalc.github.io/posts/stochadexI.html](https://umbralcalc.github.io/posts/stochadexI.html).
- An article demonstrating how to implement probabilistic learning methods with the stochadex: [https://umbralcalc.github.io/posts/stochadexII.html](https://umbralcalc.github.io/posts/stochadexII.html).
- An article demonstrating how to adaptively infer simulation parameters with the stochadex: [https://umbralcalc.github.io/posts/stochadexIII.html](https://umbralcalc.github.io/posts/stochadexIII.html).
- An article demonstrating how to optimise agent interactions with simulations with the stochadex: [https://umbralcalc.github.io/posts/stochadexIV.html](https://umbralcalc.github.io/posts/stochadexIV.html).

## Building and running the binary

```shell
# update the go modules
go mod tidy

# build the binary
go build -o bin/ ./cmd/stochadex

# run your config with the dashboard off
./bin/stochadex --config ./cfg/config.yaml
```

## Building and running the real-time dashboard

```shell
# install the dependencies of and build the app
cd ./app && npm install && npm run build && cd ..

# run the stochadex with a dashboard config and checkout http://localhost:3000
./bin/stochadex --config ./cfg/config.yaml \
--dashboard ./cfg/dashboard_config.yaml
```

![Using Dashboard](app/public/using-dashboard.gif)

## Building and running the Docker container (may need sudo)

```shell
# build the stochadex container
docker build --tag stochadex .

# run the binary in the container with your configs
docker run -p 2112:2112 stochadex --config ./cfg/config.yaml \
--dashboard ./cfg/dashboard_config.yaml
```

## Developing the code and real-time dashboard

You can add any new stochastic phenomena you like by following the patterns for other processes given in the `pkg/phenomena` package. The key step is to create a new struct for your process which implements the `simulator.Iteration` interface.

To develop the real-time dashboard, you can start the development server by running `cd ./app && npm start && cd ..` and view the code in the `app/` directory. The dashboard is a React app which is served by the stochadex via a websocket connection.
