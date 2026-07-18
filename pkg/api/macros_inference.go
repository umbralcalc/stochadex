package api

import (
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// The heavier inference/stats macros. Their AppliedX inputs nest deeply (a
// ParameterisedModel, a WindowedPartitions holding inner partition configs), and
// are expressed as typed spec structs decoded straight from YAML by MacroConfig —
// which both handles the nesting and preserves scalar types (see the macros.go
// note on the `y`->true coercion).

// dataRefSpec is the data form of analysis.DataRef.
type dataRefSpec struct {
	PartitionName string `yaml:"partition_name"`
	ValueIndices  []int  `yaml:"value_indices,omitempty"`
}

func (s dataRefSpec) resolve() analysis.DataRef {
	return analysis.DataRef{PartitionName: s.PartitionName, ValueIndices: s.ValueIndices}
}

func resolveDataRefs(specs []dataRefSpec) []analysis.DataRef {
	refs := make([]analysis.DataRef, len(specs))
	for i, spec := range specs {
		refs[i] = spec.resolve()
	}
	return refs
}

// parameterisedModelSpec is the data form of analysis.ParameterisedModel: a
// likelihood spec plus its parameter wiring.
type parameterisedModelSpec struct {
	Likelihood         simulator.ComponentSpec                  `yaml:"likelihood"`
	Params             map[string][]float64                     `yaml:"params,omitempty"`
	ParamsAsPartitions map[string][]string                      `yaml:"params_as_partitions,omitempty"`
	ParamsFromUpstream map[string]simulator.NamedUpstreamConfig `yaml:"params_from_upstream,omitempty"`
}

func (s parameterisedModelSpec) resolve() (analysis.ParameterisedModel, error) {
	likelihood, err := resolveLikelihood(s.Likelihood)
	if err != nil {
		return analysis.ParameterisedModel{}, err
	}
	return analysis.ParameterisedModel{
		Likelihood:         likelihood,
		Params:             simulator.NewParams(s.Params),
		ParamsAsPartitions: s.ParamsAsPartitions,
		ParamsFromUpstream: s.ParamsFromUpstream,
	}, nil
}

// windowedPartitionSpec / windowedPartitionsSpec are the data form of the sliding
// window: inner partitions (each a full data-spec PartitionConfig) plus the data
// references that drive them.
type windowedPartitionSpec struct {
	Partition        simulator.PartitionConfig                `yaml:"partition"`
	OutsideUpstreams map[string]simulator.NamedUpstreamConfig `yaml:"outside_upstreams,omitempty"`
}

type windowedPartitionsSpec struct {
	Partitions []windowedPartitionSpec `yaml:"partitions,omitempty"`
	Data       []dataRefSpec           `yaml:"data,omitempty"`
	Depth      int                     `yaml:"depth"`
}

func (s windowedPartitionsSpec) resolve() (analysis.WindowedPartitions, error) {
	windowed := analysis.WindowedPartitions{Depth: s.Depth, Data: resolveDataRefs(s.Data)}
	for i := range s.Partitions {
		partition := s.Partitions[i].Partition
		partition.Init()
		if partition.IterationSpec.IsData() {
			iteration, err := ResolveIteration(partition.IterationSpec)
			if err != nil {
				return windowed, fmt.Errorf("window partition %q: %w", partition.Name, err)
			}
			partition.Iteration = iteration
		}
		windowed.Partitions = append(windowed.Partitions, analysis.WindowedPartition{
			Partition:        &partition,
			OutsideUpstreams: s.Partitions[i].OutsideUpstreams,
		})
	}
	return windowed, nil
}

// likelihoodComparisonSpec is the data form of analysis.AppliedLikelihoodComparison.
type likelihoodComparisonSpec struct {
	macroTypeField         `yaml:",inline"`
	Name                   string                 `yaml:"name"`
	Model                  parameterisedModelSpec `yaml:"model"`
	Data                   dataRefSpec            `yaml:"data"`
	Window                 windowedPartitionsSpec `yaml:"window"`
	EmbeddedBurnInSteps    *int                   `yaml:"embedded_burn_in_steps,omitempty"`
	WindowDataHistoryDepth map[string]int         `yaml:"window_data_history_depth,omitempty"`
}

func (s likelihoodComparisonSpec) resolveApplied() (analysis.AppliedLikelihoodComparison, error) {
	model, err := s.Model.resolve()
	if err != nil {
		return analysis.AppliedLikelihoodComparison{}, err
	}
	window, err := s.Window.resolve()
	if err != nil {
		return analysis.AppliedLikelihoodComparison{}, err
	}
	return analysis.AppliedLikelihoodComparison{
		Name:                   s.Name,
		Model:                  model,
		Data:                   s.Data.resolve(),
		Window:                 window,
		EmbeddedBurnInSteps:    s.EmbeddedBurnInSteps,
		WindowDataHistoryDepth: s.WindowDataHistoryDepth,
	}, nil
}

func (s *likelihoodComparisonSpec) resolve(
	storage *simulator.StateTimeStorage,
) ([]*simulator.PartitionConfig, map[string]int, error) {
	applied, err := s.resolveApplied()
	if err != nil {
		return nil, nil, err
	}
	partition := analysis.NewLikelihoodComparisonPartition(applied, storage)
	return []*simulator.PartitionConfig{partition}, applied.WindowDataHistoryDepth, nil
}

// posteriorEstimationSpec is the data form of analysis.AppliedPosteriorEstimation.
type posteriorEstimationSpec struct {
	macroTypeField `yaml:",inline"`
	LogNorm        struct {
		Name    string  `yaml:"name"`
		Default float64 `yaml:"default"`
	} `yaml:"log_norm"`
	Mean struct {
		Name    string    `yaml:"name"`
		Default []float64 `yaml:"default"`
	} `yaml:"mean"`
	Covariance struct {
		Name         string    `yaml:"name"`
		Default      []float64 `yaml:"default"`
		JustVariance bool      `yaml:"just_variance,omitempty"`
	} `yaml:"covariance"`
	Sampler struct {
		Name         string                 `yaml:"name"`
		Default      []float64              `yaml:"default"`
		Distribution parameterisedModelSpec `yaml:"distribution"`
	} `yaml:"sampler"`
	Comparison   likelihoodComparisonSpec `yaml:"comparison"`
	PastDiscount float64                  `yaml:"past_discount"`
	MemoryDepth  int                      `yaml:"memory_depth"`
	Seed         uint64                   `yaml:"seed"`
}

func (s posteriorEstimationSpec) resolveApplied() (analysis.AppliedPosteriorEstimation, error) {
	distribution, err := s.Sampler.Distribution.resolve()
	if err != nil {
		return analysis.AppliedPosteriorEstimation{}, err
	}
	comparison, err := s.Comparison.resolveApplied()
	if err != nil {
		return analysis.AppliedPosteriorEstimation{}, err
	}
	return analysis.AppliedPosteriorEstimation{
		LogNorm:    analysis.PosteriorLogNorm{Name: s.LogNorm.Name, Default: s.LogNorm.Default},
		Mean:       analysis.PosteriorMean{Name: s.Mean.Name, Default: s.Mean.Default},
		Covariance: analysis.PosteriorCovariance{Name: s.Covariance.Name, Default: s.Covariance.Default, JustVariance: s.Covariance.JustVariance},
		Sampler: analysis.PosteriorSampler{
			Name:         s.Sampler.Name,
			Default:      s.Sampler.Default,
			Distribution: distribution,
		},
		Comparison:   comparison,
		PastDiscount: s.PastDiscount,
		MemoryDepth:  s.MemoryDepth,
		Seed:         s.Seed,
	}, nil
}

func (s *posteriorEstimationSpec) resolve(
	storage *simulator.StateTimeStorage,
) ([]*simulator.PartitionConfig, map[string]int, error) {
	applied, err := s.resolveApplied()
	if err != nil {
		return nil, nil, err
	}
	partitions := analysis.NewPosteriorEstimationPartitions(applied, storage)
	return partitions, applied.Comparison.WindowDataHistoryDepth, nil
}

// meanGradientFunc is the Function field type of LikelihoodMeanGradient.
type meanGradientFunc = func(params *simulator.Params, likeMeanGrad []float64) []float64

var meanGradientFunctions = map[string]meanGradientFunc{
	"mean_gradient": inference.MeanGradientFunc,
}

// parameterisedModelWithGradientSpec is the data form of
// analysis.ParameterisedModelWithGradient (a gradient-capable likelihood).
type parameterisedModelWithGradientSpec struct {
	Likelihood         simulator.ComponentSpec                  `yaml:"likelihood"`
	Params             map[string][]float64                     `yaml:"params,omitempty"`
	ParamsAsPartitions map[string][]string                      `yaml:"params_as_partitions,omitempty"`
	ParamsFromUpstream map[string]simulator.NamedUpstreamConfig `yaml:"params_from_upstream,omitempty"`
}

func (s parameterisedModelWithGradientSpec) resolve() (analysis.ParameterisedModelWithGradient, error) {
	likelihood, err := resolveLikelihood(s.Likelihood)
	if err != nil {
		return analysis.ParameterisedModelWithGradient{}, err
	}
	withGradient, ok := likelihood.(inference.LikelihoodDistributionWithGradient)
	if !ok {
		return analysis.ParameterisedModelWithGradient{}, fmt.Errorf(
			"likelihood %q does not support gradients", s.Likelihood.Type)
	}
	return analysis.ParameterisedModelWithGradient{
		Likelihood:         withGradient,
		Params:             simulator.NewParams(s.Params),
		ParamsAsPartitions: s.ParamsAsPartitions,
		ParamsFromUpstream: s.ParamsFromUpstream,
	}, nil
}

// likelihoodMeanFunctionFitSpec is the data form of
// analysis.AppliedLikelihoodMeanFunctionFit.
type likelihoodMeanFunctionFitSpec struct {
	macroTypeField `yaml:",inline"`
	Name           string                             `yaml:"name"`
	Model          parameterisedModelWithGradientSpec `yaml:"model"`
	Gradient       struct {
		Function string `yaml:"function"`
		Width    int    `yaml:"width"`
	} `yaml:"gradient"`
	Data                   dataRefSpec            `yaml:"data"`
	Window                 windowedPartitionsSpec `yaml:"window"`
	EmbeddedBurnInSteps    *int                   `yaml:"embedded_burn_in_steps,omitempty"`
	WindowDataHistoryDepth map[string]int         `yaml:"window_data_history_depth,omitempty"`
	LearningRate           float64                `yaml:"learning_rate"`
	DescentIterations      int                    `yaml:"descent_iterations"`
	WarmStart              bool                   `yaml:"warm_start,omitempty"`
}

func (spec *likelihoodMeanFunctionFitSpec) resolve(
	storage *simulator.StateTimeStorage,
) ([]*simulator.PartitionConfig, map[string]int, error) {
	model, err := spec.Model.resolve()
	if err != nil {
		return nil, nil, err
	}
	window, err := spec.Window.resolve()
	if err != nil {
		return nil, nil, err
	}
	gradientFn, ok := meanGradientFunctions[spec.Gradient.Function]
	if !ok {
		return nil, nil, fmt.Errorf("gradient: unknown function %q", spec.Gradient.Function)
	}
	applied := analysis.AppliedLikelihoodMeanFunctionFit{
		Name:                   spec.Name,
		Model:                  model,
		Gradient:               analysis.LikelihoodMeanGradient{Function: gradientFn, Width: spec.Gradient.Width},
		Data:                   spec.Data.resolve(),
		Window:                 window,
		EmbeddedBurnInSteps:    spec.EmbeddedBurnInSteps,
		WindowDataHistoryDepth: spec.WindowDataHistoryDepth,
		LearningRate:           spec.LearningRate,
		DescentIterations:      spec.DescentIterations,
		WarmStart:              spec.WarmStart,
	}
	partition := analysis.NewLikelihoodMeanFunctionFitPartition(applied, storage)
	return []*simulator.PartitionConfig{partition}, applied.WindowDataHistoryDepth, nil
}
