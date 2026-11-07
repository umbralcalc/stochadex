package macros

import (
	"fmt"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestPosteriorReadsSampler(t *testing.T) {
	sampled := map[string]simulator.NamedUpstreamConfig{"mean": {Upstream: "sampler"}}
	other := map[string]simulator.NamedUpstreamConfig{"mean": {Upstream: "elsewhere"}}
	for _, c := range []struct {
		name   string
		model  map[string]simulator.NamedUpstreamConfig
		window []map[string]simulator.NamedUpstreamConfig
		reads  bool
	}{
		{"the model reading it", sampled, nil, true},
		{"a window partition reading it", other, []map[string]simulator.NamedUpstreamConfig{nil, sampled}, true},
		{"nothing reading it", other, []map[string]simulator.NamedUpstreamConfig{other}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := PosteriorReadsSampler("sampler", c.model, c.window); got != c.reads {
				t.Errorf("got %v", got)
			}
		})
	}

	storage := simulator.NewStateTimeStorage()
	storage.SetTimes([]float64{0, 1, 2})
	storage.SetValues("obs", [][]float64{{1}, {2}, {3}})
	posterior := func(window []WindowedPartition) AppliedPosteriorEstimation {
		return AppliedPosteriorEstimation{
			LogNorm:    PosteriorLogNorm{Name: "n"},
			Mean:       PosteriorMean{Name: "m", Default: []float64{0}},
			Covariance: PosteriorCovariance{Name: "c", Default: []float64{1}},
			Sampler: PosteriorSampler{Name: "s", Default: []float64{0},
				Distribution: ParameterisedModel{Likelihood: &inference.NormalLikelihoodDistribution{}}},
			Comparison: AppliedLikelihoodComparison{
				Name: "l",
				Model: ParameterisedModel{Likelihood: &inference.NormalLikelihoodDistribution{},
					Params: simulator.NewParams(map[string][]float64{"mean": {0}, "variance": {1}})},
				Data: analysis.DataRef{PartitionName: "obs"},
				Window: WindowedPartitions{Partitions: window,
					Data: []analysis.DataRef{{PartitionName: "obs"}}, Depth: 2},
			},
			MemoryDepth: 1,
		}
	}

	t.Run("a posterior whose window partition reads its sampler builds", func(t *testing.T) {
		partitions := NewPosteriorEstimationPartitions(posterior([]WindowedPartition{{
			Partition: &simulator.PartitionConfig{Name: "w", Iteration: &general.ParamValuesIteration{},
				Params:          simulator.NewParams(map[string][]float64{"param_values": {0}}),
				InitStateValues: []float64{0}, StateHistoryDepth: 1},
			OutsideUpstreams: map[string]simulator.NamedUpstreamConfig{"param_values": {Upstream: "s"}},
		}}), storage)
		if len(partitions) != 5 {
			t.Errorf("got %d partitions", len(partitions))
		}
	})

	t.Run("a posterior whose comparison ignores its sampler panics, saying why", func(t *testing.T) {
		defer func() {
			if got := fmt.Sprint(recover()); !strings.Contains(got, `must read the sampler "s"`) {
				t.Errorf("got %q", got)
			}
		}()
		NewPosteriorEstimationPartitions(posterior(nil), storage)
	})
}
