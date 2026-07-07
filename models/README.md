# Domain-models catalogue

This catalogue enables downstream stochadex applications to 'teach' the engine what good domain models look like, and recurring needs surface for promotion into the core.

## What each entry is

The repo boundary follows the **generative / inferential split**. This engine owns the
**forward model** — the thing that *simulates*. Downstream project repos own inference,
data ingestion, calibration, and the decision layer. Each catalogue entry is therefore
three artifacts:

1. **Methodology card** (`card.md`) — the primary human- and agent-legible description:
   the real-world system, what it ingests, its assumptions, validity regime, failure
   modes, and the question it answers. Because the stub is Go (not a declarative YAML
   file), the card carries the structural spec.
2. **SDK-based, data-free simulation stub** (`stub.go` + `*_test.go`) — the generative
   core only, built via the stochadex SDK (`Settings` + `Implementations`), wired into
   this engine's CI with at least one *meaningful* assertion about generative behaviour
   (a conservation property or the correct direction of a parameter response), not merely
   "it runs."
3. **Downstream pointer** — a link, in the card, to the project repo where inference,
   data, and the decision layer live.

Any **bespoke `simulator.Iteration` implementations** a model needs sit *beside* its stub
(e.g. `colonisation.go`). The catalogue is the staging ground for the "should this be in
core?" question — an extension that recurs across several models, doing substantially the
same job, is signalling it wants promoting. That mechanism is deliberately not designed up
front; it emerges from the recurrence.

## Entries

| Model | Real-world system | Downstream |
|---|---|---|
| [antimicrobial-resistance](antimicrobial-resistance/card.md) | Hospital cephalosporin resistance: two-strain colonisation → bloodstream infection under prescribing pressure | [repo](https://github.com/umbralcalc/antimicrobial-resistance) |
| [floodrisk](floodrisk/card.md) | Catchment flood dynamics: stochastic rainfall → rainfall-runoff cascade → river peak flow under climate perturbation | [repo](https://github.com/umbralcalc/floodrisk) |
| [energy-balancer](energy-balancer/card.md) | GB grid balancing: mean-reverting residual demand → co-moving imbalance price + carbon intensity → price- vs carbon-threshold battery dispatch under rising renewable intermittency | [repo](https://github.com/umbralcalc/energy-balancer) |
| [business-survival](business-survival/card.md) | Local-authority business demography: monthly sector×age Leslie register under formation, ONS-derived exit hazards, macro covariates and support-policy multipliers → register stock + five-year cohort survival | [repo](https://github.com/umbralcalc/business-survival) |
| [trywizard](trywizard/card.md) | Rugby match dynamics: coupled Cox counting processes for tries/penalties/cards driven by a log-linear (Poisson-GLM) rate model with substitution covariates → scoreline + home win probability under substitution timing | [repo](https://github.com/umbralcalc/trywizard) |
| [anglersim](anglersim/card.md) | Brown trout population dynamics: mean-reverting flow/temperature/dissolved-oxygen forcing + a warming trend → stochastic Ricker log-density under climate perturbation and flow/water-quality management levers | [repo](https://github.com/umbralcalc/anglersim) |
| [bathing-water-forecaster](bathing-water-forecaster/card.md) | Bathing-water pollution exceedances: a shared regional Ornstein–Uhlenbeck "wet-week" anomaly coupled to many designated sites → per-site latent log-concentration → E. coli exceedance probability, cohering across the coastline under weather variability and coupling strength | [repo](https://github.com/umbralcalc/bathing-water-forecaster) |

See [`CONVENTIONS.md`](CONVENTIONS.md) for the format each entry follows and how to add
one. New entries adopt the format from birth; existing downstream models are catalogued
deliberately (flagships) or opportunistically (the long tail), never by forced retrofit.
