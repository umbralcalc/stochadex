package macros

// This file builds rolling likelihood windows, posterior partitions, and
// storage-backed replays. Windowed likelihoods embed a short simulation
// whose length is Window.Depth; outer burn_in_steps on that embedding default
// to Window.Depth so inner timesteps align with available history (see
// EmbeddedBurnInSteps to decouple). Use ValidateWindowDataHistoryDepth
// with analysis.AddPartitionsToStateTimeStorage window sizes to fail fast if
// history is too shallow for FromHistoryIteration — and note that too deep is
// just as wrong (it replays zeros), which is why it warns.

import (
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// WindowedPartition configures a partition that participates in a finite
// windowed simulation.
//
// Usage hints:
//   - Partition defines the inner partition and its params.
//   - OutsideUpstreams map allows wiring upstreams from outside the window.
type WindowedPartition struct {
	Partition        *simulator.PartitionConfig
	OutsideUpstreams map[string]simulator.NamedUpstreamConfig
}

// WindowedPartitions defines the sliding-window context used by analysis.
//
// Usage hints:
//   - Partitions are simulated inside the window.
//   - Data references supply historical values to seed and drive the window.
//   - Depth is the number of steps in the window.
type WindowedPartitions struct {
	Partitions []WindowedPartition
	Data       []analysis.DataRef
	Depth      int
}

// ParameterisedModel bundles a likelihood distribution with its parameter
// configuration and any cross-partition parameter wiring required at runtime.
type ParameterisedModel struct {
	Likelihood         inference.LikelihoodDistribution
	Params             simulator.Params
	ParamsAsPartitions map[string][]string
	ParamsFromUpstream map[string]simulator.NamedUpstreamConfig
}

// Init ensures the params and internal parameter wiring maps are initialised.
func (p *ParameterisedModel) Init() {
	// A model declared with no params at all (e.g. one taking its mean from
	// upstream) leaves Params.Map nil, and the constructors below Set into it.
	if p.Params.Map == nil {
		p.Params = simulator.NewParams(make(map[string][]float64))
	}
	if p.ParamsAsPartitions == nil {
		p.ParamsAsPartitions = make(map[string][]string)
	}
	if p.ParamsFromUpstream == nil {
		p.ParamsFromUpstream =
			make(map[string]simulator.NamedUpstreamConfig)
	}
}

// AppliedLikelihoodComparison configures a rolling likelihood comparison
// between referenced data and a model over a sliding window.
type AppliedLikelihoodComparison struct {
	Name   string
	Model  ParameterisedModel
	Data   analysis.DataRef
	Window WindowedPartitions
	// EmbeddedBurnInSteps, if non-nil, sets outer burn-in steps before the
	// embedded window runs (see general.EmbeddedSimulationRunIteration). If
	// nil, defaults to Window.Depth so the inner replay starts with a full window.
	EmbeddedBurnInSteps *int
	// WindowDataHistoryDepth, if non-nil, must map every Window.Data
	// PartitionName to that partition’s StateHistoryDepth in the outer
	// simulation; each value must equal Window.Depth. Smaller panics, and
	// so does larger, which would silently void the likelihood (see
	// assertWindowDataHistoryNotTooDeep).
	WindowDataHistoryDepth map[string]int
}

// NewLikelihoodComparisonPartition builds a PartitionConfig embedding an
// inner windowed simulation to evaluate the likelihood over a rolling window,
// producing a per-step comparison score.
func NewLikelihoodComparisonPartition(
	applied AppliedLikelihoodComparison,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	if applied.Window.Depth < 1 {
		panic(fmt.Sprintf("macros: Window.Depth must be >= 1, got %d", applied.Window.Depth))
	}
	assertWindowDataSourcesDeepEnough(
		applied.Window.Depth,
		applied.Window.Data,
		applied.WindowDataHistoryDepth,
	)
	generator := simulator.NewConfigGenerator()
	generator.SetSimulation(&simulator.SimulationConfig{
		OutputCondition: &simulator.NilOutputCondition{},
		OutputFunction:  &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: applied.Window.Depth,
		},
		TimestepFunction: &general.FromHistoryTimestepFunction{},
		// This will be overwritten with the times in the data...
		InitTimeValue: 0.0,
	})
	burnIn := windowBurnIn(applied.Window.Depth, applied.EmbeddedBurnInSteps)
	layout := applied.Window.layout(generator, storage, burnIn)
	applied.Model.Init()
	generator.SetPartition(layout.AddComparison(applied.Model.Params.Map,
		applied.Model.ParamsAsPartitions, applied.Model.ParamsFromUpstream,
		AggregationRef{Partition: applied.Data.PartitionName, Indices: applied.Data.ValueIndices},
	).partition("comparison", &inference.DataComparisonIteration{Likelihood: applied.Model.Likelihood}))
	return layout.host(applied.Name, generator, 1)
}

