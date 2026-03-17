package inference

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

func TestSMCHarness(t *testing.T) {
	t.Run(
		"test that SMC iterations run with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"smc_settings.yaml",
			)
			// Inner simulation: 2 particles, each with a ParamValuesIteration
			// (model prediction) and a DataComparisonIteration (loglike).
			// Data is constant [1.0], model prediction comes from proposal params.
			N := 2
			innerIterSettings := make([]simulator.IterationSettings, 0)
			innerIterations := make([]simulator.Iteration, 0)
			for p := range N {
				_ = p
				// ParamValuesIteration: outputs the particle's first param as "prediction"
				innerIterSettings = append(innerIterSettings, simulator.IterationSettings{
					Name: "",
					Params: simulator.NewParams(map[string][]float64{
						"param_values": {0.0},
					}),
					InitStateValues:   []float64{0.0},
					StateWidth:        1,
					StateHistoryDepth: 2,
					Seed:              0,
				})
				innerIterations = append(innerIterations, &general.ParamValuesIteration{})

				// DataComparisonIteration: compares prediction to constant data [1.0]
				innerIterSettings = append(innerIterSettings, simulator.IterationSettings{
					Name: "",
					Params: simulator.NewParams(map[string][]float64{
						"mean":               {0.0},
						"variance":           {1.0},
						"latest_data_values": {1.0},
						"cumulative":         {1},
						"burn_in_steps":      {0},
					}),
					ParamsFromUpstream: map[string]simulator.UpstreamConfig{
						"mean": {Upstream: 2 * p},
					},
					InitStateValues:   []float64{0.0},
					StateWidth:        1,
					StateHistoryDepth: 2,
					Seed:              0,
				})
				innerIterations = append(innerIterations, &DataComparisonIteration{
					Likelihood: &NormalLikelihoodDistribution{},
				})
			}

			innerSettings := &simulator.Settings{
				Iterations:            innerIterSettings,
				InitTimeValue:         0.0,
				TimestepsHistoryDepth: 2,
			}
			innerSettings.Init()

			innerImpl := &simulator.Implementations{
				Iterations:      innerIterations,
				OutputCondition: &simulator.NilOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 5,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}

			// Wire embedded sim params: forward proposal params to inner partitions
			embeddedUpstream := map[string]simulator.UpstreamConfig{
				// Particle 0: param_values from proposal[0], variance from proposal[1]
				"0/param_values": {Upstream: 0, Indices: []int{0}},
				"1/variance":     {Upstream: 0, Indices: []int{1}},
				// Particle 1: param_values from proposal[2], variance from proposal[3]
				"2/param_values": {Upstream: 0, Indices: []int{2}},
				"3/variance":     {Upstream: 0, Indices: []int{3}},
			}
			settings.Iterations[1].ParamsFromUpstream = embeddedUpstream

			iterations := []simulator.Iteration{
				&SMCProposalIteration{},
				general.NewEmbeddedSimulationRunIteration(innerSettings, innerImpl),
				&SMCPosteriorIteration{},
			}
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 2,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("harness test failed: %v", err)
			}
		},
	)
}

func TestLogSumExp(t *testing.T) {
	t.Run("standard values", func(t *testing.T) {
		x := []float64{-1.0, -2.0, -3.0}
		result := LogSumExp(x)
		expected := math.Log(math.Exp(-1) + math.Exp(-2) + math.Exp(-3))
		if math.Abs(result-expected) > 1e-10 {
			t.Errorf("got %f, expected %f", result, expected)
		}
	})
	t.Run("all negative infinity", func(t *testing.T) {
		x := []float64{math.Inf(-1), math.Inf(-1)}
		result := LogSumExp(x)
		if !math.IsInf(result, -1) {
			t.Errorf("expected -Inf, got %f", result)
		}
	})
	t.Run("single element", func(t *testing.T) {
		x := []float64{5.0}
		result := LogSumExp(x)
		if math.Abs(result-5.0) > 1e-10 {
			t.Errorf("got %f, expected 5.0", result)
		}
	})
	t.Run("empty", func(t *testing.T) {
		result := LogSumExp([]float64{})
		if !math.IsInf(result, -1) {
			t.Errorf("expected -Inf, got %f", result)
		}
	})
}

