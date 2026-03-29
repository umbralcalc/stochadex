package analysis

import (
	"fmt"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

func TestInference(t *testing.T) {
	t.Run(
		"test that the posterior estimation works",
		func(t *testing.T) {
			storage := NewStateTimeStorageFromPartitions(
				[]*simulator.PartitionConfig{
					{
						Name: "test_data",
						Iteration: &inference.DataGenerationIteration{
							Likelihood: &inference.NormalLikelihoodDistribution{},
						},
						Params: simulator.NewParams(map[string][]float64{
							"mean":              {1.8, 5.0},
							"covariance_matrix": {2.5, 0.0, 0.0, 9.0},
						}),
						InitStateValues:   []float64{1.3, 8.3},
						StateHistoryDepth: 1,
						Seed:              123,
					},
				},
				&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 500},
				&simulator.ConstantTimestepFunction{Stepsize: 1.0},
				0.0,
			)
			params := simulator.NewParams(make(map[string][]float64))
			params.Set("mean", []float64{1.8, 5.0})
			params.Set("covariance_matrix", []float64{2.5, 0.0, 0.0, 9.0})
			partitions := NewPosteriorEstimationPartitions(
				AppliedPosteriorEstimation{
					LogNorm: PosteriorLogNorm{
						Name:    "test_post_log_norm",
						Default: 0.0,
					},
					Mean: PosteriorMean{
						Name:    "test_post_mean",
						Default: []float64{1.8, 5.0},
					},
					Covariance: PosteriorCovariance{
						Name:    "test_post_cov",
						Default: []float64{2.5, 0.0, 0.0, 9.0},
					},
					Sampler: PosteriorSampler{
						Name:    "test_post_sampler",
						Default: []float64{1.8, 5.0},
						Distribution: ParameterisedModel{
							Likelihood: &inference.NormalLikelihoodDistribution{
								AllowDefaultCovarianceFallback: true,
							},
							Params: simulator.NewParams(map[string][]float64{
								"default_covariance": {2.5, 0.0, 0.0, 9.0},
								"cov_burn_in_steps":  {200},
							}),
							ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
								"mean":              {Upstream: "test_post_mean"},
								"covariance_matrix": {Upstream: "test_post_cov"},
							},
						},
					},
					Comparison: AppliedLikelihoodComparison{
						Name: "test_likelihood",
						Model: ParameterisedModel{
							Likelihood: &inference.NormalLikelihoodDistribution{},
							Params:     params,
						},
						Data: DataRef{PartitionName: "test_data"},
						Window: WindowedPartitions{
							Data:  []DataRef{{PartitionName: "test_data"}},
							Depth: 200,
						},
						WindowDataHistoryDepth: map[string]int{"test_data": 200},
					},
					PastDiscount: 1.0,
					MemoryDepth:  200,
					Seed:         1234,
				},
				storage,
			)
			ValidateWindowDataHistoryDepth(
				200,
				map[string]int{"test_data": 200},
				[]DataRef{{PartitionName: "test_data"}},
			)
			storage = AddPartitionsToStateTimeStorage(
				storage,
				partitions,
				map[string]int{"test_data": 200},
			)
			for _, name := range storage.GetNames() {
				for _, values := range storage.GetValues(name) {
					if floats.HasNaN(values) {
						t.Error("partition " + name + " values have NaN")
					}
				}
			}
		},
	)
	t.Run(
		"JustVariance posterior covariance uses diagonal defaults",
		func(t *testing.T) {
			storage := NewStateTimeStorageFromPartitions(
				[]*simulator.PartitionConfig{
					{
						Name: "test_data",
						Iteration: &inference.DataGenerationIteration{
							Likelihood: &inference.NormalLikelihoodDistribution{},
						},
						Params: simulator.NewParams(map[string][]float64{
							"mean":              {1.8, 5.0},
							"covariance_matrix": {2.5, 0.0, 0.0, 9.0},
						}),
						InitStateValues:   []float64{1.3, 8.3},
						StateHistoryDepth: 1,
						Seed:              123,
					},
				},
				&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 500},
				&simulator.ConstantTimestepFunction{Stepsize: 1.0},
				0.0,
			)
			params := simulator.NewParams(make(map[string][]float64))
			params.Set("mean", []float64{1.8, 5.0})
			params.Set("covariance_matrix", []float64{2.5, 0.0, 0.0, 9.0})
			partitions := NewPosteriorEstimationPartitions(
				AppliedPosteriorEstimation{
					LogNorm: PosteriorLogNorm{
						Name:    "jv_post_log_norm",
						Default: 0.0,
					},
					Mean: PosteriorMean{
						Name:    "jv_post_mean",
						Default: []float64{1.8, 5.0},
					},
					Covariance: PosteriorCovariance{
						Name:         "jv_post_cov",
						Default:      []float64{2.5, 9.0},
						JustVariance: true,
					},
					Sampler: PosteriorSampler{
						Name:    "jv_post_sampler",
						Default: []float64{1.8, 5.0},
						Distribution: ParameterisedModel{
							Likelihood: &inference.NormalLikelihoodDistribution{
								AllowDefaultCovarianceFallback: true,
							},
							Params: simulator.NewParams(map[string][]float64{
								"default_covariance": {2.5, 0.0, 0.0, 9.0},
								"cov_burn_in_steps":  {200},
							}),
							ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
								"mean": {Upstream: "jv_post_mean"},
							},
						},
					},
					Comparison: AppliedLikelihoodComparison{
						Name: "jv_likelihood",
						Model: ParameterisedModel{
							Likelihood: &inference.NormalLikelihoodDistribution{},
							Params:     params,
						},
						Data: DataRef{PartitionName: "test_data"},
						Window: WindowedPartitions{
							Data:  []DataRef{{PartitionName: "test_data"}},
							Depth: 200,
						},
						WindowDataHistoryDepth: map[string]int{"test_data": 200},
					},
					PastDiscount: 1.0,
					MemoryDepth:  200,
					Seed:         1234,
				},
				storage,
			)
			storage = AddPartitionsToStateTimeStorage(
				storage,
				partitions,
				map[string]int{"test_data": 200},
			)
			for _, name := range storage.GetNames() {
				for _, values := range storage.GetValues(name) {
					if floats.HasNaN(values) {
						t.Errorf("partition %s has NaN", name)
					}
				}
			}
		},
	)
	t.Run(
		"wrong full covariance default length panics at setup",
		func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("expected panic")
				}
				if !strings.Contains(fmt.Sprint(r), "N²") {
					t.Fatalf("unexpected panic: %v", r)
				}
			}()
			storage := simulator.NewStateTimeStorage()
			model := ParameterisedModel{
				Likelihood: &inference.NormalLikelihoodDistribution{},
				Params:     simulator.NewParams(map[string][]float64{}),
			}
			model.Init()
			samplerModel := ParameterisedModel{
				Likelihood: &inference.NormalLikelihoodDistribution{},
				Params:     simulator.NewParams(map[string][]float64{}),
			}
			samplerModel.Init()
			_ = NewPosteriorEstimationPartitions(
				AppliedPosteriorEstimation{
					LogNorm:    PosteriorLogNorm{Name: "ln", Default: 0},
					Mean:       PosteriorMean{Name: "m", Default: []float64{0, 0}},
					Covariance: PosteriorCovariance{Name: "c", Default: []float64{1, 0, 0, 0, 1}},
					Sampler: PosteriorSampler{
						Name:         "s",
						Default:      []float64{0, 0},
						Distribution: samplerModel,
					},
					Comparison: AppliedLikelihoodComparison{
						Name:  "z",
						Model: model,
						Data:  DataRef{PartitionName: "x"},
						Window: WindowedPartitions{
							Depth: 1,
						},
					},
					PastDiscount: 1,
					MemoryDepth:  1,
				},
				storage,
			)
		},
	)
}
