package analysis

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
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
					Names: PosteriorEstimationNames{
						LogNorm:    "test_post_log_norm",
						Mean:       "test_post_mean",
						Covariance: "test_post_cov",
						Sampler:    "test_post_sampler",
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
					},
					Defaults: PosteriorDefaults{
						LogNorm:    0.0,
						Mean:       []float64{1.8, 5.0},
						Covariance: []float64{2.5, 0.0, 0.0, 9.0},
						Sampler:    []float64{1.8, 5.0},
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
						t.Error("partition " + name + " values have NaN")
					}
				}
			}
		},
	)
	t.Run(
		"test that the posterior t-kernel estimation works",
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
			partitions := NewPosteriorTKernelEstimationPartitions(
				AppliedPosteriorTKernelEstimation{
					Names: PosteriorTKernelEstimationNames{
						Updater: "test_updater",
						Sampler: "test_post_sampler",
					},
					Comparison: AppliedTKernelComparison{
						Name: "test_t_kernel",
						Model: ParameterisedTKernel{
							Data:              DataRef{PartitionName: "test_data"},
							Depth:             20,
							DegreesOfFreedom:  1,
							ScaleMatrixValues: []float64{2.5, 0.0, 0.0, 9.0},
							TimeDeltaRanges: []general.TimeDeltaRange{{
								LowerDelta: 0.0,
								UpperDelta: 21.0,
							}},
						},
						Data: DataRef{PartitionName: "test_data"},
						Window: WindowedPartitions{
							Data:  []DataRef{{PartitionName: "test_data"}},
							Depth: 200,
						},
					},
					ResamplingCov: []float64{1.0, 0.0, 0.0, 1.0},
					Defaults: PosteriorTKernelDefaults{
						Updater: []float64{2.5, 0.0, 0.0, 9.0, 1.0},
						Sampler: []float64{1.8, 5.0},
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
				map[string]int{"test_data": 220},
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
}
