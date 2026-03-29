// Package analysis builds rolling likelihood windows, posterior partitions,
// and storage-backed replays. Windowed likelihoods embed a short simulation
// whose length is Window.Depth; outer burn_in_steps on that embedding default
// to Window.Depth so inner timesteps align with available history (see
// EmbeddedBurnInSteps to decouple). Use ValidateWindowDataHistoryDepth with
// AddPartitionsToStateTimeStorage window sizes to fail fast if history is
// too shallow for FromHistoryIteration.
package analysis

import (
	"fmt"

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
	Data       []DataRef
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

// Init ensures internal parameter wiring maps are initialised.
func (p *ParameterisedModel) Init() {
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
	Data   DataRef
	Window WindowedPartitions
	// EmbeddedBurnInSteps, if non-nil, sets outer burn-in steps before the
	// embedded window runs (see general.EmbeddedSimulationRunIteration). If
	// nil, defaults to Window.Depth so the inner replay starts with a full window.
	EmbeddedBurnInSteps *int
	// WindowDataHistoryDepth, if non-nil, must map every Window.Data
	// PartitionName to that partition’s StateHistoryDepth in the outer
	// simulation; each value must be >= Window.Depth.
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
		panic(fmt.Sprintf("analysis: Window.Depth must be >= 1, got %d", applied.Window.Depth))
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
	simInitStateValues := make([]float64, 0)
	simParamsFromUpstream := make(map[string]simulator.NamedUpstreamConfig)
	if applied.Window.Partitions != nil {
		for _, partition := range applied.Window.Partitions {
			generator.SetPartition(partition.Partition)
			simInitStateValues = append(
				simInitStateValues,
				partition.Partition.InitStateValues...,
			)
			if partition.OutsideUpstreams == nil {
				continue
			}
			for paramsName, upstream := range partition.OutsideUpstreams {
				simParamsFromUpstream[partition.Partition.Name+
					"/"+paramsName] = upstream
			}
		}
	}
	simParamsAsPartitions := make(map[string][]string)
	if applied.Window.Data != nil {
		for _, ref := range applied.Window.Data {
			if ref.ValueIndices != nil {
				panic("value indices are not supported in window data")
			}
			initStateValues := ref.GetTimeIndexFromStorage(storage, 0)
			generator.SetPartition(&simulator.PartitionConfig{
				Name:              ref.PartitionName,
				Iteration:         &general.FromHistoryIteration{},
				Params:            simulator.NewParams(make(map[string][]float64)),
				InitStateValues:   initStateValues,
				StateHistoryDepth: 1,
				Seed:              0,
			})
			simInitStateValues = append(simInitStateValues, initStateValues...)
			simParamsAsPartitions[ref.PartitionName+
				"/update_from_partition_history"] = []string{ref.PartitionName}
			simParamsAsPartitions[ref.PartitionName+
				"/initial_state_from_partition_history"] = []string{ref.PartitionName}
			simParamsFromUpstream[ref.PartitionName+"/latest_data_values"] =
				simulator.NamedUpstreamConfig{Upstream: ref.PartitionName}
		}
	}
	applied.Model.Init()
	applied.Model.Params.Set("cumulative", []float64{1})
	applied.Model.Params.Set("burn_in_steps", []float64{0})
	applied.Model.ParamsFromUpstream["latest_data_values"] =
		simulator.NamedUpstreamConfig{
			Upstream: applied.Data.PartitionName,
			Indices:  applied.Data.ValueIndices,
		}
	generator.SetPartition(&simulator.PartitionConfig{
		Name: "comparison",
		Iteration: &inference.DataComparisonIteration{
			Likelihood: applied.Model.Likelihood,
		},
		Params:             applied.Model.Params,
		ParamsAsPartitions: applied.Model.ParamsAsPartitions,
		ParamsFromUpstream: applied.Model.ParamsFromUpstream,
		InitStateValues:    []float64{0.0},
		StateHistoryDepth:  1,
		Seed:               0,
	})
	simInitStateValues = append(simInitStateValues, 0.0)
	burnIn := applied.Window.Depth
	if applied.EmbeddedBurnInSteps != nil {
		burnIn = *applied.EmbeddedBurnInSteps
	}
	if burnIn < 0 {
		panic(fmt.Sprintf("analysis: EmbeddedBurnInSteps must be >= 0, got %d", burnIn))
	}
	simParams := simulator.NewParams(map[string][]float64{
		"burn_in_steps": {float64(burnIn)},
	})
	return &simulator.PartitionConfig{
		Name: applied.Name,
		Iteration: general.NewEmbeddedSimulationRunIteration(
			generator.GenerateConfigs(),
		),
		Params:             simParams,
		ParamsAsPartitions: simParamsAsPartitions,
		ParamsFromUpstream: simParamsFromUpstream,
		InitStateValues:    simInitStateValues,
		StateHistoryDepth:  1,
		Seed:               0,
	}
}

// ParameterisedModelWithGradient augments ParameterisedModel with gradient
// support for optimisation routines.
type ParameterisedModelWithGradient struct {
	Likelihood         inference.LikelihoodDistributionWithGradient
	Params             simulator.Params
	ParamsAsPartitions map[string][]string
	ParamsFromUpstream map[string]simulator.NamedUpstreamConfig
}

// Init ensures internal parameter wiring maps are initialised.
func (p *ParameterisedModelWithGradient) Init() {
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
	Data     DataRef
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
		panic(fmt.Sprintf("analysis: Window.Depth must be >= 1, got %d", applied.Window.Depth))
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
	simInitStateValues := make([]float64, 0)
	simParamsFromUpstream := make(map[string]simulator.NamedUpstreamConfig)
	if applied.Window.Partitions != nil {
		for _, partition := range applied.Window.Partitions {
			generator.SetPartition(partition.Partition)
			simInitStateValues = append(
				simInitStateValues,
				partition.Partition.InitStateValues...,
			)
			if partition.OutsideUpstreams == nil {
				continue
			}
			for paramsName, upstream := range partition.OutsideUpstreams {
				simParamsFromUpstream[partition.Partition.Name+
					"/"+paramsName] = upstream
			}
		}
	}
	simParamsAsPartitions := make(map[string][]string)
	if applied.Window.Data != nil {
		for _, ref := range applied.Window.Data {
			if ref.ValueIndices != nil {
				panic("value indices are not supported in window data")
			}
			initStateValues := ref.GetTimeIndexFromStorage(storage, 0)
			generator.SetPartition(&simulator.PartitionConfig{
				Name:              ref.PartitionName,
				Iteration:         &general.FromHistoryIteration{},
				Params:            simulator.NewParams(make(map[string][]float64)),
				InitStateValues:   initStateValues,
				StateHistoryDepth: 1,
				Seed:              0,
			})
			simInitStateValues = append(simInitStateValues, initStateValues...)
			simParamsAsPartitions[ref.PartitionName+
				"/update_from_partition_history"] = []string{ref.PartitionName}
			simParamsAsPartitions[ref.PartitionName+
				"/initial_state_from_partition_history"] = []string{ref.PartitionName}
			simParamsFromUpstream[ref.PartitionName+"/latest_data_values"] =
				simulator.NamedUpstreamConfig{Upstream: ref.PartitionName}
		}
	}
	applied.Model.Init()
	applied.Model.Params.Set(applied.Data.PartitionName+"->data_values", []float64{})
	applied.Model.ParamsAsPartitions["mean_partition"] = []string{"gradient_descent"}
	generator.SetPartition(&simulator.PartitionConfig{
		Name: "gradient",
		Iteration: &inference.DataComparisonGradientIteration{
			Likelihood:   applied.Model.Likelihood,
			GradientFunc: applied.Gradient.Function,
		},
		Params:             applied.Model.Params,
		ParamsAsPartitions: applied.Model.ParamsAsPartitions,
		ParamsFromUpstream: applied.Model.ParamsFromUpstream,
		InitStateValues:    make([]float64, applied.Gradient.Width),
		StateHistoryDepth:  1,
		Seed:               0,
	})
	simParamsAsPartitions["gradient/update_from_partition_history"] =
		[]string{applied.Data.PartitionName}
	simParamsFromUpstream["gradient/target_state"] =
		simulator.NamedUpstreamConfig{Upstream: applied.Data.PartitionName}
	simInitStateValues = append(
		simInitStateValues, make([]float64, applied.Gradient.Width)...)
	gradientDescentOffset := len(simInitStateValues)
	gradientDescentParams := simulator.NewParams(make(map[string][]float64))
	gradientDescentParams.Set("learning_rate", []float64{applied.LearningRate})
	generator.SetPartition(&simulator.PartitionConfig{
		Name:      "gradient_descent",
		Iteration: &continuous.GradientDescentIteration{},
		Params:    gradientDescentParams,
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"gradient": {Upstream: "gradient"},
		},
		InitStateValues:   make([]float64, applied.Gradient.Width),
		StateHistoryDepth: 1,
		Seed:              0,
	})
	simInitStateValues = append(
		simInitStateValues, make([]float64, applied.Gradient.Width)...)
	burnInFit := applied.Window.Depth
	if applied.EmbeddedBurnInSteps != nil {
		burnInFit = *applied.EmbeddedBurnInSteps
	}
	if burnInFit < 0 {
		panic(fmt.Sprintf("analysis: EmbeddedBurnInSteps must be >= 0, got %d", burnInFit))
	}
	simParams := simulator.NewParams(map[string][]float64{
		"burn_in_steps": {float64(burnInFit)},
	})
	if applied.WarmStart {
		simParams.Set(
			"gradient_descent/init_state_values_from_outer",
			[]float64{float64(gradientDescentOffset), float64(applied.Gradient.Width)},
		)
	}
	return &simulator.PartitionConfig{
		Name: applied.Name,
		Iteration: general.NewEmbeddedSimulationRunIteration(
			generator.GenerateConfigs(),
		),
		Params:             simParams,
		ParamsAsPartitions: simParamsAsPartitions,
		ParamsFromUpstream: simParamsFromUpstream,
		InitStateValues:    simInitStateValues,
		StateHistoryDepth:  applied.Window.Depth,
		Seed:               0,
	}
}
