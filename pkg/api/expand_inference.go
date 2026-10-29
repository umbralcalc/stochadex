package api

import (
	"fmt"
	"maps"

	"github.com/umbralcalc/stochadex/pkg/macros"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	yaml3 "gopkg.in/yaml.v3"
)

// The inference and statistics macros as core-language config, mirroring the
// pkg/macros constructors. A likelihood window is a nested run, stated in place
// as an embedded_simulation_run: the window's own partitions, a from_history
// replay of each data partition, and the partition scoring them; its host
// forwards outside values in and feeds the data's history.

// partitionAsData is a decoded data-spec partition (a window partition the
// config states) written back out as data.
func partitionAsData(partition simulator.PartitionConfig) (expandedPartition, error) {
	if !partition.IterationSpec.IsData() {
		return expandedPartition{}, fmt.Errorf("partition %q needs an iteration: {type: ...}",
			partition.Name)
	}
	return expandedPartition{
		Name:               partition.Name,
		Iteration:          component(partition.IterationSpec.Type, partition.IterationSpec.Fields),
		Params:             maps.Clone(partition.Params.Map),
		ParamsAsPartitions: maps.Clone(partition.ParamsAsPartitions),
		ParamsFromUpstream: upstreamsAsData(partition.ParamsFromUpstream),
		InitStateValues:    partition.InitStateValues,
		StateHistoryDepth:  partition.StateHistoryDepth,
		Seed:               partition.Seed,
	}, nil
}

func upstreamsAsData(upstreams map[string]simulator.NamedUpstreamConfig) map[string]expandedUpstream {
	if len(upstreams) == 0 {
		return nil
	}
	data := make(map[string]expandedUpstream, len(upstreams))
	for key, upstream := range upstreams {
		data[key] = expandedUpstream{Upstream: upstream.Upstream, Indices: upstream.Indices}
	}
	return data
}

// nestedRunComponent is an embedded_simulation_run iteration stating a run in place.
func nestedRunComponent(partitions []expandedPartition, simulation map[string]interface{}) *yaml3.Node {
	return component("embedded_simulation_run", map[string]interface{}{
		"run": map[string]interface{}{"partitions": partitions, "simulation": simulation}})
}

// windowHost is what a likelihood window's host partition and its run hold
// before the scoring partitions are added.
type windowHost struct {
	partitions   []expandedPartition
	initState    []float64
	fromUpstream map[string]expandedUpstream
	asPartitions map[string][]string
	inner        map[string]bool
}

// expandWindow states a likelihood window's partitions and data replays, as
// macros.NewLikelihoodComparisonPartition and its kin build them.
func expandWindow(
	ctx *expansionContext,
	window windowedPartitionsSpec,
	historyDepths map[string]int,
) (*windowHost, error) {
	if window.Depth < 1 {
		return nil, fmt.Errorf("window depth must be >= 1, got %d", window.Depth)
	}
	host := &windowHost{fromUpstream: map[string]expandedUpstream{},
		asPartitions: map[string][]string{}, inner: map[string]bool{}}
	for _, windowed := range window.Partitions {
		partition, err := partitionAsData(windowed.Partition)
		if err != nil {
			return nil, err
		}
		host.partitions = append(host.partitions, partition)
		host.initState = append(host.initState, partition.InitStateValues...)
		host.inner[partition.Name] = true
		for param, upstream := range windowed.OutsideUpstreams {
			host.fromUpstream[partition.Name+"/"+param] =
				expandedUpstream{Upstream: upstream.Upstream, Indices: upstream.Indices}
		}
	}
	for _, ref := range window.Data {
		if len(ref.ValueIndices) > 0 {
			return nil, fmt.Errorf("window data %q: value indices are not supported in window data",
				ref.PartitionName)
		}
		if historyDepths != nil {
			depth, set := historyDepths[ref.PartitionName]
			if !set {
				return nil, fmt.Errorf("window_data_history_depth has no entry for window data "+
					"partition %q", ref.PartitionName)
			}
			if depth < window.Depth {
				return nil, fmt.Errorf("window data partition %q has history depth %d, below "+
					"the window's depth %d", ref.PartitionName, depth, window.Depth)
			}
		}
		first, err := ctx.firstRow(ref.PartitionName)
		if err != nil {
			return nil, err
		}
		host.partitions = append(host.partitions, expandedPartition{
			Name:              ref.PartitionName,
			Iteration:         component("from_history", nil),
			Params:            map[string][]float64{},
			InitStateValues:   first,
			StateHistoryDepth: 1,
		})
		host.initState = append(host.initState, first...)
		host.inner[ref.PartitionName] = true
		host.asPartitions[ref.PartitionName+"/update_from_partition_history"] = []string{ref.PartitionName}
		host.asPartitions[ref.PartitionName+"/initial_state_from_partition_history"] = []string{ref.PartitionName}
		host.fromUpstream[ref.PartitionName+"/latest_data_values"] = expandedUpstream{Upstream: ref.PartitionName}
	}
	return host, nil
}

