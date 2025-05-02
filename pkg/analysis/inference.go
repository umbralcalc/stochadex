package analysis

import (
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/kernels"
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

// ParameterisedTKernel defines the configuration for a t-kernel
// density estimation model applied to a referenced dataset with
// its corresponding parameters to set.
type ParameterisedTKernel struct {
	Data              DataRef
	Depth             int
	DegreesOfFreedom  float64
	ScaleMatrixValues []float64
	TimeDeltaRanges   []general.TimeDeltaRange
}

// AppliedTKernelComparison is the base configuration for a rolling
// comparison between a referenced dataset and kernel density estimation
// model applied to another referenced dataset.
type AppliedTKernelComparison struct {
	Name   string
	Model  ParameterisedTKernel
	Data   DataRef
	Window WindowedPartitions
}

// NewTKernelComparison creates a new PartitionConfig for a rolling
// comparison between a referenced dataset and kernel density estimation
// model applied to another referenced dataset.
func NewAppliedTKernelComparisonPartition(
	applied AppliedTKernelComparison,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	generator := simulator.NewConfigGenerator()
	generator.SetSimulation(&simulator.SimulationConfig{
		OutputCondition: &simulator.NilOutputCondition{},
		OutputFunction:  &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: applied.Window.Depth,
		},
		TimestepFunction: &general.FromHistoryTimestepFunction{
			InitStepsTaken: applied.Model.Depth,
		},
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
				Name: ref.PartitionName,
				Iteration: &general.FromHistoryIteration{
					InitStepsTaken: applied.Model.Depth - 1,
				},
				Params:            simulator.NewParams(make(map[string][]float64)),
				InitStateValues:   initStateValues,
				StateHistoryDepth: applied.Model.Depth,
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
	for _, timeDeltaRange := range applied.Model.TimeDeltaRanges {
		generator.SetPartition(&simulator.PartitionConfig{
			Name: "comparison" + fmt.Sprintf(
				"_%f_%f", timeDeltaRange.LowerDelta, timeDeltaRange.UpperDelta),
			Iteration: &general.CumulativeIteration{
				Iteration: &general.ValuesFunctionVectorSumIteration{
					Function:       general.UnitValueFunction,
					Kernel:         &kernels.TDistributionStateIntegrationKernel{},
					TimeDeltaRange: &timeDeltaRange,
				},
			},
			Params: simulator.NewParams(map[string][]float64{
				"degrees_of_freedom": {applied.Model.DegreesOfFreedom},
				"scale_matrix":       applied.Model.ScaleMatrixValues,
			}),
			ParamsAsPartitions: map[string][]string{
				"data_values_partition": {applied.Model.Data.PartitionName},
			},
			ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
				"latest_data_values": {
					Upstream: applied.Data.PartitionName,
					Indices:  applied.Data.ValueIndices,
				},
			},
			InitStateValues:   []float64{0.0},
			StateHistoryDepth: 1,
			Seed:              0,
		})
	}
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

// PosteriorTKernelEstimationNames is a collection of the names given to
// partitions used in the AppliedPosteriorTKernelEstimation.
type PosteriorTKernelEstimationNames struct {
	Updater string
	Sampler string
}

// PosteriorTKernelDefaults is a collection of the defaults given to
// partitions used in the AppliedPosteriorTKernelEstimation.
type PosteriorTKernelDefaults struct {
	Updater []float64
	Sampler []float64
}

// AppliedPosteriorTKernelEstimation is the base configuration for an
// online inference of a simulation (specified by partition configs)
// from a referenced dataset using t-distribution kernel densities.
type AppliedPosteriorTKernelEstimation struct {
	Names         PosteriorTKernelEstimationNames
	Comparison    AppliedTKernelComparison
	Defaults      PosteriorTKernelDefaults
	ResamplingCov []float64
	PastDiscount  float64
	MemoryDepth   int
	Seed          uint64
}

// NewPosteriorTKernelEstimationPartitions creates a set of PartitionConfigs
// for an online posterior estimation process using t-distribution kernel
// densities.
func NewPosteriorTKernelEstimationPartitions(
	applied AppliedPosteriorTKernelEstimation,
	storage *simulator.StateTimeStorage,
) []*simulator.PartitionConfig {
	compPartition := NewAppliedTKernelComparisonPartition(
		applied.Comparison,
		storage,
	)
	compPartition.StateHistoryDepth = applied.MemoryDepth
	partitions := make([]*simulator.PartitionConfig, 0)
	loglikeIndices := make([]float64, 0)
	loglikePartitions := make([]string, 0)
	for i, timeDeltaRange := range applied.Comparison.Model.TimeDeltaRanges {
		// TODO: Still missing the scale matrix updates to the compPartition config...
		partitions = append(partitions, &simulator.PartitionConfig{
			Name: applied.Names.Updater + fmt.Sprintf(
				"_%f_%f", timeDeltaRange.LowerDelta, timeDeltaRange.UpperDelta),
			Iteration: &inference.PosteriorKernelUpdateIteration{
				TimeDeltaRange: &timeDeltaRange,
			},
			Params: simulator.NewParams(map[string][]float64{
				"past_discounting_factor": {applied.PastDiscount},
			}),
			ParamsAsPartitions: map[string][]string{
				"data_values_partition": {applied.Comparison.Data.PartitionName},
			},
			ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
				"latest_data_values": {
					Upstream: applied.Comparison.Data.PartitionName,
				},
			},
			InitStateValues:   applied.Defaults.Updater,
			StateHistoryDepth: 1,
			Seed:              0,
		})
		loglikePartitions = append(loglikePartitions, applied.Comparison.Name)
		loglikeIndices = append(
			loglikeIndices, float64(len(compPartition.InitStateValues)-i-1))
	}
	partitions = append(partitions, &simulator.PartitionConfig{
		Name:      applied.Names.Sampler,
		Iteration: &inference.PosteriorImportanceResampleIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"past_discounting_factor": {applied.PastDiscount},
			"loglike_indices":         loglikeIndices,
			"sample_covariance":       applied.ResamplingCov,
		}),
		ParamsAsPartitions: map[string][]string{
			"loglike_partitions": loglikePartitions,
			"param_partitions":   {applied.Comparison.Data.PartitionName},
		},
		InitStateValues:   applied.Defaults.Sampler,
		StateHistoryDepth: 1,
		Seed:              applied.Seed,
	})
	partitions = append(partitions, compPartition)
	return partitions
}
