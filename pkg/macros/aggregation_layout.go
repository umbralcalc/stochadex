package macros

import (
	"maps"
	"slices"
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// AggregationRef is the values of a partition an aggregation reads.
type AggregationRef struct {
	Partition string
	Indices   []int
}

// aggregationRef resolves a data reference against storage.
func aggregationRef(ref analysis.DataRef, storage *simulator.StateTimeStorage) AggregationRef {
	return AggregationRef{Partition: ref.PartitionName, Indices: ref.GetValueIndices(storage)}
}

// indexParam is indices as a param's values.
func (r AggregationRef) indexParam() []float64 {
	values := make([]float64, len(r.Indices))
	for i, index := range r.Indices {
		values[i] = float64(index)
	}
	return values
}

// AggregationLayout is the data an aggregation's partition is wired with: everything
// about it but its name and iteration, which holds a typed function and kernel. The
// constructors here build their partitions from it, and a config states the same
// aggregation as data with these values, so the two cannot disagree on the wiring.
type AggregationLayout struct {
	Params             map[string][]float64
	ParamsAsPartitions map[string][]string
	ParamsFromUpstream map[string]simulator.NamedUpstreamConfig
	InitStateValues    []float64
	// InitStateFromUpstream names the params_from_upstream key the partition takes
	// its initial state from, if any.
	InitStateFromUpstream string
}

// partition is the layout as a partition with this name and iteration.
func (l AggregationLayout) partition(name string, iteration simulator.Iteration) *simulator.PartitionConfig {
	return &simulator.PartitionConfig{
		Name:                  name,
		Iteration:             iteration,
		Params:                simulator.NewParams(l.Params),
		ParamsAsPartitions:    l.ParamsAsPartitions,
		ParamsFromUpstream:    l.ParamsFromUpstream,
		InitStateValues:       l.InitStateValues,
		InitStateFromUpstream: l.InitStateFromUpstream,
		StateHistoryDepth:     1,
		Seed:                  0,
	}
}

// dataValuesLayout is the wiring every aggregation of a data partition's values shares:
// the rows it reads from history, and the latest one.
func dataValuesLayout(data AggregationRef, initState []float64) AggregationLayout {
	return AggregationLayout{
		Params:             map[string][]float64{"data_values_indices": data.indexParam()},
		ParamsAsPartitions: map[string][]string{"data_values_partition": {data.Partition}},
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"latest_data_values": {Upstream: data.Partition, Indices: data.Indices},
		},
		InitStateValues: initState,
	}
}

// repeatedValue is n copies of value.
func repeatedValue(value float64, n int) []float64 {
	values := make([]float64, n)
	for i := range values {
		values[i] = value
	}
	return values
}

// NewVectorMeanLayout is the layout of a rolling weighted mean of data's values,
// starting from defaultValue.
func NewVectorMeanLayout(data AggregationRef, defaultValue float64) AggregationLayout {
	return dataValuesLayout(data, repeatedValue(defaultValue, len(data.Indices)))
}

// NewVectorCumulativeMeanLayout is the layout of a running mean over every row of
// data, starting from the data's own initial state.
func NewVectorCumulativeMeanLayout(data AggregationRef) AggregationLayout {
	layout := dataValuesLayout(data, make([]float64, len(data.Indices)))
	layout.InitStateFromUpstream = "latest_data_values"
	return layout
}

// NewVectorVarianceLayout is the layout of a rolling weighted variance of data's values
// around mean, starting from defaultValue. Dividing by the weights' sum less one is
// the correction for mean being estimated from the same values.
func NewVectorVarianceLayout(mean, data AggregationRef, defaultValue float64) AggregationLayout {
	layout := NewVectorMeanLayout(data, defaultValue)
	layout.Params["subtract_from_normalisation"] = []float64{1}
	layout.ParamsFromUpstream["mean"] = simulator.NamedUpstreamConfig{
		Upstream: mean.Partition, Indices: mean.Indices}
	return layout
}

// NewVectorCovarianceLayout is the layout of a rolling weighted covariance matrix of
// data's values around mean. Its state is the matrix, row-major, starting with
// defaultValue on the diagonal.
func NewVectorCovarianceLayout(mean, data AggregationRef, defaultValue float64) AggregationLayout {
	width := len(data.Indices)
	initState := make([]float64, width*width)
	for i := range width {
		initState[i*width+i] = defaultValue
	}
	layout := dataValuesLayout(data, initState)
	layout.ParamsFromUpstream["mean"] = simulator.NamedUpstreamConfig{
		Upstream: mean.Partition, Indices: mean.Indices}
	return layout
}

// AggregationGrouping is one element of a grouped aggregation's group tuple: the
// values of a partition that hold it, and the value it takes in each group.
type AggregationGrouping struct {
	Partition      string
	ValueIndices   []float64
	AcceptedValues []float64
}

// NewGroupedAggregationLayout is the layout of an aggregation of data's values by group,
// with outputs values, each starting from defaultValue. A group is the tuple of its
// groupings' values, rounded to precision.
func NewGroupedAggregationLayout(
	data AggregationRef,
	groupings []AggregationGrouping,
	precision, outputs int,
	defaultValue float64,
) AggregationLayout {
	defaults := repeatedValue(defaultValue, outputs)
	layout := AggregationLayout{
		Params: map[string][]float64{
			"float_precision":     {float64(precision)},
			"state_value_indices": data.indexParam(),
			"default_values":      defaults,
		},
		ParamsAsPartitions: map[string][]string{"state_partition": {data.Partition}},
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"latest_states": {Upstream: data.Partition},
		},
		InitStateValues: slices.Clone(defaults),
	}
	for k, grouping := range groupings {
		key := strconv.Itoa(k)
		maps.Copy(layout.Params, map[string][]float64{
			"grouping_value_indices_tupindex_" + key: grouping.ValueIndices,
			"accepted_value_group_tupindex_" + key:   grouping.AcceptedValues,
		})
		layout.ParamsAsPartitions["grouping_partition_tupindex_"+key] = []string{grouping.Partition}
		layout.ParamsFromUpstream["latest_groupings_tupindex_"+key] =
			simulator.NamedUpstreamConfig{Upstream: grouping.Partition}
	}
	return layout
}
