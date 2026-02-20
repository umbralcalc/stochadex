package analysis

import (
	"math"
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
			params := simulator.NewParams(make(map[string][]float64))
			params.Set("mean", []float64{1.8, 5.0})
			params.Set("covariance_matrix", []float64{2.5, 0.0, 0.0, 9.0})
			likePartition := NewLikelihoodComparisonPartition(
				AppliedLikelihoodComparison{
					Name: "test_likelihood",
					Model: ParameterisedModel{
						Likelihood: &inference.NormalLikelihoodDistribution{},
						Params:     params,
					},
					Data: DataRef{PartitionName: "test_data"},
					Window: WindowedPartitions{
						Data:  []DataRef{{PartitionName: "test_data"}},
						Depth: 10,
					},
				},
				storage,
			)
			storage = AddPartitionsToStateTimeStorage(
				storage,
				[]*simulator.PartitionConfig{likePartition},
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

func TestWarmStartConvergence(t *testing.T) {
	t.Run(
		"test that warm-start accumulates optimizer state across outer steps",
		func(t *testing.T) {
			// Generate data from a Normal distribution.
			storage := NewStateTimeStorageFromPartitions(
				[]*simulator.PartitionConfig{
					{
						Name: "test_data",
						Iteration: &inference.DataGenerationIteration{
							Likelihood: &inference.NormalLikelihoodDistribution{},
						},
						Params: simulator.NewParams(map[string][]float64{
							"mean":              {2.0, 4.0},
							"covariance_matrix": {1.0, 0.0, 0.0, 1.0},
						}),
						InitStateValues:   []float64{2.0, 4.0},
						StateHistoryDepth: 1,
						Seed:              42,
					},
				},
				&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 50},
				&simulator.ConstantTimestepFunction{Stepsize: 1.0},
				0.0,
			)
			gradientWidth := 2

			makePartition := func(name string, warmStart bool) *simulator.PartitionConfig {
				modelParams := simulator.NewParams(make(map[string][]float64))
				modelParams.Set("covariance_matrix", []float64{1.0, 0.0, 0.0, 1.0})
				return NewLikelihoodMeanFunctionFitPartition(
					AppliedLikelihoodMeanFunctionFit{
						Name: name,
						Model: ParameterisedModelWithGradient{
							Likelihood: &inference.NormalLikelihoodDistribution{},
							Params:     modelParams,
						},
						Gradient: LikelihoodMeanGradient{
							Function: inference.MeanGradientFunc,
							Width:    gradientWidth,
						},
						Data:              DataRef{PartitionName: "test_data"},
						Window:            WindowedPartitions{Depth: 3},
						LearningRate:      0.005,
						DescentIterations: 3,
						WarmStart:         warmStart,
					},
					storage,
				)
			}

			coldPartition := makePartition("cold_fit", false)
			warmPartition := makePartition("warm_fit", true)

			storage = AddPartitionsToStateTimeStorage(
				storage,
				[]*simulator.PartitionConfig{coldPartition, warmPartition},
				map[string]int{"test_data": 3},
			)

			for _, name := range storage.GetNames() {
				for _, values := range storage.GetValues(name) {
					if floats.HasNaN(values) {
						t.Error("partition " + name + " values have NaN")
					}
				}
			}

			coldVals := storage.GetValues("cold_fit")
			warmVals := storage.GetValues("warm_fit")
			// The gradient_descent state occupies [gradientWidth : gradientWidth*2]
			// in the concatenated outer state (gradient partition comes first).
			lastCold := coldVals[len(coldVals)-1][gradientWidth : gradientWidth*2]
			lastWarm := warmVals[len(warmVals)-1][gradientWidth : gradientWidth*2]

			// Warm-start carries the optimizer state between outer steps; its
			// gradient_descent state should have moved further from the zero
			// initialisation than cold-start (which always resets to zero).
			coldNorm := floats.Dot(lastCold, lastCold)
			warmNorm := floats.Dot(lastWarm, lastWarm)

			if math.IsNaN(coldNorm) || math.IsNaN(warmNorm) {
				t.Errorf("norms are NaN: cold=%v warm=%v", coldNorm, warmNorm)
			}
			if warmNorm <= coldNorm {
				t.Errorf(
					"warm-start |state|²=%v should exceed cold-start |state|²=%v; "+
						"cold final=%v warm final=%v",
					warmNorm, coldNorm, lastCold, lastWarm,
				)
			}
		},
	)
}

func TestWarmStartConvergesToGroundTruth(t *testing.T) {
	t.Run(
		"test that warm-start gradient ascent converges toward the true mean",
		func(t *testing.T) {
			trueMean := []float64{2.0, 4.0}
			gradientWidth := 2
			windowDepth := 5
			storage := NewStateTimeStorageFromPartitions(
				[]*simulator.PartitionConfig{
					{
						Name: "test_data",
						Iteration: &inference.DataGenerationIteration{
							Likelihood: &inference.NormalLikelihoodDistribution{},
						},
						Params: simulator.NewParams(map[string][]float64{
							"mean":              trueMean,
							"covariance_matrix": {1.0, 0.0, 0.0, 1.0},
						}),
						InitStateValues:   trueMean,
						StateHistoryDepth: 1,
						Seed:              42,
					},
				},
				&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 80},
				&simulator.ConstantTimestepFunction{Stepsize: 1.0},
				0.0,
			)
			modelParams := simulator.NewParams(make(map[string][]float64))
			modelParams.Set("covariance_matrix", []float64{1.0, 0.0, 0.0, 1.0})
			fitPartition := NewLikelihoodMeanFunctionFitPartition(
				AppliedLikelihoodMeanFunctionFit{
					Name: "converge_fit",
					Model: ParameterisedModelWithGradient{
						Likelihood: &inference.NormalLikelihoodDistribution{},
						Params:     modelParams,
					},
					Gradient: LikelihoodMeanGradient{
						Function: inference.MeanGradientFunc,
						Width:    gradientWidth,
					},
					Data:              DataRef{PartitionName: "test_data"},
					Window:            WindowedPartitions{Depth: windowDepth},
					LearningRate:      0.1,
					DescentIterations: 5,
					WarmStart:         true,
				},
				storage,
			)
			// Forward ascent=1 into the inner gradient_descent partition so the
			// optimizer maximises (rather than minimises) the log-likelihood.
			fitPartition.Params.Set("gradient_descent/ascent", []float64{1})

			storage = AddPartitionsToStateTimeStorage(
				storage,
				[]*simulator.PartitionConfig{fitPartition},
				map[string]int{"test_data": windowDepth},
			)

			for _, values := range storage.GetValues("converge_fit") {
				if floats.HasNaN(values) {
					t.Error("converge_fit values have NaN")
				}
			}

			vals := storage.GetValues("converge_fit")
			// gradient_descent state occupies [gradientWidth : gradientWidth*2].
			fitted := vals[len(vals)-1][gradientWidth : gradientWidth*2]

			// MSE relative to initial state (zeros) — i.e. how far truth is.
			initialMSE := 0.0
			for _, v := range trueMean {
				initialMSE += v * v
			}
			initialMSE /= float64(gradientWidth)

			// Warm-start gradient ascent should converge well within the initial
			// distance from truth; use a generous threshold of initialMSE/4.
			mse := 0.0
			for i, v := range fitted {
				diff := v - trueMean[i]
				mse += diff * diff
			}
			mse /= float64(gradientWidth)

			if mse >= initialMSE/4 {
				t.Errorf(
					"converged MSE=%v should be < initialMSE/4=%v; fitted=%v",
					mse, initialMSE/4, fitted,
				)
			}
		},
	)
}

