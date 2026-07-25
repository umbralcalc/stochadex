package api

import (
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/analysis"
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

func (s *scalarRegressionStatsSpec) resolve(
	storage *simulator.StateTimeStorage,
) ([]*simulator.PartitionConfig, map[string]int, error) {
	mode, ok := regressionModes[s.Mode]
	if !ok {
		return nil, nil, fmt.Errorf("scalar_regression_stats: unknown mode %q", s.Mode)
	}
	partition := macros.NewScalarRegressionStatsPartition(
		macros.AppliedScalarRegressionStats{
			Name:              s.Name,
			Y:                 s.Y.resolve(),
			X:                 s.X.resolve(),
			Intercept:         s.Intercept,
			Mode:              mode,
			WindowLength:      s.WindowLength,
			MinDenominator:    s.MinDenominator,
			StateHistoryDepth: s.StateHistoryDepth,
		},
		storage,
	)
	return []*simulator.PartitionConfig{partition}, nil, nil
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
}

func (s *groupedAggregationSpec) resolve(
	storage *simulator.StateTimeStorage,
) ([]*simulator.PartitionConfig, map[string]int, error) {
	aggregation, ok := aggregationFunctions[s.Aggregation]
	if !ok {
		return nil, nil, fmt.Errorf("grouped_aggregation: unknown aggregation %q", s.Aggregation)
	}
	grouped := analysis.NewGroupedStateTimeStorage(
		analysis.AppliedGrouping{GroupBy: resolveDataRefs(s.GroupBy), Precision: s.Precision},
		storage,
	)
	applied := macros.AppliedAggregation{
		Name:         s.Name,
		Data:         s.Data.resolve(),
		DefaultValue: s.DefaultValue,
	}
	if s.Kernel.IsData() {
		kernel, err := resolveKernel(s.Kernel)
		if err != nil {
			return nil, nil, err
		}
		applied.Kernel = kernel
	}
	partition := macros.NewGroupedAggregationPartition(aggregation, applied, grouped)
	applyParams(partition, s.Params)
	window := s.Window
	if window == 0 {
		window = 1
	}
	windows := map[string]int{s.Name: 1, s.Data.PartitionName: window}
	// The group-by partitions are read alongside the data, so they need the window too.
	for _, ref := range s.GroupBy {
		windows[ref.PartitionName] = window
	}
	return []*simulator.PartitionConfig{partition}, windows, nil
}