// burnIn is a window host's burn-in: the window's depth unless the config sets
// one.
func burnIn(window windowedPartitionsSpec, set *int) (int, error) {
	steps := window.Depth
	if set != nil {
		steps = *set
	}
	if steps < 0 {
		return 0, fmt.Errorf("embedded_burn_in_steps must be >= 0, got %d", steps)
	}
	return steps, nil
}

// expandComparison is a likelihood_comparison's host partition, as
// macros.NewLikelihoodComparisonPartition builds it.
func (s likelihoodComparisonSpec) expandComparison(ctx *expansionContext) (expandedPartition, error) {
	host, err := expandWindow(ctx, s.Window, s.WindowDataHistoryDepth)
	if err != nil {
		return expandedPartition{}, err
	}
	steps, err := burnIn(s.Window, s.EmbeddedBurnInSteps)
	if err != nil {
		return expandedPartition{}, err
	}
	if _, err := ctx.width(s.Data.PartitionName); err != nil {
		return expandedPartition{}, err
	}
	params := maps.Clone(s.Model.Params)
	if params == nil {
		params = map[string][]float64{}
	}
	params["cumulative"], params["burn_in_steps"] = []float64{1}, []float64{0}
	upstreams := maps.Clone(s.Model.ParamsFromUpstream)
	if upstreams == nil {
		upstreams = map[string]simulator.NamedUpstreamConfig{}
	}
	upstreams["latest_data_values"] = simulator.NamedUpstreamConfig{
		Upstream: s.Data.PartitionName, Indices: s.Data.ValueIndices}
	// The model's wiring to a partition outside the window (a posterior's
	// sampler) is forwarded in by the host each step, as comparison/<param>.
	inner := map[string]simulator.NamedUpstreamConfig{}
	for key, upstream := range upstreams {
		if host.inner[upstream.Upstream] {
			inner[key] = upstream
		} else {
			host.fromUpstream["comparison/"+key] =
				expandedUpstream{Upstream: upstream.Upstream, Indices: upstream.Indices}
		}
	}
	if s.Model.Likelihood.IsZero() {
		return expandedPartition{}, fmt.Errorf("model needs a likelihood: {type: ...}")
	}
	host.partitions = append(host.partitions, expandedPartition{
		Name: "comparison",
		Iteration: component("data_comparison", map[string]interface{}{
			"likelihood": component(s.Model.Likelihood.Type, s.Model.Likelihood.Fields)}),
		Params:             params,
		ParamsAsPartitions: maps.Clone(s.Model.ParamsAsPartitions),
		ParamsFromUpstream: upstreamsAsData(inner),
		InitStateValues:    []float64{0},
		StateHistoryDepth:  1,
	})
	return expandedPartition{
		Name: s.Name,
		Iteration: nestedRunComponent(host.partitions, map[string]interface{}{
			"termination_condition": component("number_of_steps",
				map[string]interface{}{"max_steps": s.Window.Depth}),
			"timestep_function": component("from_history", nil),
			"init_time_value":   0.0,
		}),
		Params:             map[string][]float64{"burn_in_steps": {float64(steps)}},
		ParamsAsPartitions: host.asPartitions,
		ParamsFromUpstream: host.fromUpstream,
		InitStateValues:    append(host.initState, 0),
		StateHistoryDepth:  1,
	}, nil
}

