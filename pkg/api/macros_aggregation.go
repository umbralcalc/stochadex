package api

import (
	"fmt"
	"maps"

	"github.com/umbralcalc/stochadex/pkg/macros"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	yaml3 "gopkg.in/yaml.v3"
)

// The aggregation macros: rolling windowed mean / variance / covariance of a data
// reference, weighted by an integration kernel.

// aggregationFields are the AppliedAggregation fields shared by the aggregation
// macros, plus the params: pass-through and the data-history window.
type aggregationFields struct {
	Name         string                  `yaml:"name"`
	Data         dataRefSpec             `yaml:"data"`
	Kernel       simulator.ComponentSpec `yaml:"kernel,omitempty"`
	DefaultValue float64                 `yaml:"default_value,omitempty"`
	Params       map[string][]float64    `yaml:"params,omitempty"`
	Window       int                     `yaml:"window,omitempty"`
}

func (f aggregationFields) window() int {
	if f.Window == 0 {
		return 1
	}
	return f.Window
}

// checkWindowFills fails when the run is too short for the macro's window to
// fill. A windowed aggregation returns default_value until its data has a
// window of history, so a window longer than the run never gives anything
// else: a silent constant, typically 0. steps is the run's length, 0 when it
// is not known.
func (f aggregationFields) checkWindowFills(macro string, steps int, cumulative bool) error {
	if steps == 0 || f.window() <= steps {
		return nil
	}
	fix := fmt.Sprintf("set window: to at most %d", steps)
	if cumulative {
		fix = "for the mean over every row, set cumulative: true and drop window: and kernel:; " +
			"for a rolling mean, " + fix
	}
	return configError(fmt.Errorf("api: %s %q has window: %d, but the run replays its "+
		"input for only %d steps; it returns default_value (%v) until its window has filled, "+
		"so it would return that every step. Fix: %s",
		macro, f.Name, f.window(), steps, f.DefaultValue, fix))
}

type vectorMeanSpec struct {
	macroTypeField    `yaml:",inline"`
	aggregationFields `yaml:",inline"`
	// Cumulative makes the macro the running mean over every row of its data,
	// each weighted equally, with no window or kernel (see
	// macros.NewVectorCumulativeMeanPartition).
	Cumulative bool `yaml:"cumulative,omitempty"`
}

// checkCumulative rejects the windowed-mean fields beside cumulative: true,
// which weights every row equally and keeps no window.
func (s *vectorMeanSpec) checkCumulative() error {
	switch {
	case s.Kernel.IsData():
		return fmt.Errorf("vector_mean %q: cumulative: true weights every row equally, so it "+
			"takes no kernel:; remove it", s.Name)
	case s.Window != 0:
		return fmt.Errorf("vector_mean %q: cumulative: true reads every row, so it takes no "+
			"window:; remove it", s.Name)
	case len(s.Params) != 0:
		return fmt.Errorf("vector_mean %q: cumulative: true has no kernel to give params: to; "+
			"remove them", s.Name)
	case s.DefaultValue != 0:
		return fmt.Errorf("vector_mean %q: cumulative: true starts at the data's first row, "+
			"so it takes no default_value:; remove it", s.Name)
	}
	return nil
}

type vectorVarianceSpec struct {
	macroTypeField    `yaml:",inline"`
	Mean              dataRefSpec `yaml:"mean"`
	aggregationFields `yaml:",inline"`
}

type vectorCovarianceSpec struct {
	macroTypeField    `yaml:",inline"`
	Mean              dataRefSpec `yaml:"mean"`
	aggregationFields `yaml:",inline"`
}