func TestLikelihoodMeanFunctionFit(t *testing.T) {
	t.Run(
		"test that the likelihood mean function fit works",
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
			params := simulator.NewParams(make(map[string][]float64))
			params.Set("covariance_matrix", []float64{2.5, 0.0, 0.0, 9.0})
			likePartition := NewLikelihoodMeanFunctionFitPartition(
				AppliedLikelihoodMeanFunctionFit{
					Name: "test_mean_fit",
					Model: ParameterisedModelWithGradient{
						Likelihood: &inference.NormalLikelihoodDistribution{},
						Params:     params,
					},
					Gradient: LikelihoodMeanGradient{
						Function: inference.MeanGradientFunc,
						Width:    2,
					},
					Data:              DataRef{PartitionName: "test_data"},
					Window:            WindowedPartitions{Depth: 10},
					LearningRate:      0.02,
					DescentIterations: 100,
				},
				storage,
			)
			storage = AddPartitionsToStateTimeStorage(
				storage,
				[]*simulator.PartitionConfig{likePartition},
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
	t.Run(
		"test that cold-start gradient ascent converges toward the true mean",
		func(t *testing.T) {
			trueMean := []float64{2.0, 4.0}
			gradientWidth := 2
			windowDepth := 5
			storage := NewStateTimeStorageFromPartitions(
				[]*simulator.PartitionConfig{
					{
						Name: "test_data",
						Iteration: &inference.DataGenerationIteration{
							Likelihood: &inference.NormalLikelihoodDistribution{},
						},
						Params: simulator.NewParams(map[string][]float64{
							"mean":              trueMean,
							"covariance_matrix": {1.0, 0.0, 0.0, 1.0},
						}),
						InitStateValues:   trueMean,
						StateHistoryDepth: 1,
						Seed:              42,
					},
				},
				&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 80},
				&simulator.ConstantTimestepFunction{Stepsize: 1.0},
				0.0,
			)
			modelParams := simulator.NewParams(make(map[string][]float64))
			modelParams.Set("covariance_matrix", []float64{1.0, 0.0, 0.0, 1.0})
			fitPartition := NewLikelihoodMeanFunctionFitPartition(
				AppliedLikelihoodMeanFunctionFit{
					Name: "cold_converge_fit",
					Model: ParameterisedModelWithGradient{
						Likelihood: &inference.NormalLikelihoodDistribution{},
						Params:     modelParams,
					},
					Gradient: LikelihoodMeanGradient{
						Function: inference.MeanGradientFunc,
						Width:    gradientWidth,
					},
					Data:              DataRef{PartitionName: "test_data"},
					Window:            WindowedPartitions{Depth: windowDepth},
					LearningRate:      0.1,
					DescentIterations: 50,
					WarmStart:         false,
				},
				storage,
			)
			// Enable gradient ascent so each outer step maximises log-likelihood.
			fitPartition.Params.Set("gradient_descent/ascent", []float64{1})

			storage = AddPartitionsToStateTimeStorage(
				storage,
				[]*simulator.PartitionConfig{fitPartition},
				map[string]int{"test_data": windowDepth},
			)

			for _, values := range storage.GetValues("cold_converge_fit") {
				if floats.HasNaN(values) {
					t.Error("cold_converge_fit values have NaN")
				}
			}

			vals := storage.GetValues("cold_converge_fit")
			// gradient_descent state occupies [gradientWidth : gradientWidth*2].
			fitted := vals[len(vals)-1][gradientWidth : gradientWidth*2]

			// MSE of initial state (zeros) relative to true mean.
			initialMSE := 0.0
			for _, v := range trueMean {
				initialMSE += v * v
			}
			initialMSE /= float64(gradientWidth)

			// With 50 inner ascent steps from zero, (1-lr)^50 ≈ 0.005 so the
			// optimizer reaches ≈99.5% of the current window mean; the final
			// state should be well within initialMSE/4 of the true mean.
			mse := 0.0
			for i, v := range fitted {
				diff := v - trueMean[i]
				mse += diff * diff
			}
			mse /= float64(gradientWidth)

			if mse >= initialMSE/4 {
				t.Errorf(
					"converged MSE=%v should be < initialMSE/4=%v; fitted=%v",
					mse, initialMSE/4, fitted,
				)
			}
		},
	)
}
