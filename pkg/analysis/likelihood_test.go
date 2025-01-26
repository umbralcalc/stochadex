package analysis

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

func TestLikelihood(t *testing.T) {
	t.Run(
		"test that the likelihood comparison works",
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
				&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 100},
				&simulator.ConstantTimestepFunction{Stepsize: 1.0},
				0.0,
			)
			likePartition := NewLikelihoodComparisonPartition(
				AppliedLikelihoodComparison{
					Name:  "test_likelihood",
					Model: &inference.NormalLikelihoodDistribution{},
					Data:  DataRef{PartitionName: "test_data"},
					Window: WindowedPartitionsData{
						Partitions: []DataRef{{PartitionName: "test_data"}},
						Depth:      10,
					},
				},
				storage,
			)
			likePartition.Params.Set(
				"comparison/mean",
				[]float64{1.8, 5.0},
			)
			likePartition.Params.Set(
				"comparison/covariance_matrix",
				[]float64{2.5, 0.0, 0.0, 9.0},
			)
			storage = AddPartitionToStateTimeStorage(
				storage,
				likePartition,
				map[string]int{"test_data": 10},
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
