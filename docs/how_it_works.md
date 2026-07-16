---
title: "How it works"
logo: true
---

# How it works
<div style="height:0.75em;"></div>

## Interfaces and data types

The fundamental data types in the stochadex simulation engine are [Go](https://go.dev/) types which can be configured as [`Settings`](http://stochadex.github.io/pkg/simulator.html#Settings) (pure data) or [`Implementations`](http://stochadex.github.io/pkg/simulator.html#Implementations) (code which implements the provided interfaces).

<center><img src="https://pub-afdb1348ec964ca5b530aa758c0bdc56.r2.dev/assets/stochadex/stochadex-data-types.svg" /></center>

The key example among these is the [`Iteration`](http://stochadex.github.io/pkg/simulator.html#Iteration) interface.

<center><img src="https://pub-afdb1348ec964ca5b530aa758c0bdc56.r2.dev/assets/stochadex/fundamental-loop-code.svg" /></center>

The [`Iteration`](http://stochadex.github.io/pkg/simulator.html#Iteration) interface can be used to implement any simulation in practice.

To illustrate this point, we can show how the internal logic may be implemented to recreate some well-known stochastic processes.

## Example: Wiener process

For example, the [Wiener process](https://en.wikipedia.org/wiki/Wiener_process) has some very simple logic.

<center><img src="https://pub-afdb1348ec964ca5b530aa758c0bdc56.r2.dev/assets/stochadex/wiener-process.svg" /></center>

The two diagram boxes (`NewWienerProcessIncrement` and `AddToRecentState`) map directly to two helper steps inside `Iterate`:

```go
// X_{t+1} = X_t + sqrt(variance * dt) * Z,  Z ~ N(0, 1)
type WienerProcessIteration struct{ normal *distuv.Normal }

// NewWienerProcessIncrement: draw an i.i.d. Brownian increment per dimension.
func (w *WienerProcessIteration) NewWienerProcessIncrement(
	params *simulator.Params, width int, dt float64,
) []float64 {
	inc := make([]float64, width)
	for i := range inc {
		inc[i] = math.Sqrt(params.GetIndex("variances", i)*dt) * w.normal.Rand()
	}
	return inc
}

func (w *WienerProcessIteration) Iterate(
	params *simulator.Params, partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	h := stateHistories[partitionIndex]
	inc := w.NewWienerProcessIncrement(params, h.StateWidth, timestepsHistory.NextIncrement)
	// AddToRecentState: row 0 of the history is the most recent state.
	next := make([]float64, h.StateWidth)
	floats.AddTo(next, h.Values.RawRowView(0), inc)
	return next
}
```

## Example: Itô's lemma

It is well-known (especially by those in finance) that [Itô's lemma](https://en.wikipedia.org/wiki/It%C3%B4%27s_lemma) can be used to adapt the model formulae for a Wiener process after a mathematical function (a.k.a. transformation) has been applied to it. 

We can demonstrate that the [`Iteration`](http://stochadex.github.io/pkg/simulator.html#Iteration) interface can support this kind of transformation as well, through some more complicated logic.

<center><img src="https://pub-afdb1348ec964ca5b530aa758c0bdc56.r2.dev/assets/stochadex/ito-lemma.svg" /></center>

For `Y = g(X, t)` with `dX = sqrt(variance) dW`, Itô gives `dY = (DgDt + 0.5 * variance * D2gDx2) dt + DgDx * dW`. The diagram splits the iterate into the same three named pieces (derivatives, Wiener increment, then the Itô combination) and the code mirrors that one-to-one:

```go
type ItoLemmaIteration struct{ wiener WienerProcessIteration }

// GFunctionDerivatives: ∂g/∂t, ∂g/∂x, ∂²g/∂x² evaluated at the current state.
// Replace the body with whichever transformation g is being applied.
func (i *ItoLemmaIteration) GFunctionDerivatives(
	x []float64, t float64,
) (DgDt, DgDx, D2gDx2 []float64) { /* ... */ }

// ItoLemmaFunction: combine derivatives + Wiener increment into the Y increment.
func (i *ItoLemmaIteration) ItoLemmaFunction(
	DgDt, DgDx, D2gDx2, wienerInc, variances []float64, dt float64,
) []float64 {
	inc := make([]float64, len(DgDt))
	for j := range inc {
		inc[j] = (DgDt[j]+0.5*variances[j]*D2gDx2[j])*dt + DgDx[j]*wienerInc[j]
	}
	return inc
}

func (i *ItoLemmaIteration) Iterate(
	params *simulator.Params, partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	h := stateHistories[partitionIndex]
	dt := timestepsHistory.NextIncrement
	state := h.Values.RawRowView(0)
	DgDt, DgDx, D2gDx2 := i.GFunctionDerivatives(state, timestepsHistory.Values.AtVec(0))
	wienerInc := i.wiener.NewWienerProcessIncrement(params, h.StateWidth, dt)
	inc := i.ItoLemmaFunction(DgDt, DgDx, D2gDx2, wienerInc, params.Get("variances"), dt)
	next := make([]float64, h.StateWidth)
	floats.AddTo(next, state, inc) // AddToRecentState
	return next
}
```

## Example: Time-inhomogeneous Poisson process

What about event-based processes?

The time-inhomogeneous [Poisson process](https://en.wikipedia.org/wiki/Poisson_point_process), is an example of an event-based process which counts the cumulative number of events in time, while its event rate varies.

The [`Iteration`](http://stochadex.github.io/pkg/simulator.html#Iteration) interface can support this too.

<center><img src="https://pub-afdb1348ec964ca5b530aa758c0bdc56.r2.dev/assets/stochadex/inhomogeneous-poisson.svg" /></center>

The two diagram boxes (`EventRateLambdaFunction` and `DrawNewEventIncrement`) make the time-varying rate explicit. The rate function reads the current time directly from the timesteps history, which is what makes the process inhomogeneous:

```go
type InhomogeneousPoissonIteration struct{ uniform *distuv.Uniform }

// EventRateLambdaFunction: the per-step rate λ(t) which is here, a sinusoidal example.
func (p *InhomogeneousPoissonIteration) EventRateLambdaFunction(
	params *simulator.Params, t float64, width int,
) []float64 {
	lambda := make([]float64, width)
	for i := range lambda {
		lambda[i] = params.GetIndex("baseline", i) +
			params.GetIndex("amplitude", i)*math.Sin(t)
	}
	return lambda
}

// DrawNewEventIncrement: small-dt Bernoulli arrival per dimension.
func (p *InhomogeneousPoissonIteration) DrawNewEventIncrement(
	lambda []float64, dt float64,
) []float64 {
	inc := make([]float64, len(lambda))
	for i := range inc {
		if p.uniform.Rand() < lambda[i]*dt {
			inc[i] = 1.0
		}
	}
	return inc
}

func (p *InhomogeneousPoissonIteration) Iterate(
	params *simulator.Params, partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	h := stateHistories[partitionIndex]
	lambda := p.EventRateLambdaFunction(
		params, timestepsHistory.Values.AtVec(0), h.StateWidth)
	inc := p.DrawNewEventIncrement(lambda, timestepsHistory.NextIncrement)
	next := make([]float64, h.StateWidth)
	floats.AddTo(next, h.Values.RawRowView(0), inc) // AddToRecentState
	return next
}
```

## Example: Hawkes process

Let's give one more example.

The [Hawkes process](https://en.wikipedia.org/wiki/Hawkes_process) couples the history of events to the current event rate. One may therefore categorise this process as 'non-Markovian'.

This means that the Hawkes process needs a memory of the state partition history in order to calculate its next state values. 

The [`Iteration`](http://stochadex.github.io/pkg/simulator.html#Iteration) interface obviously supports this quite easily through accessing its own [`StateHistory`](http://stochadex.github.io/pkg/simulator.html#StateHistory).

<center><img src="https://pub-afdb1348ec964ca5b530aa758c0bdc56.r2.dev/assets/stochadex/hawkes-process.svg" /></center>

Compared with the inhomogeneous Poisson, the only diagram box that changes is the rate computation: `ExcitingKernel` now sweeps the partition's own state history, accumulating a kernel-weighted contribution from each past event into the current `LambdaValue`. The `DrawNewEventIncrement` and `AddToRecentState` boxes are reused unchanged:

```go
type HawkesProcessIteration struct{ uniform *distuv.Uniform }

// ExcitingKernel: λ(t) = μ + Σ_k kernel(t - t_k) over past events read from
// the partition's own StateHistory (this is what makes Hawkes non-Markovian).
func (h *HawkesProcessIteration) ExcitingKernel(
	params *simulator.Params,
	history *simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	lambda := params.GetCopy("background_rates") // μ
	now := timestepsHistory.Values.AtVec(0)
	decay := params.GetIndex("decay", 0)
	for k := 1; k < history.StateHistoryDepth; k++ {
		// Number of new events at past row k = state[k-1] - state[k].
		dEvents := make([]float64, history.StateWidth)
		floats.SubTo(dEvents, history.Values.RawRowView(k-1), history.Values.RawRowView(k))
		w := math.Exp(-(now - timestepsHistory.Values.AtVec(k)) / decay)
		floats.AddScaled(lambda, w, dEvents)
	}
	return lambda
}

func (h *HawkesProcessIteration) Iterate(
	params *simulator.Params, partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	hist := stateHistories[partitionIndex]
	lambda := h.ExcitingKernel(params, hist, timestepsHistory)
	// Reuses the same DrawNewEventIncrement step as the Poisson example.
	inc := drawNewEventIncrement(h.uniform, lambda, timestepsHistory.NextIncrement)
	next := make([]float64, hist.StateWidth)
	floats.AddTo(next, hist.Values.RawRowView(0), inc)
	return next
}
```

## Serial dependency graphs and modularity

Multiple [`Iteration`](http://stochadex.github.io/pkg/simulator.html#Iteration)s can run within the stochadex for each step in time. In order to construct serial dependency graphs between them, we can utilise upstream-downstream [`Iteration`](http://stochadex.github.io/pkg/simulator.html#Iteration) relationships.

<center><img src="https://pub-afdb1348ec964ca5b530aa758c0bdc56.r2.dev/assets/stochadex/stochadex-parallel-serial.svg" /></center>

Structuring groups of [`Iteration`](http://stochadex.github.io/pkg/simulator.html#Iteration)s in this way can increase modularity. For example, the time-inhomogeneous Poisson process can be implemented serially.

<center><img src="https://pub-afdb1348ec964ca5b530aa758c0bdc56.r2.dev/assets/stochadex/inhomo-poisson-parallel-serial.svg" /></center>

The single iteration above splits into two partitions: a stateless rate computation (Partition A) and a generic constant-rate Poisson sampler (Partition B) wired to A via `params_from_upstream`. The Poisson sampler no longer needs to know what the rate is; it just consumes whatever the upstream produces:

```yaml
# rate_function: produces λ(t) into its single state slot per step.
# poisson_sampler: a plain Poisson process whose `rates` param is replaced,
# every step, by the most recent state of `rate_function`.
iterations:
  - name: rate_function
    init_state_values: [0.0]
    state_width: 1
    state_history_depth: 1
    # Iteration: any iteration that emits the desired λ(t), e.g. a sinusoid.
  - name: poisson_sampler
    params_from_upstream:
      rates:
        upstream: 0   # index of rate_function
    init_state_values: [0.0]
    state_width: 1
    state_history_depth: 1
    # Iteration: &discrete.PoissonProcessIteration{}
```

This is exactly the relationship used by [`CoxProcessIteration`](http://stochadex.github.io/pkg/discrete.html#CoxProcessIteration); when the upstream rate is itself stochastic, the same wiring becomes a Cox (doubly stochastic) process for free.

## Simulation loop and embedded simulation runs

The simulation run loop coordinates the serial relationships between [`Iteration`](http://stochadex.github.io/pkg/simulator.html#Iteration)s while maximising concurrency in execution of each [`Iteration`](http://stochadex.github.io/pkg/simulator.html#Iteration) per step in time.

<center><img src="https://pub-afdb1348ec964ca5b530aa758c0bdc56.r2.dev/assets/stochadex/stochadex-loop.svg" /></center>

Given that this loop always runs for any stochadex simulation, it is sufficient to describe any simulation uniquely through the dependency diagram between its [`Iteration`](http://stochadex.github.io/pkg/simulator.html#Iteration)s.

Given the simulation run loop, we are also able to implement an [`EmbeddedSimulationRunIteration`](https://stochadex.github.io/pkg/general.html#EmbeddedSimulationRunIteration).

The [`EmbeddedSimulationRunIteration`](https://stochadex.github.io/pkg/general.html#EmbeddedSimulationRunIteration) is a special [`Iteration`](http://stochadex.github.io/pkg/simulator.html#Iteration) which performs entire simulation runs from start to end for every timestep and outputs the end state as its next state values. Its presence can make the diagrams a little more complex, but way more flexible in application.

<center><img src="https://pub-afdb1348ec964ca5b530aa758c0bdc56.r2.dev/assets/stochadex/embedded-simulations.svg" /></center>

Concretely, the inner simulation is just another [`Settings`](http://stochadex.github.io/pkg/simulator.html#Settings) + [`Implementations`](http://stochadex.github.io/pkg/simulator.html#Implementations) pair. The embedded iteration owns its own [`PartitionCoordinator`](http://stochadex.github.io/pkg/simulator.html#PartitionCoordinator), runs it to termination on every outer step, and returns the concatenated final state as its row:

```go
// Inner simulation; runs to its own TerminationCondition once per outer step.
innerSettings := simulator.LoadSettingsFromYaml("./inner_settings.yaml")
innerImpls := &simulator.Implementations{
	Iterations: []simulator.Iteration{
		&continuous.WienerProcessIteration{},
	},
	OutputCondition:      &simulator.NilOutputCondition{},
	OutputFunction:       &simulator.NilOutputFunction{},
	TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 50},
	TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 0.1},
}

// Use it like any other Iteration in the outer simulation.
outerEmbedded := general.NewEmbeddedSimulationRunIteration(innerSettings, innerImpls)
```

Composing [`Iteration`](http://stochadex.github.io/pkg/simulator.html#Iteration)s together and [`EmbeddedSimulationRunIteration`](https://stochadex.github.io/pkg/general.html#EmbeddedSimulationRunIteration)s unlocks a huge universe of possible simulation algorithms. Here are just a few examples.

## Example: Probabilistic sample weighting

This algorithm estimates historically-weighted statistics and uses them to construct a probabilistic comparison.

<center><img src="https://pub-afdb1348ec964ca5b530aa758c0bdc56.r2.dev/assets/stochadex/prob-reweighting-code.svg" /></center>

Each diagram box is a partition stacked over a shared [`StateTimeStorage`](http://stochadex.github.io/pkg/simulator.html#StateTimeStorage): a data source feeds an exponentially-weighted rolling mean, which feeds an exponentially-weighted rolling covariance. The kernel choice is what makes the statistics 'probabilistic'; it controls how strongly past data is reweighted at each step:

```go
// `data` is loaded from CSV (the .DataGeneration box in the diagram).
storage, _ := analysis.NewStateTimeStorageFromCsv(
	"data.csv", 0, map[string][]int{"data": {1, 2, 3, 4}}, true,
)

// .ConditionalProbability + .ComputeReweightedMean to give an exponentially-weighted mean.
mean := analysis.NewVectorMeanPartition(analysis.AppliedAggregation{
	Name:   "mean",
	Data:   analysis.DataRef{PartitionName: "data"},
	Kernel: &kernels.ExponentialIntegrationKernel{},
}, storage)
mean.Params.Set("exponential_weighting_timescale", []float64{100.0})

// .ComputeReweightedCovariance using the same kernel, conditioned on the mean partition.
cov := analysis.NewVectorVariancePartition(
	analysis.DataRef{PartitionName: "mean"},
	analysis.AppliedAggregation{
		Name:   "cov",
		Data:   analysis.DataRef{PartitionName: "data"},
		Kernel: &kernels.ExponentialIntegrationKernel{},
	},
	storage,
)
cov.Params.Set("exponential_weighting_timescale", []float64{100.0})

storage = analysis.AddPartitionsToStateTimeStorage(
	storage,
	[]*simulator.PartitionConfig{mean, cov},
	map[string]int{"data": 200, "mean": 1},
)
```

## Example: Online simulation parameter estimation

This algorithm uses a sequence of probabilities (typically estimated by the algorithm above) to estimate the posterior probabilities of simulation parameters.

<center><img src="https://pub-afdb1348ec964ca5b530aa758c0bdc56.r2.dev/assets/stochadex/simulation-inference-code.svg"/></center>

The diagram's 'Embedded Simulation' block (`IterateSimulation` + `IterateFromHistory` + `DataComparison`) is exactly an [`EmbeddedSimulationRunIteration`](https://stochadex.github.io/pkg/general.html#EmbeddedSimulationRunIteration) configured to replay rolling statistics over a window of history; the outer block (`ComputePosteriorParams` + `SamplePosterior`) is the posterior log-norm / mean / covariance / sampler chain. [`NewPosteriorEstimationPartitions`](http://stochadex.github.io/pkg/analysis.html#NewPosteriorEstimationPartitions) wires all of this together from a single spec:

```go
// `model` is the parameterised likelihood; `simPartition` is the inner-sim
// PartitionConfig whose params are perturbed by the sampler each outer step.
partitions := analysis.NewPosteriorEstimationPartitions(
	analysis.AppliedPosteriorEstimation{
		LogNorm:    analysis.PosteriorLogNorm{Name: "log_norm", Default: 0.0},
		Mean:       analysis.PosteriorMean{Name: "post_mean", Default: []float64{0, 0}},
		Covariance: analysis.PosteriorCovariance{Name: "post_cov", Default: []float64{1, 0, 0, 1}},
		Sampler: analysis.PosteriorSampler{
			Name: "sampler", Default: []float64{0, 0}, Distribution: model,
		},
		Comparison: analysis.AppliedLikelihoodComparison{
			Name:  "loglike",
			Model: model,
			Data:  analysis.DataRef{PartitionName: "obs"},
			Window: analysis.WindowedPartitions{
				Partitions: []analysis.WindowedPartition{{
					Partition: simPartition,
					// .IterateSimulation reads its mean param from the sampler which
					// closes the loop between proposed params and likelihood evaluation.
					OutsideUpstreams: map[string]simulator.NamedUpstreamConfig{
						"mean": {Upstream: "sampler"},
					},
				}},
				Depth: 100,
			},
		},
		PastDiscount: 1.0,  // .ComputePosteriorParams discount on past loglikes
		MemoryDepth:  200,
		Seed:         1234,
	},
	storage,
)
```

## Example: Optimising with evolutionary strategies

The [evolutionary strategies](https://en.wikipedia.org/wiki/Evolution_strategy) algorithm can be applied to search future simulation trajectories to find the best set of policy parameters needed to achieve some discounted future reward.

This algorithm relies on sorting the sampled simulation trajectories according to their discounted future rewards and then using the top fraction of these to update the best known policy parameters (and the variance around them) after each timestep.

<center><img src="https://pub-afdb1348ec964ca5b530aa758c0bdc56.r2.dev/assets/stochadex/discounted-return-optimiser-code.svg"/></center>

Each diagram box becomes a named partition in the spec passed to [`NewEvolutionStrategyOptimisationPartitions`](http://stochadex.github.io/pkg/analysis.html#NewEvolutionStrategyOptimisationPartitions): the embedded simulation accumulates `UpdateDiscountedReturn`, `SortPolicyByReturn` keeps a top-k collection, and `UpdateBestPolicy` / `UpdateCovariance` move the sampling distribution toward the winners. The `rewardCfg` and `simCfg` below are user-supplied [`PartitionConfig`](http://stochadex.github.io/pkg/simulator.html#PartitionConfig)s for the reward and inner-simulation iterations:

```go
partitions := analysis.NewEvolutionStrategyOptimisationPartitions(
	analysis.AppliedEvolutionStrategyOptimisation{
		// .SamplePolicyParams: draws candidate policies from the running Gaussian.
		Sampler: analysis.EvolutionStrategySampler{
			Name: "sampler", Default: []float64{0, 0},
		},
		// .SortPolicyByReturn: keeps the top-k by discounted return.
		Sorting: analysis.EvolutionStrategySorting{
			Name: "sorted", CollectionSize: 10, EmptyValue: -1e9,
		},
		// .UpdateBestPolicy + .UpdateCovariance: weighted update of the running stats.
		Mean: analysis.EvolutionStrategyMean{
			Name: "best", Default: []float64{0, 0},
			Weights: []float64{0.5, 0.3, 0.2}, LearningRate: 0.5,
		},
		Covariance: analysis.EvolutionStrategyCovariance{
			Name: "cov", Default: []float64{4, 0, 0, 4}, LearningRate: 0.3,
		},
		// .UpdateDiscountedReturn: sums per-step rewards across the embedded run.
		Reward: analysis.EvolutionStrategyReward{
			Partition: analysis.WindowedPartition{
				Partition: rewardCfg,
				OutsideUpstreams: map[string]simulator.NamedUpstreamConfig{
					"sample_values": {Upstream: "sampler"},
				},
			},
			DiscountFactor: 0.9,
		},
		// .IterateSimulation: the inner trajectory the rewards are computed against.
		Window: analysis.WindowedPartitions{
			Partitions: []analysis.WindowedPartition{{Partition: simCfg}},
			Depth:      5,
		},
		Seed: 12345,
	},
	nil,
)
```