func TestCholeskyDecomp(t *testing.T) {
	t.Run("2x2 SPD", func(t *testing.T) {
		// [[4, 2], [2, 3]]
		a := []float64{4, 2, 2, 3}
		L := choleskyDecomp(a, 2)
		if L == nil {
			t.Fatal("expected non-nil Cholesky factor")
		}
		// L should be [[2, 0], [1, sqrt(2)]]
		if math.Abs(L[0]-2.0) > 1e-10 {
			t.Errorf("L[0,0]=%f, expected 2", L[0])
		}
		if math.Abs(L[2]-1.0) > 1e-10 {
			t.Errorf("L[1,0]=%f, expected 1", L[2])
		}
		if math.Abs(L[3]-math.Sqrt(2)) > 1e-10 {
			t.Errorf("L[1,1]=%f, expected sqrt(2)", L[3])
		}
	})
	t.Run("non-PD returns nil", func(t *testing.T) {
		a := []float64{1, 3, 3, 1}
		L := choleskyDecomp(a, 2)
		if L != nil {
			t.Error("expected nil for non-PD matrix")
		}
	})
}

func TestWeightedQuantiles(t *testing.T) {
	values := []float64{1.0, 2.0, 3.0, 4.0, 5.0}
	weights := []float64{0.2, 0.2, 0.2, 0.2, 0.2}
	probs := []float64{0.1, 0.5, 0.9}
	result := WeightedQuantiles(values, weights, probs)
	if result[0] != 1.0 {
		t.Errorf("10th percentile=%f, expected 1.0", result[0])
	}
	if result[1] != 3.0 {
		t.Errorf("50th percentile=%f, expected 3.0", result[1])
	}
	if result[2] != 5.0 {
		t.Errorf("90th percentile=%f, expected 5.0", result[2])
	}
}

func TestComputePosterior(t *testing.T) {
	// 3 particles with known log-likelihoods
	params := [][]float64{
		{1.0, 2.0},
		{3.0, 4.0},
		{5.0, 6.0},
	}
	logLiks := []float64{0.0, 0.0, 0.0} // equal weights
	result := ComputePosterior([]string{"a", "b"}, params, logLiks, nil)

	// With equal weights, posterior mean should be the sample mean
	if !floats.EqualApprox(result.PosteriorMean, []float64{3.0, 4.0}, 1e-10) {
		t.Errorf("posterior mean=%v, expected [3, 4]", result.PosteriorMean)
	}
	// Check weights sum to 1
	wSum := 0.0
	for _, w := range result.Weights {
		wSum += w
	}
	if math.Abs(wSum-1.0) > 1e-10 {
		t.Errorf("weights sum=%f, expected 1.0", wSum)
	}
	// Std should be positive
	for i, s := range result.PosteriorStd {
		if s <= 0 {
			t.Errorf("posterior std[%d]=%f, expected positive", i, s)
		}
	}
}

func TestSampleMultivariateNormal(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 43))
	mean := []float64{1.0, 2.0}
	cov := []float64{1.0, 0.0, 0.0, 1.0}
	priors := []Prior{
		&UniformPrior{Lo: -10, Hi: 10},
		&UniformPrior{Lo: -10, Hi: 10},
	}
	samples := sampleMultivariateNormal(rng, 1000, mean, cov, priors)
	if len(samples) != 1000 {
		t.Fatalf("expected 1000 samples, got %d", len(samples))
	}
	// Check empirical mean is close to target
	sMean := []float64{0, 0}
	for _, s := range samples {
		sMean[0] += s[0]
		sMean[1] += s[1]
	}
	sMean[0] /= 1000
	sMean[1] /= 1000
	if math.Abs(sMean[0]-1.0) > 0.15 {
		t.Errorf("sample mean[0]=%f, expected ~1.0", sMean[0])
	}
	if math.Abs(sMean[1]-2.0) > 0.15 {
		t.Errorf("sample mean[1]=%f, expected ~2.0", sMean[1])
	}
}
