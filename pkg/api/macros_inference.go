package api

import (
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

// parameterisedModelSpec is the data form of macros.ParameterisedModel: a
// likelihood spec plus its parameter wiring.
type parameterisedModelSpec struct {
	Likelihood         simulator.ComponentSpec                  `yaml:"likelihood"`
	Params             map[string][]float64                     `yaml:"params,omitempty"`
	ParamsAsPartitions map[string][]string                      `yaml:"params_as_partitions,omitempty"`
	ParamsFromUpstream map[string]simulator.NamedUpstreamConfig `yaml:"params_from_upstream,omitempty"`
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

// likelihoodComparisonSpec is the data form of macros.AppliedLikelihoodComparison.
type likelihoodComparisonSpec struct {
	macroTypeField         `yaml:",inline"`
	Name                   string                 `yaml:"name"`
	Model                  parameterisedModelSpec `yaml:"model"`
	Data                   dataRefSpec            `yaml:"data"`
	Window                 windowedPartitionsSpec `yaml:"window"`
	EmbeddedBurnInSteps    *int                   `yaml:"embedded_burn_in_steps,omitempty"`
	WindowDataHistoryDepth map[string]int         `yaml:"window_data_history_depth,omitempty"`
}

// posteriorEstimationSpec is the data form of macros.AppliedPosteriorEstimation.
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

// meanGradientFunc is the Function field type of LikelihoodMeanGradient.
type meanGradientFunc = func(params *simulator.Params, likeMeanGrad []float64) []float64

var meanGradientFunctions = map[string]meanGradientFunc{
	"mean_gradient": inference.MeanGradientFunc,
}

// parameterisedModelWithGradientSpec is the data form of
// macros.ParameterisedModelWithGradient (a gradient-capable likelihood).
type parameterisedModelWithGradientSpec struct {
	Likelihood         simulator.ComponentSpec                  `yaml:"likelihood"`
	Params             map[string][]float64                     `yaml:"params,omitempty"`
	ParamsAsPartitions map[string][]string                      `yaml:"params_as_partitions,omitempty"`
	ParamsFromUpstream map[string]simulator.NamedUpstreamConfig `yaml:"params_from_upstream,omitempty"`
}

// likelihoodMeanFunctionFitSpec is the data form of
// macros.AppliedLikelihoodMeanFunctionFit.
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
