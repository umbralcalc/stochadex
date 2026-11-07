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

// windowRun is a likelihood window's nested run: inline when its partitions
// allow (see macros.withInlineExecution), since the window re-runs its inner steps every outer step and the default strategy's
// goroutine per partition per inner step would dominate its cost.
func windowRun(partitions []expandedPartition, simulation map[string]interface{}) *yaml3.Node {
	if inlineOrdered(partitions) {
		simulation["execution_strategy"] = component("inline", nil)
	}
	return nestedRunComponent(partitions, simulation)
}

// inlineOrdered reports that every within-step upstream of a run's partitions
// comes earlier in partition order, which inline execution requires.
func inlineOrdered(partitions []expandedPartition) bool {
	order := make(map[string]int, len(partitions))
	for index, partition := range partitions {
		order[partition.Name] = index
	}
	for index, partition := range partitions {
		for _, upstream := range partition.ParamsFromUpstream {
			if earlier, inner := order[upstream.Upstream]; !inner || earlier >= index {
				return false
			}
		}
	}
	return true
}

// windowHost is a likelihood window as data: the partitions it runs, and its layout.
type windowHost struct {
	partitions []expandedPartition
	layout     macros.WindowLayout
}

// expandWindow states a likelihood window's partitions and data replays, as
// macros.NewLikelihoodComparisonPartition and its kin build them, with burnIn outer
// steps (the window's depth unless set) before it first runs.
func expandWindow(
	ctx *expansionContext,
	window windowedPartitionsSpec,
	historyDepths map[string]int,
	burnInSet *int,
) (*windowHost, error) {
	if window.Depth < 1 {
		return nil, fmt.Errorf("window depth must be >= 1, got %d", window.Depth)
	}
	host := &windowHost{}
	var refs []macros.WindowPartitionRef
	for _, windowed := range window.Partitions {
		partition, err := partitionAsData(windowed.Partition)
		if err != nil {
			return nil, err
		}
		host.partitions = append(host.partitions, partition)
		refs = append(refs, macros.WindowPartitionRef{Name: partition.Name,
			InitState: partition.InitStateValues, OutsideUpstreams: windowed.OutsideUpstreams})
	}
	var replays []macros.WindowReplay
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
			// The window replays its data from the bottom of the history, so
			// any more history than the window anchors it in rows that are
			// still zero, and the likelihood stops reading the data at all.
			if depth > window.Depth {
				return nil, fmt.Errorf("window data partition %q has history depth %d, above "+
					"the window's depth %d, so the window would replay zero-filled rows and the "+
					"likelihood would carry no information; set window_data_history_depth for %q "+
					"equal to the window's depth (%d)",
					ref.PartitionName, depth, window.Depth, ref.PartitionName, window.Depth)
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
		replays = append(replays, macros.WindowReplay{Name: ref.PartitionName, InitState: first})
	}
	burnIn := window.Depth
	if burnInSet != nil {
		burnIn = *burnInSet
	}
	if burnIn < 0 {
		return nil, fmt.Errorf("embedded_burn_in_steps must be >= 0, got %d", burnIn)
	}
	host.layout = macros.NewWindowLayout(refs, replays, burnIn)
	return host, nil
}

// add adds a partition inside the window, wired by its layout.
func (h *windowHost) add(name string, iteration *yaml3.Node, layout macros.InnerLayout) {
	h.partitions = append(h.partitions, expandedPartition{
		Name:               name,
		Iteration:          iteration,
		Params:             layout.Params,
		ParamsAsPartitions: layout.ParamsAsPartitions,
		ParamsFromUpstream: upstreamsAsData(layout.ParamsFromUpstream),
		InitStateValues:    layout.InitStateValues,
		StateHistoryDepth:  1,
	})
}

// run is the host partition running the window over simulation.
func (h *windowHost) run(name string, simulation map[string]interface{}, historyDepth int) expandedPartition {
	return expandedPartition{
		Name:               name,
		Iteration:          windowRun(h.partitions, simulation),
		Params:             h.layout.Params,
		ParamsAsPartitions: h.layout.ParamsAsPartitions,
		ParamsFromUpstream: upstreamsAsData(h.layout.ParamsFromUpstream),
		InitStateValues:    h.layout.InitStateValues,
		StateHistoryDepth:  historyDepth,
	}
}

// expandComparison is a likelihood_comparison's host partition, as
// macros.NewLikelihoodComparisonPartition builds it.
func (s likelihoodComparisonSpec) expandComparison(ctx *expansionContext) (expandedPartition, error) {
	host, err := expandWindow(ctx, s.Window, s.WindowDataHistoryDepth, s.EmbeddedBurnInSteps)
	if err != nil {
		return expandedPartition{}, err
	}
	if _, err := ctx.width(s.Data.PartitionName); err != nil {
		return expandedPartition{}, err
	}
	if s.Model.Likelihood.IsZero() {
		return expandedPartition{}, fmt.Errorf("model needs a likelihood: {type: ...}")
	}
	host.add("comparison", component("data_comparison", map[string]interface{}{
		"likelihood": component(s.Model.Likelihood.Type, s.Model.Likelihood.Fields)}),
		host.layout.AddComparison(s.Model.Params, s.Model.ParamsAsPartitions, s.Model.ParamsFromUpstream,
			macros.AggregationRef{Partition: s.Data.PartitionName, Indices: s.Data.ValueIndices}))
	return host.run(s.Name, map[string]interface{}{
		"termination_condition": component("number_of_steps",
			map[string]interface{}{"max_steps": s.Window.Depth}),
		"timestep_function": component("from_history", nil),
		"init_time_value":   0.0,
	}, 1), nil
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
	host, err := expandWindow(ctx, s.Window, s.WindowDataHistoryDepth, s.EmbeddedBurnInSteps)
	if err != nil {
		return nil, err
	}
	if _, err := ctx.width(s.Data.PartitionName); err != nil {
		return nil, err
	}
	gradient, descent := host.layout.AddMeanFit(s.Model.Params, s.Model.ParamsAsPartitions,
		s.Model.ParamsFromUpstream, s.Data.PartitionName, s.Gradient.Width, s.LearningRate, s.WarmStart)
	host.add("gradient", component("data_comparison_gradient", map[string]interface{}{
		"likelihood":        component(s.Model.Likelihood.Type, s.Model.Likelihood.Fields),
		"gradient_function": s.Gradient.Function,
	}), gradient)
	host.add("gradient_descent", component("gradient_descent", nil), descent)
	return &expansion{
		partitions: []expandedPartition{host.run(s.Name, map[string]interface{}{
			"termination_condition": component("number_of_steps",
				map[string]interface{}{"max_steps": s.DescentIterations}),
			"timestep_function": component("constant", map[string]interface{}{"stepsize": 1.0}),
			"init_time_value":   0.0,
		}, s.Window.Depth)},
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
	// The statistics start from the first rows' pair, so the data's initial
	// row, which the run never steps to, counts too. index has checked both
	// partitions are visible.
	yFirst, xFirst := ctx.firstRows[s.Y.PartitionName], ctx.firstRows[s.X.PartitionName]
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
		InitStateValues: macros.ScalarRegressionInitState(s.Intercept, mode, s.WindowLength,
			minDenominator, xFirst[x], yFirst[y]),
		StateHistoryDepth: max(s.StateHistoryDepth, 1),
	}}}, nil
}
