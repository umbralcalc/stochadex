package analysis

import (
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

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
	Names        PosteriorEstimationNames
	Comparison   AppliedLikelihoodComparison
	Defaults     PosteriorDefaults
	PastDiscount float64
	MemoryDepth  int
	Seed         uint64
}

// NewPosteriorEstimationPartitions creates a set of PartitionConfigs for
// an online posterior estimation process using rolling statistics.
func NewPosteriorEstimationPartitions(
	applied AppliedPosteriorEstimation,
	storage *simulator.StateTimeStorage,
) []*simulator.PartitionConfig {
	compPartition := NewLikelihoodComparisonPartition(
		applied.Comparison,
		storage,
	)
	compPartition.StateHistoryDepth = applied.MemoryDepth
	loglikePartitions := []string{applied.Comparison.Name}
	paramPartitions := []string{applied.Names.Sampler}
	loglikeIndices := []float64{
		float64(len(compPartition.InitStateValues) - 1)}
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
		Name: applied.Names.Mean,
		Iteration: &inference.PosteriorMeanIteration{
			Transform: inference.MeanTransform,
		},
		Params: meanParams,
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
	samplerParams.Set(
		"cov_burn_in_steps",
		[]float64{float64(applied.MemoryDepth)},
	)
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
	partitions = append(partitions, compPartition)
	return partitions
}
