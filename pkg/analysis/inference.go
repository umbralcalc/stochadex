package analysis

import (
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// PosteriorLogNorm defines the configuration needed to specify
// the posterior log-normalisation in the AppliedPosteriorEstimation.
type PosteriorLogNorm struct {
	Name    string
	Default float64
}

// PosteriorMean defines the configuration needed to specify
// the posterior mean in the AppliedPosteriorEstimation.
type PosteriorMean struct {
	Name    string
	Default []float64
}

// PosteriorCovariance defines the configuration needed to specify
// the posterior covariance in the AppliedPosteriorEstimation.
//
// When JustVariance is true, Default has length N (per-dimension variance)
// and NewPosteriorEstimationPartitions wires the sampler to use
// variance_partition for the posterior output (not a dense covariance).
type PosteriorCovariance struct {
	Name         string
	Default      []float64
	JustVariance bool
}

// PosteriorSampler defines the configuration needed to specify
// the posterior sampler in the AppliedPosteriorEstimation.
type PosteriorSampler struct {
	Name         string
	Default      []float64
	Distribution ParameterisedModel
}

// AppliedPosteriorEstimation is the base configuration for an online
// inference of a simulation (specified by partition configs) from a
// referenced dataset.
//
// Windowed likelihood comparison: the embedded partition uses burn_in_steps
// equal to Window.Depth by default so the inner FromHistory replay has a
// full window before the first meaningful likelihood (earlier outer steps
// repeat the same inner log-likelihood, often 0, which can dominate
// PosteriorLogNormalisationIteration until history rolls). Override with
// Comparison.EmbeddedBurnInSteps. Optional Comparison.WindowDataHistoryDepth
// opts into a setup-time check that each Window.Data source partition’s
// StateHistoryDepth is at least Window.Depth.
type AppliedPosteriorEstimation struct {
	LogNorm      PosteriorLogNorm
	Mean         PosteriorMean
	Covariance   PosteriorCovariance
	Sampler      PosteriorSampler
	Comparison   AppliedLikelihoodComparison
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
	validateAppliedPosteriorWidths(applied)
	if applied.MemoryDepth < 1 {
		panic(fmt.Sprintf("analysis: MemoryDepth must be >= 1, got %d", applied.MemoryDepth))
	}
	compPartition := NewLikelihoodComparisonPartition(
		applied.Comparison,
		storage,
	)
	compPartition.StateHistoryDepth = applied.MemoryDepth
	loglikePartitions := []string{applied.Comparison.Name}
	paramPartitions := []string{applied.Sampler.Name}
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
		Name:      applied.LogNorm.Name,
		Iteration: &inference.PosteriorLogNormalisationIteration{},
		Params:    logNormParams,
		ParamsAsPartitions: map[string][]string{
			"loglike_partitions": loglikePartitions,
		},
		InitStateValues:   []float64{applied.LogNorm.Default},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	meanParams := simulator.NewParams(make(map[string][]float64))
	meanParams.Set("loglike_indices", loglikeIndices)
	partitions = append(partitions, &simulator.PartitionConfig{
		Name: applied.Mean.Name,
		Iteration: &inference.PosteriorMeanIteration{
			Transform: inference.MeanTransform,
		},
		Params: meanParams,
		ParamsAsPartitions: map[string][]string{
			"loglike_partitions": loglikePartitions,
			"param_partitions":   paramPartitions,
		},
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"posterior_log_normalisation": {Upstream: applied.LogNorm.Name},
		},
		InitStateValues:   applied.Mean.Default,
		StateHistoryDepth: 1,
		Seed:              0,
	})
	covParams := simulator.NewParams(make(map[string][]float64))
	covParams.Set("loglike_indices", loglikeIndices)
	if applied.Covariance.JustVariance {
		partitions = append(partitions, &simulator.PartitionConfig{
			Name: applied.Covariance.Name,
			Iteration: &inference.PosteriorMeanIteration{
				Transform: inference.VarianceTransform,
			},
			Params: covParams,
			ParamsAsPartitions: map[string][]string{
				"loglike_partitions": loglikePartitions,
				"param_partitions":   paramPartitions,
			},
			ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
				"posterior_log_normalisation": {Upstream: applied.LogNorm.Name},
				"mean":                        {Upstream: applied.Mean.Name},
			},
			InitStateValues:   applied.Covariance.Default,
			StateHistoryDepth: 1,
			Seed:              0,
		})
	} else {
		partitions = append(partitions, &simulator.PartitionConfig{
			Name:      applied.Covariance.Name,
			Iteration: &inference.PosteriorCovarianceIteration{},
			Params:    covParams,
			ParamsAsPartitions: map[string][]string{
				"loglike_partitions": loglikePartitions,
				"param_partitions":   paramPartitions,
			},
			ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
				"posterior_log_normalisation": {Upstream: applied.LogNorm.Name},
				"mean":                        {Upstream: applied.Mean.Name},
			},
			InitStateValues:   applied.Covariance.Default,
			StateHistoryDepth: 1,
			Seed:              0,
		})
	}
	applied.Sampler.Distribution.Init()
	if applied.Covariance.JustVariance {
		pas := make(map[string][]string)
		for k, v := range applied.Sampler.Distribution.ParamsAsPartitions {
			pas[k] = v
		}
		pas["variance_partition"] = []string{applied.Covariance.Name}
		applied.Sampler.Distribution.ParamsAsPartitions = pas
		pfu := make(map[string]simulator.NamedUpstreamConfig)
		for k, v := range applied.Sampler.Distribution.ParamsFromUpstream {
			if k == "covariance_matrix" {
				continue
			}
			pfu[k] = v
		}
		applied.Sampler.Distribution.ParamsFromUpstream = pfu
	}
	partitions = append(partitions, &simulator.PartitionConfig{
		Name: applied.Sampler.Name,
		Iteration: &inference.DataGenerationIteration{
			Likelihood: applied.Sampler.Distribution.Likelihood,
		},
		Params:             applied.Sampler.Distribution.Params,
		ParamsAsPartitions: applied.Sampler.Distribution.ParamsAsPartitions,
		ParamsFromUpstream: applied.Sampler.Distribution.ParamsFromUpstream,
		InitStateValues:    applied.Sampler.Default,
		StateHistoryDepth:  1,
		Seed:               applied.Seed,
	})
	partitions = append(partitions, compPartition)
	return partitions
}
