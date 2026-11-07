package macros

import (
	"reflect"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestScalarRegressionLayout(t *testing.T) {
	refs := AppliedScalarRegressionStats{
		Name: "regression",
		Y:    analysis.DataRef{PartitionName: "ys", ValueIndices: []int{1}},
		X:    analysis.DataRef{PartitionName: "xs", ValueIndices: []int{0}},
	}
	upstreams := map[string]simulator.NamedUpstreamConfig{
		"y": {Upstream: "ys", Indices: []int{1}},
		"x": {Upstream: "xs", Indices: []int{0}},
	}
	params := func(intercept, mode, window, minDenominator float64) map[string][]float64 {
		return map[string][]float64{
			"scalar_regression_intercept":       {intercept},
			"scalar_regression_mode":            {mode},
			"scalar_regression_window_length":   {window},
			"scalar_regression_min_denominator": {minDenominator},
		}
	}
	// Each starts from the one pair x = 2, y = 5, worked by hand.
	for _, c := range []struct {
		name  string
		apply func(*AppliedScalarRegressionStats)
		want  ScalarRegressionLayout
	}{
		{
			"an intercept, with the defaults",
			func(a *AppliedScalarRegressionStats) { a.Intercept = true },
			ScalarRegressionLayout{
				Partition: InnerLayout{Params: params(1, 0, 0, 1e-12), ParamsFromUpstream: upstreams,
					// n, Sx, Sy, Sxx, Sxy, Syy, then alpha = Sy / n with no slope from one pair.
					InitStateValues: []float64{1, 2, 5, 4, 10, 25, 5, 0, 0}},
				HistoryDepth: 1,
			},
		},
		{
			"through the origin, under a minimum denominator above Sxx",
			func(a *AppliedScalarRegressionStats) {
				a.MinDenominator, a.StateHistoryDepth = 5, 3
			},
			ScalarRegressionLayout{
				Partition: InnerLayout{Params: params(0, 0, 0, 5), ParamsFromUpstream: upstreams,
					// Sxx, Sxy, Syy, n, then no slope, since Sxx = 4 is under 5.
					InitStateValues: []float64{4, 10, 25, 1, 0, 0}},
				HistoryDepth: 3,
			},
		},
		{
			"a window through the origin",
			func(a *AppliedScalarRegressionStats) {
				a.Mode, a.WindowLength = RegressionStatsWindow, 2
			},
			ScalarRegressionLayout{
				Partition: InnerLayout{Params: params(0, 1, 2, 1e-12), ParamsFromUpstream: upstreams,
					// The pairs, the count, then Sxx, Sxy, Syy, n and the slope Sxy / Sxx.
					InitStateValues: []float64{2, 5, 0, 0, 1, 4, 10, 25, 1, 2.5, 0}},
				HistoryDepth: 1,
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			applied := refs
			c.apply(&applied)
			layout := NewScalarRegressionLayout(applied, 1, 0, 2, 5)
			if !reflect.DeepEqual(layout, c.want) {
				t.Errorf("got  %+v\nwant %+v", layout, c.want)
			}

			storage := simulator.NewStateTimeStorage()
			storage.SetTimes([]float64{0, 1})
			storage.SetValues("xs", [][]float64{{2}, {3}})
			storage.SetValues("ys", [][]float64{{0, 5}, {0, 7}})
			partition := NewScalarRegressionStatsPartition(applied, storage)
			if !reflect.DeepEqual(partition.Params.Map, c.want.Partition.Params) ||
				!reflect.DeepEqual(partition.ParamsFromUpstream, upstreams) ||
				!reflect.DeepEqual(partition.InitStateValues, c.want.Partition.InitStateValues) ||
				partition.StateHistoryDepth != c.want.HistoryDepth {
				t.Errorf("the partition does not read its layout: %+v", partition)
			}
		})
	}
}