// ParameterisedModelWithGradient augments ParameterisedModel with gradient
// support for optimisation routines.
type ParameterisedModelWithGradient struct {
	Likelihood         inference.LikelihoodDistributionWithGradient
	Params             simulator.Params
	ParamsAsPartitions map[string][]string
	ParamsFromUpstream map[string]simulator.NamedUpstreamConfig
}

// Init ensures the params and internal parameter wiring maps are initialised.
func (p *ParameterisedModelWithGradient) Init() {
	if p.Params.Map == nil {
		p.Params = simulator.NewParams(make(map[string][]float64))
	}
	if p.ParamsAsPartitions == nil {
		p.ParamsAsPartitions = make(map[string][]string)
	}
	if p.ParamsFromUpstream == nil {
		p.ParamsFromUpstream =
			make(map[string]simulator.NamedUpstreamConfig)
	}
}

// LikelihoodMeanGradient specifies a function mapping params and the gradient
// of the likelihood mean to a parameter update direction.
type LikelihoodMeanGradient struct {
	Function func(
		params *simulator.Params,
		likeMeanGrad []float64,
	) []float64
	Width int
}

// AppliedLikelihoodMeanFunctionFit configures online fitting of the model's
// likelihood mean to data using a gradient function and learning rate over a
// finite descent schedule.
type AppliedLikelihoodMeanFunctionFit struct {
	Name     string
	Model    ParameterisedModelWithGradient
	Gradient LikelihoodMeanGradient
	Data     analysis.DataRef
	Window   WindowedPartitions
	// EmbeddedBurnInSteps mirrors AppliedLikelihoodComparison.EmbeddedBurnInSteps.
	EmbeddedBurnInSteps *int
	// WindowDataHistoryDepth mirrors AppliedLikelihoodComparison.WindowDataHistoryDepth.
	WindowDataHistoryDepth map[string]int
	LearningRate           float64
	DescentIterations      int
	// WarmStart, if true, seeds each outer step's inner gradient descent from
	// the previous outer step's output rather than from a fixed param. Enables
	// convergence to a global MLE over the full data window as outer steps
	// accumulate (standard online SGD behaviour).
	WarmStart bool
}

// NewLikelihoodMeanFunctionFitPartition builds a PartitionConfig embedding an
// inner simulation that runs gradient descent to fit the likelihood mean to
// the referenced data window.
func NewLikelihoodMeanFunctionFitPartition(
	applied AppliedLikelihoodMeanFunctionFit,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	if applied.Window.Depth < 1 {
		panic(fmt.Sprintf("macros: Window.Depth must be >= 1, got %d", applied.Window.Depth))
	}
	assertWindowDataSourcesDeepEnough(
		applied.Window.Depth,
		applied.Window.Data,
		applied.WindowDataHistoryDepth,
	)
	generator := simulator.NewConfigGenerator()
	generator.SetSimulation(&simulator.SimulationConfig{
		OutputCondition: &simulator.NilOutputCondition{},
		OutputFunction:  &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: applied.DescentIterations,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:    0.0,
	})
	burnIn := windowBurnIn(applied.Window.Depth, applied.EmbeddedBurnInSteps)
	layout := applied.Window.layout(generator, storage, burnIn)
	applied.Model.Init()
	gradient, descent := layout.AddMeanFit(applied.Model.Params.Map, applied.Model.ParamsAsPartitions,
		applied.Model.ParamsFromUpstream, applied.Data.PartitionName, applied.Gradient.Width,
		applied.LearningRate, applied.WarmStart)
	generator.SetPartition(gradient.partition("gradient", &inference.DataComparisonGradientIteration{
		Likelihood:   applied.Model.Likelihood,
		GradientFunc: applied.Gradient.Function,
	}))
	generator.SetPartition(descent.partition("gradient_descent", &continuous.GradientDescentIteration{}))
	return layout.host(applied.Name, generator, applied.Window.Depth)
}

