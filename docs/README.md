---
title: "Home"
---

<img src="./assets/logo.svg" width=600/>

<div class="badges"><a href="https://github.com/umbralcalc/stochadex/releases"><img src="https://img.shields.io/github/v/tag/umbralcalc/stochadex?style=for-the-badge&amp;label=version&amp;color=4D7F37" alt="Latest version" /></a> <a href="https://github.com/umbralcalc/stochadex/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/umbralcalc/stochadex/ci.yml?branch=main&amp;style=for-the-badge&amp;label=CI" alt="CI" /></a> <a href="https://codecov.io/gh/umbralcalc/stochadex"><img src="https://img.shields.io/codecov/c/github/umbralcalc/stochadex?style=for-the-badge&amp;label=coverage" alt="Test coverage" /></a> <a href="https://github.com/umbralcalc/stochadex"><img src="https://img.shields.io/badge/github-%23121011.svg?style=for-the-badge&amp;logo=github&amp;logoColor=white" alt="Github" /></a> <a href="https://github.com/umbralcalc/stochadex/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg?style=for-the-badge" alt="MIT" /></a> <a href="https://go.dev/"><img src="https://img.shields.io/badge/go-%2300ADD8.svg?style=for-the-badge&logo=go&logoColor=white" alt="Go" /></a></div>
<div style="height:0.75em;"></div>

## So what is the 'stochadex' project?

It's a simulation engine written in [Go](https://go.dev/) which can be used to sample from, and learn computational models for, a whole 'Pokédex' of possible real-world systems.

For software engineers, the stochadex simulation framework abstracts away many of the common features that sampling algorithms have for performing these computations behind a highly-configurable interface.

This simulation engine is designed based on the simulation software fundamentals described in [this collection of blog posts](https://umbralcalc.github.io/posts/simulating_real_world_systems_as_a_programmer_introduction.html).

## When to use it

The stochadex fits best when you're in [Go](https://go.dev/) and want stochastic simulation and online inference or simulation-based decision-making (like MCTS) together over one composable primitive and a single deployable binary. This is a combination no other Go library offers (to our knowledge).

It's a powerful framework with tons of features, really generalisable abstractions and principled design. However, you should probably reach for something else when:

- **Large fixed-shape, GPU, or autodiff-heavy Bayesian modelling** → [Stan](https://mc-stan.org/), [PyMC](https://www.pymc.io/), or Julia's [SciML](https://sciml.ai/).
- **Pure discrete-event simulation** (entities through queues and servers) → [godes](https://github.com/agoussia/godes).
- **Plain numerics or classical ML in Go** → [gonum](https://github.com/gonum/gonum), which the stochadex is built on.
- **Training neural networks or deep RL** → train in Python, then import a frozen ONNX/TorchScript model to run inference behind an `Iteration`.

## Projects using it

- [Event-based rugby match simulations to evaluate manager decision-making](https://github.com/umbralcalc/trywizard)
- [Fish ecosystem simulations using environment data to evaluate sustanability policies](https://github.com/umbralcalc/anglersim)
- [Antimicrobial resistance (AMR) stewardship simulations to evaluate hospital guidelines](https://github.com/umbralcalc/antimicrobial-resistance)
- [Stochastic simulations of catchment-scale flood dynamics under climate change](https://github.com/umbralcalc/floodrisk)
- [Energy storage dispatch simulation with demand response optimisation](https://github.com/umbralcalc/energy-balancer)
- [Planning approval policies for affordability with housing market simulations](https://github.com/umbralcalc/homark)
- [Small business survival and support policy simulation](https://github.com/umbralcalc/business-survival)
- [18xx gameplay design tool and Monte Carlo Tree Search (MCTS) agents](https://github.com/umbralcalc/18xxdesigner)
- [Turning simulations into in-browser interactive dashboards](https://github.com/umbralcalc/dexetera)
- [Inferring structural causal models from open source datasets](https://github.com/umbralcalc/openaction2outcome)
- [Seasonal bathing water pollution exceedance forecasts](https://github.com/umbralcalc/bathing-water-forecaster)

## Other cool engines

- [SimPy](https://gitlab.com/team-simpy/simpy/)
- [StoSpa](https://github.com/BartoszBartmanski/StoSpa)
- [FLAME GPU](https://github.com/FLAMEGPU/FLAMEGPU2/)
