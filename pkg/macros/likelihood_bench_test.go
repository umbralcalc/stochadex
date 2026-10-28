package macros

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// BenchmarkLikelihoodComparisonWindow runs a likelihood comparison over a
// replayed data window, the inner loop of posterior_estimation: every outer step
// re-runs the window's Depth inner steps.
func BenchmarkLikelihoodComparisonWindow(b *testing.B) {
	const steps, windowDepth = 200, 20
	// AddPartitionsToStateTimeStorage adds to the storage it is given, so each
	// run starts from fresh data, built outside the timer.
	newData := func() *simulator.StateTimeStorage {
		return analysis.NewStateTimeStorageFromPartitions(
			[]*simulator.PartitionConfig{{
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
			}},
			&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: steps},
			&simulator.ConstantTimestepFunction{Stepsize: 1.0},
			0.0,
		)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		data := newData()
		b.StartTimer()
		partition := NewLikelihoodComparisonPartition(
			AppliedLikelihoodComparison{
				Name: "test_likelihood",
				Model: ParameterisedModel{
					Likelihood: &inference.NormalLikelihoodDistribution{},
					Params: simulator.NewParams(map[string][]float64{
						"mean":              {1.8, 5.0},
						"covariance_matrix": {2.5, 0.0, 0.0, 9.0},
					}),
				},
				Data: analysis.DataRef{PartitionName: "test_data"},
				Window: WindowedPartitions{
					Data:  []analysis.DataRef{{PartitionName: "test_data"}},
					Depth: windowDepth,
				},
			},
			data,
		)
		analysis.AddPartitionsToStateTimeStorage(
			data,
			[]*simulator.PartitionConfig{partition},
			map[string]int{"test_data": windowDepth},
		)
	}
}
