package inference

import (
	"math"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats/scalar"
)

// TestEnsembleKalmanAnalysis pins the update maths against the analytic Kalman
// filter. With mean-zero (here, zero) observation perturbations the stochastic EnKF
// analysis MEAN is exact — it equals x̄_f + K(y − H x̄_f) with the Kalman gain built
// from the ensemble sample covariance — so these expected values are hand-computed,
// not tolerance-fudged.
func TestEnsembleKalmanAnalysis(t *testing.T) {
	t.Run("scalar state, fully observed", func(t *testing.T) {
		// Ensemble {-1, 1}: mean 0, sample variance P = 2. R = 1, so the gain is
		// K = P/(P+R) = 2/3. Observation y = 3.
		forecast := [][]float64{{-1}, {1}}
		analysis := EnsembleKalmanAnalysis(
			forecast, []float64{3}, []int{0}, []float64{1},
			[][]float64{{0}, {0}},
		)
		// member i: x_i + (2/3)(3 - x_i)  ->  {-1+8/3, 1+4/3} = {5/3, 7/3}.
		want := [][]float64{{5.0 / 3.0}, {7.0 / 3.0}}
		for i := range want {
			if !scalar.EqualWithinAbs(analysis[i][0], want[i][0], 1e-9) {
				t.Errorf("member %d = %v, want %v", i, analysis[i][0], want[i][0])
			}
		}
		// Analysis mean is exactly the Kalman posterior mean, 2.
		mean := (analysis[0][0] + analysis[1][0]) / 2
		if !scalar.EqualWithinAbs(mean, 2.0, 1e-9) {
			t.Errorf("analysis mean = %v, want 2", mean)
		}
	})

	t.Run("partial observation updates the unobserved dimension", func(t *testing.T) {
		// A 2-D state where dim 1 = 2 * dim 0 across the ensemble, so it is perfectly
		// correlated with the observed dim 0 and MUST be updated through the ensemble
		// cross-covariance — the property a plain per-dimension filter would miss.
		// Sample covariance: P00 = 4, P11 = 16, P01 = 8. Observe dim 0 with R = 1:
		// K = [P00, P01]/(P00+R) = [4/5, 8/5]. y = 6 (forecast mean of dim 0 is 0).
		forecast := [][]float64{{-2, -4}, {0, 0}, {2, 4}}
		analysis := EnsembleKalmanAnalysis(
			forecast, []float64{6}, []int{0}, []float64{1},
			[][]float64{{0}, {0}, {0}},
		)
		want := [][]float64{{4.4, 8.8}, {4.8, 9.6}, {5.2, 10.4}}
		for i := range want {
			if !scalar.EqualWithinAbsOrRel(analysis[i][0], want[i][0], 1e-9, 1e-9) ||
				!scalar.EqualWithinAbsOrRel(analysis[i][1], want[i][1], 1e-9, 1e-9) {
				t.Errorf("member %d = %v, want %v", i, analysis[i], want[i])
			}
		}
	})
}

// steadyStateKalmanVariance iterates the scalar Riccati recursion to its fixed point,
// giving the exact analysis-error variance the EnKF should reproduce on a linear-
// Gaussian AR(1)/noisy-observation model.
func steadyStateKalmanVariance(a, q, r float64) float64 {
	pa := 0.0
	for i := 0; i < 10000; i++ {
		pf := a*a*pa + q
		pa = pf * r / (pf + r)
	}
	return pa
}

