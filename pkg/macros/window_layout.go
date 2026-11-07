package macros

import (
	"maps"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// WindowPartitionRef is a partition a likelihood window runs: its name, its initial
// state, and the params it reads from partitions outside the window.
type WindowPartitionRef struct {
	Name             string
	InitState        []float64
	OutsideUpstreams map[string]simulator.NamedUpstreamConfig
}

// WindowReplay is a data partition a window replays from its history, starting from
// the data's first row.
type WindowReplay struct {
	Name      string
	InitState []float64
}

// InnerLayout is a partition inside a window, as data: everything but its name and
// iteration.
type InnerLayout struct {
	Params             map[string][]float64
	ParamsAsPartitions map[string][]string
	ParamsFromUpstream map[string]simulator.NamedUpstreamConfig
	InitStateValues    []float64
}

// WindowLayout is the data a likelihood window's host partition is wired with: the
// params and upstreams it forwards into the window each step, and its initial state,
// the inner partitions' states in the order the window runs them. The window's own
// partitions come first, then its data replays, then what Add* adds. The constructors
// here and a config's expansion both build windows from it.
type WindowLayout struct {
	Params             map[string][]float64
	ParamsAsPartitions map[string][]string
	ParamsFromUpstream map[string]simulator.NamedUpstreamConfig
	InitStateValues    []float64
	inner              map[string]bool
}

// NewWindowLayout is the layout of a window running partitions and replaying data,
// with burnIn outer steps before it first runs.
func NewWindowLayout(partitions []WindowPartitionRef, replays []WindowReplay, burnIn int) WindowLayout {
	layout := WindowLayout{
		Params:             map[string][]float64{"burn_in_steps": {float64(burnIn)}},
		ParamsAsPartitions: map[string][]string{},
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{},
		InitStateValues:    []float64{},
		inner:              map[string]bool{},
	}
	for _, partition := range partitions {
		layout.InitStateValues = append(layout.InitStateValues, partition.InitState...)
		layout.inner[partition.Name] = true
		for param, upstream := range partition.OutsideUpstreams {
			layout.ParamsFromUpstream[partition.Name+"/"+param] = upstream
		}
	}
	for _, replay := range replays {
		layout.InitStateValues = append(layout.InitStateValues, replay.InitState...)
		layout.inner[replay.Name] = true
		layout.ParamsAsPartitions[replay.Name+"/update_from_partition_history"] = []string{replay.Name}
		layout.ParamsAsPartitions[replay.Name+"/initial_state_from_partition_history"] = []string{replay.Name}
		layout.ParamsFromUpstream[replay.Name+"/latest_data_values"] =
			simulator.NamedUpstreamConfig{Upstream: replay.Name}
	}
	return layout
}

// AddComparison adds the partition that scores data against a model each window step,
// named "comparison", and returns its layout: the model's params and wiring, its
// log-likelihood accumulating over the window. A model upstream naming a partition
// outside the window is forwarded in by the host each step, as comparison/<param>.
func (w *WindowLayout) AddComparison(
	params map[string][]float64,
	asPartitions map[string][]string,
	upstreams map[string]simulator.NamedUpstreamConfig,
	data AggregationRef,
) InnerLayout {
	params = cloneOrEmpty(params)
	params["cumulative"], params["burn_in_steps"] = []float64{1}, []float64{0}
	all := maps.Clone(upstreams)
	if all == nil {
		all = map[string]simulator.NamedUpstreamConfig{}
	}
	all["latest_data_values"] = simulator.NamedUpstreamConfig{Upstream: data.Partition, Indices: data.Indices}
	inner := map[string]simulator.NamedUpstreamConfig{}
	for key, upstream := range all {
		if w.inner[upstream.Upstream] {
			inner[key] = upstream
		} else {
			w.ParamsFromUpstream["comparison/"+key] = upstream
		}
	}
	w.InitStateValues = append(w.InitStateValues, 0)
	return InnerLayout{Params: params, ParamsAsPartitions: maps.Clone(asPartitions),
		ParamsFromUpstream: inner, InitStateValues: []float64{0}}
}

// AddMeanFit adds the partitions that fit a model's mean to data by gradient descent,
// named "gradient" and "gradient_descent", and returns their layouts. The gradient,
// width wide, is the model's against data's history; the descent steps the mean down
// it at learningRate. With warmStart, each outer step's descent starts where the last
// ended.
func (w *WindowLayout) AddMeanFit(
	params map[string][]float64,
	asPartitions map[string][]string,
	upstreams map[string]simulator.NamedUpstreamConfig,
	data string,
	width int,
	learningRate float64,
	warmStart bool,
) (gradient, descent InnerLayout) {
	params = cloneOrEmpty(params)
	params[data+"->data_values"] = []float64{}
	asPartitions = maps.Clone(asPartitions)
	if asPartitions == nil {
		asPartitions = map[string][]string{}
	}
	asPartitions["mean_partition"] = []string{"gradient_descent"}
	gradient = InnerLayout{Params: params, ParamsAsPartitions: asPartitions,
		ParamsFromUpstream: maps.Clone(upstreams), InitStateValues: make([]float64, width)}
	w.ParamsAsPartitions["gradient/update_from_partition_history"] = []string{data}
	w.ParamsFromUpstream["gradient/target_state"] = simulator.NamedUpstreamConfig{Upstream: data}
	w.InitStateValues = append(w.InitStateValues, make([]float64, width)...)
	if warmStart {
		w.Params["gradient_descent/init_state_values_from_outer"] =
			[]float64{float64(len(w.InitStateValues)), float64(width)}
	}
	descent = InnerLayout{
		Params:             map[string][]float64{"learning_rate": {learningRate}},
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{"gradient": {Upstream: "gradient"}},
		InitStateValues:    make([]float64, width),
	}
	w.InitStateValues = append(w.InitStateValues, make([]float64, width)...)
	return gradient, descent
}

// cloneOrEmpty is a copy of params, empty if there are none.
func cloneOrEmpty(params map[string][]float64) map[string][]float64 {
	if params == nil {
		return map[string][]float64{}
	}
	return maps.Clone(params)
}

// partition is the layout as a partition with this name and iteration.
func (l InnerLayout) partition(name string, iteration simulator.Iteration) *simulator.PartitionConfig {
	return &simulator.PartitionConfig{
		Name:               name,
		Iteration:          iteration,
		Params:             simulator.NewParams(l.Params),
		ParamsAsPartitions: l.ParamsAsPartitions,
		ParamsFromUpstream: l.ParamsFromUpstream,
		InitStateValues:    l.InitStateValues,
		StateHistoryDepth:  1,
	}
}
