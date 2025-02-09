package analysis

import (
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// WindowedPartitions defines a windowed history of data from
// partitions in storage and possible additional partitions to include
// when simulating the window duration.
type WindowedPartitions struct {
	Partitions []*simulator.PartitionConfig
	Data       []DataRef
	Depth      int
}

// ParameterisedModel defines a likelihood model for the data with its
// corresponding parameters to set.
type ParameterisedModel struct {
	Likelihood         inference.LikelihoodDistribution
	Params             simulator.Params
	ParamsAsPartitions map[string][]string
	ParamsFromUpstream map[string]simulator.NamedUpstreamConfig
}

// Init populates the model parameter fields if they have not been set.
func (p *ParameterisedModel) Init() {
	if p.ParamsAsPartitions == nil {
		p.ParamsAsPartitions = make(map[string][]string)
	}
	if p.ParamsFromUpstream == nil {
		p.ParamsFromUpstream =
			make(map[string]simulator.NamedUpstreamConfig)
	}
}

// AppliedLikelihoodComparison is the base configuration for a rolling
// comparison between a referenced dataset and referenced likelihood model.
type AppliedLikelihoodComparison struct {
	Name   string
	Model  ParameterisedModel
	Data   DataRef
	Window WindowedPartitions
}

// NewLikelihoodComparisonPartition creates a new PartitionConfig for
// a rolling likelihood comparison.
func NewLikelihoodComparisonPartition(
	applied AppliedLikelihoodComparison,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	generator := simulator.NewConfigGenerator()
	generator.SetSimulation(&simulator.SimulationConfig{
		OutputCondition: &simulator.NilOutputCondition{},
		OutputFunction:  &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: applied.Window.Depth,
		},
		// These will be overwritten with the times in the data...
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:    0.0,
	})
	if applied.Window.Partitions != nil {
		for _, partition := range applied.Window.Partitions {
			generator.SetPartition(partition)
		}
	}
	simInitStateValues := make([]float64, 0)
	simParamsAsPartitions := make(map[string][]string)
	simParamsFromUpstream := make(map[string]simulator.NamedUpstreamConfig)
	for _, ref := range applied.Window.Data {
		initStateValues := ref.GetIndexFromStorage(storage, 0)
		generator.SetPartition(&simulator.PartitionConfig{
			Name:              ref.PartitionName,
			Iteration:         &general.FromHistoryIteration{},
			Params:            simulator.NewParams(make(map[string][]float64)),
			InitStateValues:   initStateValues,
			StateHistoryDepth: 1,
			Seed:              0,
		})
		simInitStateValues = append(simInitStateValues, initStateValues...)
		simParamsAsPartitions[ref.PartitionName+"/state_memory_partition"] =
			[]string{ref.PartitionName}
		simParamsFromUpstream[ref.PartitionName+"/latest_data_values"] =
			simulator.NamedUpstreamConfig{
				Upstream: ref.PartitionName,
				Indices:  ref.ValueIndices,
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
	simParams := simulator.NewParams(map[string][]float64{
		"burn_in_steps": {float64(applied.Window.Depth)},
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

// PosteriorEstimationNames is a collection of the names given to
// partitions used in the AppliedPosteriorEstimation.
type PosteriorEstimationNames struct {
	LogNorm    string
	Mean       string
	Covariance string
	Sampler    string
}

// PosteriorDefaults is a collection of the defaults given to
// partitions used in the AppliedPosteriorEstimation.
type PosteriorDefaults struct {
	LogNorm    float64
	Mean       []float64
	Covariance []float64
	Sampler    []float64
}

// AppliedPosteriorEstimation is the base configuration for an online
// inference of a simulation (specified by partition configs) from a
// referenced dataset.
type AppliedPosteriorEstimation struct {
	Names         PosteriorEstimationNames
	Defaults      PosteriorDefaults
	LikelihoodRef DataRef
	PastDiscount  float64
	Seed          uint64
}

// NewPosteriorEstimationPartitions creates a set of PartitionConfigs for
// an online posterior estimation process using rolling statistics.
func NewPosteriorEstimationPartitions(
	applied AppliedPosteriorEstimation,
) []*simulator.PartitionConfig {
	loglikeIndices := make([]float64, 0)
	loglikePartitions := make([]string, 0)
	paramPartitions := make([]string, 0)
	if applied.LikelihoodRef.ValueIndices == nil {
		panic("must set LikelihoodRef.ValueIndices to use posterior estimation")
	}
	for _, index := range applied.LikelihoodRef.ValueIndices {
		loglikeIndices = append(loglikeIndices, float64(index))
		loglikePartitions = append(
			loglikePartitions,
			applied.LikelihoodRef.PartitionName,
		)
		paramPartitions = append(
			paramPartitions,
			applied.Names.Sampler,
		)
	}
	partitions := make([]*simulator.PartitionConfig, 0)
	logNormParams := simulator.NewParams(make(map[string][]float64))
	logNormParams.Set("loglike_indices", loglikeIndices)
	logNormParams.Set(
		"past_discounting_factor",
		[]float64{applied.PastDiscount},
	)
	partitions = append(partitions, &simulator.PartitionConfig{
		Name:      applied.Names.LogNorm,
		Iteration: &inference.PosteriorLogNormalisationIteration{},
		Params:    logNormParams,
		ParamsAsPartitions: map[string][]string{
			"loglike_partitions": loglikePartitions,
		},
		InitStateValues:   []float64{applied.Defaults.LogNorm},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	meanParams := simulator.NewParams(make(map[string][]float64))
	meanParams.Set("loglike_indices", loglikeIndices)
	partitions = append(partitions, &simulator.PartitionConfig{
		Name:      applied.Names.Mean,
		Iteration: &inference.PosteriorMeanIteration{},
		Params:    meanParams,
		ParamsAsPartitions: map[string][]string{
			"loglike_partitions": loglikePartitions,
			"param_partitions":   paramPartitions,
		},
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"posterior_log_normalisation": {Upstream: applied.Names.LogNorm},
		},
		InitStateValues:   applied.Defaults.Mean,
		StateHistoryDepth: 1,
		Seed:              0,
	})
	covParams := simulator.NewParams(make(map[string][]float64))
	covParams.Set("loglike_indices", loglikeIndices)
	partitions = append(partitions, &simulator.PartitionConfig{
		Name:      applied.Names.Covariance,
		Iteration: &inference.PosteriorCovarianceIteration{},
		Params:    covParams,
		ParamsAsPartitions: map[string][]string{
			"loglike_partitions": loglikePartitions,
			"param_partitions":   paramPartitions,
		},
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"posterior_log_normalisation": {Upstream: applied.Names.LogNorm},
			"mean":                        {Upstream: applied.Names.Mean},
		},
		InitStateValues:   applied.Defaults.Covariance,
		StateHistoryDepth: 1,
		Seed:              0,
	})
	samplerParams := simulator.NewParams(make(map[string][]float64))
	samplerParams.Set("default_covariance", applied.Defaults.Covariance)
	partitions = append(partitions, &simulator.PartitionConfig{
		Name: applied.Names.Sampler,
		Iteration: &inference.DataGenerationIteration{
			Likelihood: &inference.NormalLikelihoodDistribution{},
		},
		Params: samplerParams,
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"mean":              {Upstream: applied.Names.Mean},
			"covariance_matrix": {Upstream: applied.Names.Covariance},
		},
		InitStateValues:   applied.Defaults.Sampler,
		StateHistoryDepth: 1,
		Seed:              applied.Seed,
	})
	return partitions
}