// TestEnsembleKalmanFilterTracksLatentState is the expected-behaviour suite for the
// filter as a whole: run the full forecast→assimilate loop on a linear-Gaussian
// AR(1) latent state with noisy observations and check (1) the analysis mean tracks
// the hidden truth with error well below the observation noise — i.e. assimilation
// actually reduces uncertainty — and (2) the analysis ensemble spread matches the
// exact Kalman steady-state variance, which is the correctness bar an EnKF is held to.
func TestEnsembleKalmanFilterTracksLatentState(t *testing.T) {
	const (
		a        = 0.9 // AR(1) persistence
		q        = 0.1 // process-noise variance
		r        = 0.5 // observation-noise variance
		nMembers = 200
		steps    = 2000
		burnIn   = 200
	)
	sampler := rng.New(20240611)

	// Initialise the analysis ensemble as a spread around 0.
	ens := make([][]float64, nMembers)
	for i := range ens {
		ens[i] = []float64{sampler.Normal(0, 1)}
	}

	truth := 0.0
	var sqErr, obsSqErr, spreadSum float64
	var counted int
	for s := 0; s < steps; s++ {
		// Advance the hidden truth and emit a noisy observation.
		truth = a*truth + sampler.Normal(0, math.Sqrt(q))
		obs := truth + sampler.Normal(0, math.Sqrt(r))

		// Forecast: propagate each member through the (same) forward model.
		forecast := make([][]float64, nMembers)
		for i := range ens {
			forecast[i] = []float64{a*ens[i][0] + sampler.Normal(0, math.Sqrt(q))}
		}

		// Assimilate the observation.
		perturb := make([][]float64, nMembers)
		for i := range perturb {
			perturb[i] = []float64{sampler.Normal(0, math.Sqrt(r))}
		}
		ens = EnsembleKalmanAnalysis(forecast, []float64{obs}, []int{0}, []float64{r}, perturb)

		// Metrics (after burn-in).
		if s < burnIn {
			continue
		}
		mean := 0.0
		for i := range ens {
			mean += ens[i][0]
		}
		mean /= float64(nMembers)
		var variance float64
		for i := range ens {
			variance += (ens[i][0] - mean) * (ens[i][0] - mean)
		}
		variance /= float64(nMembers - 1)
		sqErr += (mean - truth) * (mean - truth)
		obsSqErr += (obs - truth) * (obs - truth)
		spreadSum += variance
		counted++
	}

	analysisRMSE := math.Sqrt(sqErr / float64(counted))
	obsRMSE := math.Sqrt(obsSqErr / float64(counted))
	avgSpread := spreadSum / float64(counted)
	pInf := steadyStateKalmanVariance(a, q, r)

	t.Logf("analysis RMSE=%.4f  obs RMSE=%.4f  avg ensemble var=%.4f  Kalman P_inf=%.4f",
		analysisRMSE, obsRMSE, avgSpread, pInf)

	// (1) The filter beats the raw observations, and lands near the theoretical
	// analysis error sqrt(P_inf).
	if analysisRMSE >= obsRMSE {
		t.Errorf("analysis RMSE %.4f not below observation RMSE %.4f — filter is not assimilating",
			analysisRMSE, obsRMSE)
	}
	if analysisRMSE > 1.6*math.Sqrt(pInf) {
		t.Errorf("analysis RMSE %.4f far above sqrt(P_inf)=%.4f", analysisRMSE, math.Sqrt(pInf))
	}
	// (2) The ensemble spread reproduces the exact Kalman steady-state variance.
	if avgSpread < 0.5*pInf || avgSpread > 2.0*pInf {
		t.Errorf("avg ensemble variance %.4f not within [0.5, 2.0]*P_inf (P_inf=%.4f)",
			avgSpread, pInf)
	}
}

// TestEnsembleKalmanFilterIteration runs the iteration inside a real coordinator and
// through the correctness harnesses (NaN / width / params-mutation / re-runnability).
// The forecast here is an independent Wiener ensemble rather than a model-in-the-loop:
// the harness checks the iteration's mechanics, and the loop dynamics are covered by
// TestEnsembleKalmanFilterTracksLatentState above.
func TestEnsembleKalmanFilterIteration(t *testing.T) {
	newImplementations := func(store *simulator.StateTimeStorage) *simulator.Implementations {
		return &simulator.Implementations{
			Iterations: []simulator.Iteration{
				&DataGenerationIteration{Likelihood: &NormalLikelihoodDistribution{}},
				&continuous.WienerProcessIteration{},
				&EnsembleKalmanFilterIteration{},
			},
			OutputCondition: &simulator.EveryStepOutputCondition{},
			OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
				MaxNumberOfSteps: 100,
			},
			TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		}
	}

	t.Run("runs in a coordinator", func(t *testing.T) {
		settings := simulator.LoadSettingsFromYaml("enkf_settings.yaml")
		impl := newImplementations(simulator.NewStateTimeStorage())
		for i, iteration := range impl.Iterations {
			iteration.Configure(i, settings)
		}
		simulator.NewPartitionCoordinator(settings, impl).Run()
	})

	t.Run("runs with harnesses", func(t *testing.T) {
		settings := simulator.LoadSettingsFromYaml("enkf_settings.yaml")
		impl := newImplementations(simulator.NewStateTimeStorage())
		if err := simulator.RunWithHarnesses(settings, impl); err != nil {
			t.Errorf("test harness failed: %v", err)
		}
	})
}
