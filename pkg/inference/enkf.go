package inference

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// EnsembleKalmanAnalysis performs one stochastic (perturbed-observations)
// Ensemble Kalman Filter update, mapping a forecast ensemble to an analysis
// ensemble given an observation.
//
// The forecast is N members each of dimension d (forecast[i] has length d). The
// observation y has length p, and obsIndices selects which d state components those
// p observations correspond to — i.e. the observation operator H is a selection
// matrix, the common case (a subset of the state is observed directly). obsVar is
// the diagonal of the observation-error covariance R (length p). perturb supplies
// the per-member observation perturbations (perturb[i] has length p); passing
// mean-zero perturbations makes the analysis-mean update exact rather than
// Monte-Carlo, which is what the correctness test relies on.
//
// The maths is the textbook stochastic EnKF:
//
//	x̄  = mean of forecast members
//	A   = forecast anomalies (member − mean)
//	Pxy = A Aₕᵀ / (N−1)           cross-covariance, state × obs   (Aₕ = A at obsIndices)
//	Pyy = Aₕ Aₕᵀ / (N−1) + R      innovation covariance,  obs × obs
//	K   = Pxy Pyy⁻¹               Kalman gain
//	xₐⁱ = xᶠⁱ + K (y + εⁱ − H xᶠⁱ)  per member i
//
// The ensemble-estimated Pxy/Pyy are what let this filter a nonlinear forward model
// without a tangent-linear operator: the caller propagates each member through any
// model to form the forecast, and only the update is linear-Gaussian.
func EnsembleKalmanAnalysis(
	forecast [][]float64,
	y []float64,
	obsIndices []int,
	obsVar []float64,
	perturb [][]float64,
) [][]float64 {
	N := len(forecast)
	if N < 2 {
		panic("inference.EnsembleKalmanAnalysis: need at least 2 ensemble members")
	}
	d := len(forecast[0])
	p := len(obsIndices)
	if len(y) != p || len(obsVar) != p {
		panic("inference.EnsembleKalmanAnalysis: y and obsVar must match len(obsIndices)")
	}

	// Forecast ensemble mean.
	mean := make([]float64, d)
	for i := range forecast {
		for k := 0; k < d; k++ {
			mean[k] += forecast[i][k]
		}
	}
	for k := 0; k < d; k++ {
		mean[k] /= float64(N)
	}

	// Anomalies A (N×d) and their projection into observation space.
	anom := make([][]float64, N)
	for i := range forecast {
		anom[i] = make([]float64, d)
		for k := 0; k < d; k++ {
			anom[i][k] = forecast[i][k] - mean[k]
		}
	}

	inv := 1.0 / float64(N-1)

	// Pxy (d×p) and Pyy (p×p) from the ensemble anomalies.
	pxy := mat.NewDense(d, p, nil)
	pyy := mat.NewDense(p, p, nil)
	for k := 0; k < d; k++ {
		for j := 0; j < p; j++ {
			var s float64
			for i := 0; i < N; i++ {
				s += anom[i][k] * anom[i][obsIndices[j]]
			}
			pxy.Set(k, j, s*inv)
		}
	}
	for a := 0; a < p; a++ {
		for b := 0; b < p; b++ {
			var s float64
			for i := 0; i < N; i++ {
				s += anom[i][obsIndices[a]] * anom[i][obsIndices[b]]
			}
			v := s * inv
			if a == b {
				v += obsVar[a]
			}
			pyy.Set(a, b, v)
		}
	}

	// K = Pxy Pyy⁻¹, obtained as Kᵀ = Pyy⁻¹ Pxyᵀ (Pyy is symmetric positive
	// definite: the observation noise R makes it invertible even if the ensemble
	// covariance is rank-deficient).
	var kt mat.Dense
	if err := kt.Solve(pyy, pxy.T()); err != nil {
		// Fall back to a pseudo-inverse if Solve reports singularity (e.g. a
		// degenerate ensemble with zero-variance observation noise).
		var pinv mat.Dense
		if perr := pinv.Inverse(pyy); perr != nil {
			panic("inference.EnsembleKalmanAnalysis: innovation covariance is singular")
		}
		kt.Mul(&pinv, pxy.T())
	}
	// kt is p×d; gain[k][j] = kt[j][k].

	analysis := make([][]float64, N)
	for i := 0; i < N; i++ {
		analysis[i] = make([]float64, d)
		// Innovation for this member: y + εⁱ − H xᶠⁱ.
		innov := make([]float64, p)
		for j := 0; j < p; j++ {
			innov[j] = y[j] + perturb[i][j] - forecast[i][obsIndices[j]]
		}
		for k := 0; k < d; k++ {
			upd := forecast[i][k]
			for j := 0; j < p; j++ {
				upd += kt.At(j, k) * innov[j]
			}
			analysis[i][k] = upd
		}
	}
	return analysis
}