func (s *likelihoodComparisonSpec) expand(ctx *expansionContext) (*expansion, error) {
	partition, err := s.expandComparison(ctx)
	if err != nil {
		return nil, err
	}
	return &expansion{partitions: []expandedPartition{partition}, depths: s.WindowDataHistoryDepth}, nil
}

func (s *posteriorEstimationSpec) expand(ctx *expansionContext) (*expansion, error) {
	n := len(s.Mean.Default)
	switch {
	case n == 0:
		return nil, fmt.Errorf("posterior mean.default must be non-empty")
	case len(s.Sampler.Default) != n:
		return nil, fmt.Errorf("sampler.default length must match the mean's %d, got %d",
			n, len(s.Sampler.Default))
	case s.Covariance.JustVariance && len(s.Covariance.Default) != n:
		return nil, fmt.Errorf("covariance.just_variance needs default of length %d, got %d",
			n, len(s.Covariance.Default))
	case !s.Covariance.JustVariance && len(s.Covariance.Default) != n*n:
		return nil, fmt.Errorf("covariance.default must have length %d (N²), got %d",
			n*n, len(s.Covariance.Default))
	case s.MemoryDepth < 1:
		return nil, fmt.Errorf("memory_depth must be >= 1, got %d", s.MemoryDepth)
	}
	samplerRead := false
	for _, upstream := range s.Comparison.Model.ParamsFromUpstream {
		samplerRead = samplerRead || upstream.Upstream == s.Sampler.Name
	}
	for _, windowed := range s.Comparison.Window.Partitions {
		for _, upstream := range windowed.OutsideUpstreams {
			samplerRead = samplerRead || upstream.Upstream == s.Sampler.Name
		}
	}
	if !samplerRead {
		return nil, fmt.Errorf("posterior comparison must read the sampler %q so the loglike "+
			"depends on the proposal, else the posterior cannot converge — either the "+
			"comparison model reads it via params_from_upstream (e.g. {mean: {upstream: %q}}), "+
			"or a window partition consumes it as an outside upstream",
			s.Sampler.Name, s.Sampler.Name)
	}
	comparison, err := s.Comparison.expandComparison(ctx)
	if err != nil {
		return nil, err
	}
	comparison.StateHistoryDepth = s.MemoryDepth
	loglikeIndices := []float64{float64(len(comparison.InitStateValues) - 1)}
	loglikes := []string{s.Comparison.Name}
	weighted := map[string][]string{"loglike_partitions": loglikes, "param_partitions": {s.Sampler.Name}}
	posterior := map[string]expandedUpstream{
		"posterior_log_normalisation": {Upstream: s.LogNorm.Name}, "mean": {Upstream: s.Mean.Name}}

	covariance := component("posterior_covariance", nil)
	samplerAsPartitions := maps.Clone(s.Sampler.Distribution.ParamsAsPartitions)
	samplerUpstreams := maps.Clone(s.Sampler.Distribution.ParamsFromUpstream)
	if s.Covariance.JustVariance {
		covariance = component("posterior_mean", map[string]interface{}{"transform": "variance"})
		if samplerAsPartitions == nil {
			samplerAsPartitions = map[string][]string{}
		}
		samplerAsPartitions["variance_partition"] = []string{s.Covariance.Name}
		delete(samplerUpstreams, "covariance_matrix")
	}
	if s.Sampler.Distribution.Likelihood.IsZero() {
		return nil, fmt.Errorf("sampler.distribution needs a likelihood: {type: ...}")
	}
	samplerParams := maps.Clone(s.Sampler.Distribution.Params)
	if samplerParams == nil {
		samplerParams = map[string][]float64{}
	}
	partitions := []expandedPartition{
		{
			Name:      s.LogNorm.Name,
			Iteration: component("posterior_log_normalisation", nil),
			Params: map[string][]float64{"loglike_indices": loglikeIndices,
				"past_discounting_factor": {s.PastDiscount}},
			ParamsAsPartitions: map[string][]string{"loglike_partitions": loglikes},
			InitStateValues:    []float64{s.LogNorm.Default},
			StateHistoryDepth:  1,
		},
		{
			Name:               s.Mean.Name,
			Iteration:          component("posterior_mean", map[string]interface{}{"transform": "mean"}),
			Params:             map[string][]float64{"loglike_indices": loglikeIndices},
			ParamsAsPartitions: weighted,
			ParamsFromUpstream: map[string]expandedUpstream{
				"posterior_log_normalisation": posterior["posterior_log_normalisation"]},
			InitStateValues:   s.Mean.Default,
			StateHistoryDepth: 1,
		},
		{
			Name:               s.Covariance.Name,
			Iteration:          covariance,
			Params:             map[string][]float64{"loglike_indices": loglikeIndices},
			ParamsAsPartitions: weighted,
			ParamsFromUpstream: posterior,
			InitStateValues:    s.Covariance.Default,
			StateHistoryDepth:  1,
		},
		{
			Name: s.Sampler.Name,
			Iteration: component("data_generation", map[string]interface{}{
				"likelihood": component(s.Sampler.Distribution.Likelihood.Type,
					s.Sampler.Distribution.Likelihood.Fields)}),
			Params:             samplerParams,
			ParamsAsPartitions: samplerAsPartitions,
			ParamsFromUpstream: upstreamsAsData(samplerUpstreams),
			InitStateValues:    s.Sampler.Default,
			StateHistoryDepth:  1,
			Seed:               s.Seed,
		},
		comparison,
	}
	return &expansion{partitions: partitions, depths: s.Comparison.WindowDataHistoryDepth}, nil
}

