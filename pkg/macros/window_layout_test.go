package macros

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestWindowLayout(t *testing.T) {
	window := func() WindowLayout {
		return NewWindowLayout(
			[]WindowPartitionRef{{Name: "walk", InitState: []float64{1, 2},
				OutsideUpstreams: map[string]simulator.NamedUpstreamConfig{"rate": {Upstream: "outer"}}}},
			[]WindowReplay{{Name: "obs", InitState: []float64{5}}}, 3)
	}

	t.Run("a window lays out its partitions, then its replays", func(t *testing.T) {
		got := window()
		if !reflect.DeepEqual(got.Params, map[string][]float64{"burn_in_steps": {3}}) ||
			!reflect.DeepEqual(got.InitStateValues, []float64{1, 2, 5}) ||
			!reflect.DeepEqual(got.ParamsAsPartitions, map[string][]string{
				"obs/update_from_partition_history":        {"obs"},
				"obs/initial_state_from_partition_history": {"obs"}}) ||
			!reflect.DeepEqual(got.ParamsFromUpstream, map[string]simulator.NamedUpstreamConfig{
				"walk/rate":              {Upstream: "outer"},
				"obs/latest_data_values": {Upstream: "obs"}}) {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("a comparison of a model with nothing set reads the data inside the window", func(t *testing.T) {
		layout := window()
		got := layout.AddComparison(nil, nil, nil, AggregationRef{Partition: "obs", Indices: []int{0}})
		want := InnerLayout{
			Params: map[string][]float64{"cumulative": {1}, "burn_in_steps": {0}},
			ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
				"latest_data_values": {Upstream: "obs", Indices: []int{0}}},
			InitStateValues: []float64{0},
		}
		if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(layout.InitStateValues, []float64{1, 2, 5, 0}) {
			t.Errorf("got %+v, window state %v", got, layout.InitStateValues)
		}
	})

	t.Run("a comparison reading outside the window has that forwarded in", func(t *testing.T) {
		layout := window()
		got := layout.AddComparison(map[string][]float64{"variance": {2}}, nil,
			map[string]simulator.NamedUpstreamConfig{"mean": {Upstream: "sampler"}},
			AggregationRef{Partition: "obs"})
		if _, inner := got.ParamsFromUpstream["mean"]; inner ||
			!reflect.DeepEqual(layout.ParamsFromUpstream["comparison/mean"], simulator.NamedUpstreamConfig{Upstream: "sampler"}) ||
			!reflect.DeepEqual(got.Params["variance"], []float64{2}) {
			t.Errorf("got %+v, window %+v", got, layout.ParamsFromUpstream)
		}
	})

	t.Run("a warm-started fit of a model with nothing set", func(t *testing.T) {
		layout := window()
		gradient, descent := layout.AddMeanFit(nil, nil, nil, "obs", 2, 0.1, true)
		wantGradient := InnerLayout{
			Params:             map[string][]float64{"obs->data_values": {}},
			ParamsAsPartitions: map[string][]string{"mean_partition": {"gradient_descent"}},
			InitStateValues:    []float64{0, 0},
		}
		wantDescent := InnerLayout{
			Params: map[string][]float64{"learning_rate": {0.1}},
			ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
				"gradient": {Upstream: "gradient"}},
			InitStateValues: []float64{0, 0},
		}
		if !reflect.DeepEqual(gradient, wantGradient) || !reflect.DeepEqual(descent, wantDescent) {
			t.Errorf("got %+v and %+v", gradient, descent)
		}
		// The descent's state starts after the window's three values and the gradient's two.
		if !reflect.DeepEqual(layout.Params["gradient_descent/init_state_values_from_outer"], []float64{5, 2}) ||
			!reflect.DeepEqual(layout.InitStateValues, []float64{1, 2, 5, 0, 0, 0, 0}) ||
			!reflect.DeepEqual(layout.ParamsFromUpstream["gradient/target_state"], simulator.NamedUpstreamConfig{Upstream: "obs"}) ||
			!reflect.DeepEqual(layout.ParamsAsPartitions["gradient/update_from_partition_history"], []string{"obs"}) {
			t.Errorf("got window %+v", layout)
		}
	})
}

func TestWindowedConstructors(t *testing.T) {
	storage := simulator.NewStateTimeStorage()
	storage.SetTimes([]float64{0, 1, 2})
	storage.SetValues("obs", [][]float64{{1}, {2}, {3}})
	comparison := func(burnIn *int, data analysis.DataRef) AppliedLikelihoodComparison {
		return AppliedLikelihoodComparison{
			Name: "l",
			Model: ParameterisedModel{Likelihood: &inference.NormalLikelihoodDistribution{},
				Params: simulator.NewParams(map[string][]float64{"mean": {0}, "variance": {1}})},
			Data:                analysis.DataRef{PartitionName: "obs"},
			Window:              WindowedPartitions{Data: []analysis.DataRef{data}, Depth: 2},
			EmbeddedBurnInSteps: burnIn,
		}
	}

	t.Run("a set burn-in replaces the window's depth", func(t *testing.T) {
		four := 4
		host := NewLikelihoodComparisonPartition(comparison(&four, analysis.DataRef{PartitionName: "obs"}), storage)
		if got := host.Params.Get("burn_in_steps"); !reflect.DeepEqual(got, []float64{4}) {
			t.Errorf("got burn-in %v", got)
		}
	})

	for _, c := range []struct {
		name   string
		burnIn int
		data   analysis.DataRef
		want   string
	}{
		{"a negative burn-in", -1, analysis.DataRef{PartitionName: "obs"}, "EmbeddedBurnInSteps must be >= 0, got -1"},
		{"window data read at chosen values", 0, analysis.DataRef{PartitionName: "obs", ValueIndices: []int{0}},
			"value indices are not supported in window data"},
	} {
		t.Run(c.name+" panics", func(t *testing.T) {
			defer func() {
				if got := fmt.Sprint(recover()); !strings.Contains(got, c.want) {
					t.Errorf("got %q, want %q", got, c.want)
				}
			}()
			NewLikelihoodComparisonPartition(comparison(&c.burnIn, c.data), storage)
		})
	}
}