// windowBurnIn is a window's burn-in: its depth unless set.
func windowBurnIn(depth int, set *int) int {
	burnIn := depth
	if set != nil {
		burnIn = *set
	}
	if burnIn < 0 {
		panic(fmt.Sprintf("macros: EmbeddedBurnInSteps must be >= 0, got %d", burnIn))
	}
	return burnIn
}

// layout adds the window's partitions and data replays to generator, in the order
// its layout lays them out, and returns that layout.
func (w WindowedPartitions) layout(
	generator *simulator.ConfigGenerator,
	storage *simulator.StateTimeStorage,
	burnIn int,
) WindowLayout {
	refs := make([]WindowPartitionRef, len(w.Partitions))
	for i, partition := range w.Partitions {
		generator.SetPartition(partition.Partition)
		refs[i] = WindowPartitionRef{Name: partition.Partition.Name,
			InitState: partition.Partition.InitStateValues, OutsideUpstreams: partition.OutsideUpstreams}
	}
	replays := make([]WindowReplay, len(w.Data))
	for i, ref := range w.Data {
		if ref.ValueIndices != nil {
			panic("value indices are not supported in window data")
		}
		replays[i] = WindowReplay{Name: ref.PartitionName, InitState: ref.GetTimeIndexFromStorage(storage, 0)}
		generator.SetPartition(&simulator.PartitionConfig{
			Name:              ref.PartitionName,
			Iteration:         &general.FromHistoryIteration{},
			Params:            simulator.NewParams(make(map[string][]float64)),
			InitStateValues:   replays[i].InitState,
			StateHistoryDepth: 1,
		})
	}
	return NewWindowLayout(refs, replays, burnIn)
}

// host is the partition running the window generator holds, wired by the layout.
func (l WindowLayout) host(name string, generator *simulator.ConfigGenerator, historyDepth int) *simulator.PartitionConfig {
	return &simulator.PartitionConfig{
		Name: name,
		Iteration: general.NewEmbeddedSimulationRunIteration(
			withInlineExecution(generator.GenerateConfigs()),
		),
		Params:             simulator.NewParams(l.Params),
		ParamsAsPartitions: l.ParamsAsPartitions,
		ParamsFromUpstream: l.ParamsFromUpstream,
		InitStateValues:    l.InitStateValues,
		StateHistoryDepth:  historyDepth,
	}
}

// withInlineExecution runs a window's embedded simulation inline when its
// partitions allow: every within-step upstream earlier in partition order, which
// the constructors guarantee for the partitions they add (data replays first,
// then the comparison or gradient reading them) but not for the window's own
// partitions, listed as the config lists them. The window re-runs Depth inner
// steps every outer step, so the default strategy's goroutine spawn and channel
// round-trip per partition per inner step dominated its cost. Output is
// identical under every strategy (see simulator.ExecutionStrategy); a window
// listed out of order keeps the default rather than tripping InlineExecution's
// ordering panic.
func withInlineExecution(
	settings *simulator.Settings,
	implementations *simulator.Implementations,
) (*simulator.Settings, *simulator.Implementations) {
	for index, iteration := range settings.Iterations {
		for _, upstream := range iteration.ParamsFromUpstream {
			if upstream.Upstream >= index {
				return settings, implementations
			}
		}
	}
	implementations.ExecutionStrategy = &simulator.InlineExecution{}
	return settings, implementations
}