func (s *likelihoodMeanFunctionFitSpec) expand(ctx *expansionContext) (*expansion, error) {
	if _, known := meanGradientFunctions[s.Gradient.Function]; !known {
		return nil, fmt.Errorf("gradient: unknown function %q", s.Gradient.Function)
	}
	if s.Model.Likelihood.IsZero() {
		return nil, fmt.Errorf("model needs a likelihood: {type: ...}")
	}
	host, err := expandWindow(ctx, s.Window, s.WindowDataHistoryDepth)
	if err != nil {
		return nil, err
	}
	steps, err := burnIn(s.Window, s.EmbeddedBurnInSteps)
	if err != nil {
		return nil, err
	}
	if _, err := ctx.width(s.Data.PartitionName); err != nil {
		return nil, err
	}
	width := s.Gradient.Width
	params := maps.Clone(s.Model.Params)
	if params == nil {
		params = map[string][]float64{}
	}
	params[s.Data.PartitionName+"->data_values"] = []float64{}
	asPartitions := maps.Clone(s.Model.ParamsAsPartitions)
	if asPartitions == nil {
		asPartitions = map[string][]string{}
	}
	asPartitions["mean_partition"] = []string{"gradient_descent"}
	host.partitions = append(host.partitions, expandedPartition{
		Name: "gradient",
		Iteration: component("data_comparison_gradient", map[string]interface{}{
			"likelihood":        component(s.Model.Likelihood.Type, s.Model.Likelihood.Fields),
			"gradient_function": s.Gradient.Function,
		}),
		Params:             params,
		ParamsAsPartitions: asPartitions,
		ParamsFromUpstream: upstreamsAsData(s.Model.ParamsFromUpstream),
		InitStateValues:    make([]float64, width),
		StateHistoryDepth:  1,
	})
	host.asPartitions["gradient/update_from_partition_history"] = []string{s.Data.PartitionName}
	host.fromUpstream["gradient/target_state"] = expandedUpstream{Upstream: s.Data.PartitionName}
	host.initState = append(host.initState, make([]float64, width)...)
	descentOffset := len(host.initState)
	host.partitions = append(host.partitions, expandedPartition{
		Name:               "gradient_descent",
		Iteration:          component("gradient_descent", nil),
		Params:             map[string][]float64{"learning_rate": {s.LearningRate}},
		ParamsFromUpstream: map[string]expandedUpstream{"gradient": {Upstream: "gradient"}},
		InitStateValues:    make([]float64, width),
		StateHistoryDepth:  1,
	})
	host.initState = append(host.initState, make([]float64, width)...)
	hostParams := map[string][]float64{"burn_in_steps": {float64(steps)}}
	if s.WarmStart {
		hostParams["gradient_descent/init_state_values_from_outer"] =
			[]float64{float64(descentOffset), float64(width)}
	}
	return &expansion{
		partitions: []expandedPartition{{
			Name: s.Name,
			Iteration: nestedRunComponent(host.partitions, map[string]interface{}{
				"termination_condition": component("number_of_steps",
					map[string]interface{}{"max_steps": s.DescentIterations}),
				"timestep_function": component("constant", map[string]interface{}{"stepsize": 1.0}),
				"init_time_value":   0.0,
			}),
			Params:             hostParams,
			ParamsAsPartitions: host.asPartitions,
			ParamsFromUpstream: host.fromUpstream,
			InitStateValues:    host.initState,
			StateHistoryDepth:  s.Window.Depth,
		}},
		depths: s.WindowDataHistoryDepth,
	}, nil
}