// EnsembleKalmanFilterIteration assimilates observations into a forecast ensemble
// with the stochastic Ensemble Kalman Filter. Its state is the analysis ensemble,
// flattened member-major: N members each of dimension d, so state width is N·d and
// member i occupies state[i·d : (i+1)·d].
//
// Wiring (the filter loop): a separate "forecast" partition propagates the previous
// analysis ensemble one step through the forward model (reading this partition's
// lag-1 state), and feeds the result here as the forecast_ensemble param
// (params_from_upstream, a within-step read). This partition then produces the
// analysis. The forecast partition's lag-1 read of the analysis is what breaks the
// cycle, exactly as the engine's cycle-breaking rule prescribes.
//
// Params:
//   - ensemble_size: N (number of members).
//   - state_dimension: d (per-member state width).
//   - forecast_ensemble: the propagated forecast, flattened member-major (N·d).
//   - latest_data_values: the observation y (length p).
//   - observation_indices: which state components y observes (length p). Defaults to
//     [0, 1, …, d−1] (the whole state observed) when absent.
//   - observation_noise_variance: diagonal of R (length p, or length 1 to broadcast).
//   - inflation: multiplicative forecast-spread inflation (optional, default 1.0),
//     applied to the forecast anomalies before the update to counter ensemble
//     collapse.
type EnsembleKalmanFilterIteration struct {
	sampler *rng.Sampler
}

func (e *EnsembleKalmanFilterIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	e.sampler = rng.New(settings.Iterations[partitionIndex].Seed)
}

func (e *EnsembleKalmanFilterIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	n := int(params.GetIndex("ensemble_size", 0))
	d := int(params.GetIndex("state_dimension", 0))
	flat := params.Get("forecast_ensemble")
	if len(flat) != n*d {
		panic("inference.EnsembleKalmanFilterIteration: forecast_ensemble width " +
			"must equal ensemble_size * state_dimension")
	}
	y := params.Get("latest_data_values")

	// Observation operator: an index selector into the state. Default to observing
	// the whole state.
	var obsIndices []int
	if raw, ok := params.GetOk("observation_indices"); ok {
		obsIndices = make([]int, len(raw))
		for j, v := range raw {
			obsIndices[j] = int(v)
		}
	} else {
		obsIndices = make([]int, d)
		for j := range obsIndices {
			obsIndices[j] = j
		}
	}
	p := len(obsIndices)
	if len(y) < p {
		panic("inference.EnsembleKalmanFilterIteration: latest_data_values shorter " +
			"than the number of observed components")
	}
	y = y[:p]

	// Observation-noise variance R (diagonal), broadcast a length-1 value.
	rawVar := params.Get("observation_noise_variance")
	obsVar := make([]float64, p)
	for j := 0; j < p; j++ {
		if len(rawVar) == 1 {
			obsVar[j] = rawVar[0]
		} else {
			obsVar[j] = rawVar[j]
		}
	}

	inflation := 1.0
	if infl, ok := params.GetOk("inflation"); ok {
		inflation = infl[0]
	}

	// Unflatten the forecast ensemble (member-major).
	forecast := make([][]float64, n)
	for i := 0; i < n; i++ {
		forecast[i] = make([]float64, d)
		copy(forecast[i], flat[i*d:(i+1)*d])
	}

	// Multiplicative inflation of the forecast spread about its mean.
	if inflation != 1.0 {
		mean := make([]float64, d)
		for i := 0; i < n; i++ {
			for k := 0; k < d; k++ {
				mean[k] += forecast[i][k]
			}
		}
		for k := 0; k < d; k++ {
			mean[k] /= float64(n)
		}
		scale := math.Sqrt(inflation)
		for i := 0; i < n; i++ {
			for k := 0; k < d; k++ {
				forecast[i][k] = mean[k] + scale*(forecast[i][k]-mean[k])
			}
		}
	}

	// Draw the per-member observation perturbations εⁱ ~ N(0, R).
	perturb := make([][]float64, n)
	for i := 0; i < n; i++ {
		perturb[i] = make([]float64, p)
		for j := 0; j < p; j++ {
			perturb[i][j] = e.sampler.Normal(0.0, math.Sqrt(obsVar[j]))
		}
	}

	analysis := EnsembleKalmanAnalysis(forecast, y, obsIndices, obsVar, perturb)

	// Flatten the analysis ensemble back to the state row.
	out := stateHistories[partitionIndex].GetNextStateRowToUpdate()
	for i := 0; i < n; i++ {
		copy(out[i*d:(i+1)*d], analysis[i])
	}
	return out
}