// expandAggregation is an aggregation macro's partition as data: its layout, over the
// iteration and value function the pkg/macros constructor uses, with the params:
// pass-through applied over the top as applyParams does.
func (f aggregationFields) expandAggregation(
	ctx *expansionContext,
	macro, iteration, function string,
	mean *dataRefSpec,
	layoutOf func(mean, data macros.AggregationRef) macros.AggregationLayout,
) (*expansion, error) {
	indices, err := ctx.indices(f.Data)
	if err != nil {
		return nil, err
	}
	if ctx.inputs[f.Data.PartitionName] {
		if err := f.checkWindowFills(macro, ctx.inputSteps, macro == "vector_mean"); err != nil {
			return nil, err
		}
	}
	var meanRef macros.AggregationRef
	if mean != nil {
		meanIndices, err := ctx.indices(*mean)
		if err != nil {
			return nil, err
		}
		meanRef = macros.AggregationRef{Partition: mean.PartitionName, Indices: meanIndices}
	}
	layout := layoutOf(meanRef, macros.AggregationRef{Partition: f.Data.PartitionName, Indices: indices})
	maps.Copy(layout.Params, f.Params)
	return &expansion{
		partitions: []expandedPartition{aggregationPartition(f.Name, component(iteration, map[string]interface{}{
			"function": function,
			"kernel":   componentOf(f.Kernel, "instantaneous"),
		}), layout)},
		// The mean is read as it stands this step, never over its history, so it asks
		// no window of its partition.
		depths: map[string]int{f.Data.PartitionName: f.window()},
	}, nil
}

// aggregationPartition is a layout as a partition with this name and iteration.
func aggregationPartition(name string, iteration *yaml3.Node, layout macros.AggregationLayout) expandedPartition {
	return expandedPartition{
		Name:               name,
		Iteration:          iteration,
		Params:             layout.Params,
		ParamsAsPartitions: layout.ParamsAsPartitions,
		ParamsFromUpstream: upstreamsAsData(layout.ParamsFromUpstream),
		InitStateValues:    layout.InitStateValues,
		InitStateFromUp:    layout.InitStateFromUpstream,
		StateHistoryDepth:  1,
	}
}

func (s *vectorMeanSpec) expand(ctx *expansionContext) (*expansion, error) {
	if s.Cumulative {
		return s.expandCumulative(ctx)
	}
	return s.expandAggregation(ctx, "vector_mean", "values_function_vector_mean", "data_values", nil,
		func(_, data macros.AggregationRef) macros.AggregationLayout {
			return macros.NewVectorMeanLayout(data, s.DefaultValue)
		})
}

func (s *vectorVarianceSpec) expand(ctx *expansionContext) (*expansion, error) {
	return s.expandAggregation(ctx, "vector_variance", "values_function_vector_mean", "data_values_variance", &s.Mean,
		func(mean, data macros.AggregationRef) macros.AggregationLayout {
			return macros.NewVectorVarianceLayout(mean, data, s.DefaultValue)
		})
}

func (s *vectorCovarianceSpec) expand(ctx *expansionContext) (*expansion, error) {
	return s.expandAggregation(ctx, "vector_covariance", "values_function_vector_covariance", "data_values", &s.Mean,
		func(mean, data macros.AggregationRef) macros.AggregationLayout {
			return macros.NewVectorCovarianceLayout(mean, data, s.DefaultValue)
		})
}

// expandCumulative is a cumulative vector_mean's partition as data, as
// macros.NewVectorCumulativeMeanPartition builds it.
func (s *vectorMeanSpec) expandCumulative(ctx *expansionContext) (*expansion, error) {
	if err := s.checkCumulative(); err != nil {
		return nil, err
	}
	indices, err := ctx.indices(s.Data)
	if err != nil {
		return nil, err
	}
	return &expansion{
		partitions: []expandedPartition{aggregationPartition(s.Name,
			component("values_function_cumulative_mean", map[string]interface{}{"function": "data_values"}),
			macros.NewVectorCumulativeMeanLayout(macros.AggregationRef{Partition: s.Data.PartitionName, Indices: indices}))},
		// It reads only the data's latest value, so it asks no window of it.
		depths: map[string]int{},
	}, nil
}