func (s *scalarRegressionStatsSpec) expand(ctx *expansionContext) (*expansion, error) {
	mode, known := regressionModes[s.Mode]
	if !known {
		return nil, fmt.Errorf("scalar_regression_stats: unknown mode %q", s.Mode)
	}
	if mode == macros.RegressionStatsWindow && s.WindowLength < 1 {
		return nil, fmt.Errorf("window_length must be >= 1 in window mode, got %d", s.WindowLength)
	}
	index := func(ref dataRefSpec) (int, error) {
		indices, err := ctx.indices(ref)
		if err != nil {
			return 0, err
		}
		if len(indices) != 1 {
			return 0, fmt.Errorf("y and x must each read exactly one value; %q gives %d",
				ref.PartitionName, len(indices))
		}
		return indices[0], nil
	}
	y, err := index(s.Y)
	if err != nil {
		return nil, err
	}
	x, err := index(s.X)
	if err != nil {
		return nil, err
	}
	flag := func(on bool) []float64 {
		if on {
			return []float64{1}
		}
		return []float64{0}
	}
	minDenominator := s.MinDenominator
	if minDenominator <= 0 {
		minDenominator = 1e-12
	}
	return &expansion{partitions: []expandedPartition{{
		Name:      s.Name,
		Iteration: component("scalar_regression_stats", nil),
		Params: map[string][]float64{
			"scalar_regression_intercept":       flag(s.Intercept),
			"scalar_regression_mode":            flag(mode == macros.RegressionStatsWindow),
			"scalar_regression_window_length":   {float64(s.WindowLength)},
			"scalar_regression_min_denominator": {minDenominator},
		},
		ParamsFromUpstream: map[string]expandedUpstream{
			macros.ScalarRegressionParamY: {Upstream: s.Y.PartitionName, Indices: []int{y}},
			macros.ScalarRegressionParamX: {Upstream: s.X.PartitionName, Indices: []int{x}},
		},
		InitStateValues:   make([]float64, macros.ScalarRegressionStateWidth(s.Intercept, mode, s.WindowLength)),
		StateHistoryDepth: max(s.StateHistoryDepth, 1),
	}}}, nil
}
