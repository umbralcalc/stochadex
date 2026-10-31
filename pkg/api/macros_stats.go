package api

import (
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/macros"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// scalarRegressionStatsSpec is the data form of macros.AppliedScalarRegressionStats.
type scalarRegressionStatsSpec struct {
	macroTypeField    `yaml:",inline"`
	Name              string      `yaml:"name"`
	Y                 dataRefSpec `yaml:"y"`
	X                 dataRefSpec `yaml:"x"`
	Intercept         bool        `yaml:"intercept,omitempty"`
	Mode              string      `yaml:"mode,omitempty"`
	WindowLength      int         `yaml:"window_length,omitempty"`
	MinDenominator    float64     `yaml:"min_denominator,omitempty"`
	StateHistoryDepth int         `yaml:"state_history_depth,omitempty"`
}

var regressionModes = map[string]macros.RegressionStatsMode{
	"":           macros.RegressionStatsCumulative,
	"cumulative": macros.RegressionStatsCumulative,
	"window":     macros.RegressionStatsWindow,
}

// groupedAggregationSpec is the data form of a grouped aggregation: an
// AppliedGrouping (group-by references + precision) that transforms the storage,
// plus a named aggregation function and the AppliedAggregation fields.
type groupedAggregationSpec struct {
	macroTypeField `yaml:",inline"`
	Name           string                  `yaml:"name"`
	Aggregation    string                  `yaml:"aggregation"`
	GroupBy        []dataRefSpec           `yaml:"group_by"`
	Precision      int                     `yaml:"precision"`
	Data           dataRefSpec             `yaml:"data"`
	DefaultValue   float64                 `yaml:"default_value,omitempty"`
	Kernel         simulator.ComponentSpec `yaml:"kernel,omitempty"`
	Params         map[string][]float64    `yaml:"params,omitempty"`
	Window         int                     `yaml:"window,omitempty"`
	// Groups declares the accepted groups, each a tuple with one value per
	// group_by reference, in output order. Unset, they are discovered from the
	// data: every distinct tuple, in the order it first appears.
	Groups [][]float64 `yaml:"groups,omitempty"`
	// UndeclaredGroups is what happens to a group the accepted groups leave
	// out: drop (the default), error, or other (one more output).
	UndeclaredGroups string `yaml:"undeclared_groups,omitempty"`
}

// policy is the spec's undeclared-groups policy.
func (s *groupedAggregationSpec) policy() (general.UndeclaredGroups, error) {
	if s.UndeclaredGroups == "" {
		return general.UndeclaredGroupsDrop, nil
	}
	policy, known := undeclaredGroupPolicies[s.UndeclaredGroups]
	if !known {
		return 0, fmt.Errorf("undeclared_groups must be drop, error or other, got %q", s.UndeclaredGroups)
	}
	return policy, nil
}

// declaredGroups checks the declared groups each give one value per group_by
// reference.
func (s *groupedAggregationSpec) declaredGroups() ([][]float64, error) {
	for i, group := range s.Groups {
		if len(group) != len(s.GroupBy) {
			return nil, fmt.Errorf("groups[%d] has %d values, but there are %d group_by "+
				"references; each group gives one value per reference", i, len(group), len(s.GroupBy))
		}
	}
	return s.Groups, nil
}

// expand states a grouped aggregation as data, as
// macros.NewGroupedAggregationPartition builds it: over the declared groups,
// or those discovered in the inputs, as the macro discovers them.
func (s *groupedAggregationSpec) expand(ctx *expansionContext) (*expansion, error) {
	if _, known := aggregationFunctions[s.Aggregation]; !known {
		return nil, fmt.Errorf("grouped_aggregation: unknown aggregation %q", s.Aggregation)
	}
	if len(s.GroupBy) == 0 {
		return nil, fmt.Errorf("grouped_aggregation needs group_by: the partitions whose values are the groups")
	}
	policy, err := s.policy()
	if err != nil {
		return nil, err
	}
	groups, err := s.declaredGroups()
	if err != nil {
		return nil, err
	}
	stateIndices, err := ctx.indices(s.Data)
	if err != nil {
		return nil, err
	}
	params := map[string][]float64{
		"float_precision":     {float64(s.Precision)},
		"state_value_indices": indexParams(stateIndices),
	}
	asPartitions := map[string][]string{"state_partition": {s.Data.PartitionName}}
	upstreams := map[string]expandedUpstream{"latest_states": {Upstream: s.Data.PartitionName}}
	window := max(s.Window, 1)
	depths := map[string]int{s.Data.PartitionName: window}
	for k, ref := range s.GroupBy {
		indices, err := ctx.indices(ref)
		if err != nil {
			return nil, err
		}
		if len(indices) != len(stateIndices) {
			return nil, fmt.Errorf("group_by %q reads %d values, but the data reads %d; each "+
				"grouping value groups the data value at its position",
				ref.PartitionName, len(indices), len(stateIndices))
		}
		key := strconv.Itoa(k)
		params["grouping_value_indices_tupindex_"+key] = indexParams(indices)
		asPartitions["grouping_partition_tupindex_"+key] = []string{ref.PartitionName}
		upstreams["latest_groupings_tupindex_"+key] = expandedUpstream{Upstream: ref.PartitionName}
		depths[ref.PartitionName] = window
	}
	if groups == nil {
		if groups, err = s.discoverGroups(ctx); err != nil {
			return nil, err
		}
	}
	for k := range s.GroupBy {
		column := make([]float64, len(groups))
		for i, group := range groups {
			column[i] = group[k]
		}
		params["accepted_value_group_tupindex_"+strconv.Itoa(k)] = column
	}
	defaults := repeated(s.DefaultValue, len(groups))
	if policy == general.UndeclaredGroupsOther {
		defaults = append(defaults, s.DefaultValue)
	}
	params["default_values"] = defaults
	maps.Copy(params, s.Params)
	fields := map[string]interface{}{
		"aggregation": s.Aggregation,
		"kernel":      componentOf(s.Kernel, "instantaneous"),
	}
	if s.UndeclaredGroups != "" {
		fields["undeclared_groups"] = s.UndeclaredGroups
	}
	return &expansion{
		partitions: []expandedPartition{{
			Name:               s.Name,
			Iteration:          component("values_grouped_aggregation", fields),
			Params:             params,
			ParamsAsPartitions: asPartitions,
			ParamsFromUpstream: upstreams,
			InitStateValues:    slices.Clone(defaults),
			StateHistoryDepth:  1,
		}},
		depths: depths,
	}, nil
}

// discoverGroups finds the groups in the inputs, as the macro finds them in its
// storage. Only an input has rows to find them in before the run.
func (s *groupedAggregationSpec) discoverGroups(ctx *expansionContext) ([][]float64, error) {
	for _, ref := range s.GroupBy {
		if !ctx.inputs[ref.PartitionName] {
			return nil, fmt.Errorf("grouped_aggregation groups by %q, which no input supplies, "+
				"so it has no recorded values to discover groups in; declare them with groups:",
				ref.PartitionName)
		}
	}
	storage, err := ctx.stored()
	if err != nil {
		return nil, err
	}
	grouped := analysis.NewGroupedStateTimeStorage(
		analysis.AppliedGrouping{GroupBy: resolveDataRefs(s.GroupBy), Precision: s.Precision}, storage)
	groups := make([][]float64, grouped.GetAcceptedValueGroupsLength())
	for i := range groups {
		groups[i] = make([]float64, len(s.GroupBy))
	}
	for k := range s.GroupBy {
		for i, value := range grouped.GetAcceptedValueGroups(k) {
			groups[i][k] = value
		}
	}
	return groups, nil
}
