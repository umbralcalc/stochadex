package anglersim

import (
	"math"

	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// RickerIteration implements a stochastic Ricker population dynamics
// model with environmental covariates and an optional Allee effect.
// It operates in log-density space for numerical stability.
//
// State: [log_density]
//
// Params:
//   - growth_rate:            [r0]       — baseline intrinsic growth rate
//   - density_dependence:     [alpha]    — strength of density-dependent mortality
//   - covariate_coefficients: [β1…βk]   — linear effects of environmental covariates
//   - covariates:             [c1…ck]   — current covariate values (from upstream)
//   - process_noise_sd:       [sigma]    — standard deviation of process noise
//   - allee_effect:           [gamma]    — Allee effect strength (0 = no Allee effect)
//
// Dynamics (per timestep):
//
//	N_t = exp(log(N_t))
//	allee = 1 − exp(−γ·N_t)       [depensatory multiplier on growth]
//	log(N_{t+1}) = log(N_t) + r0·allee + Σ(βi·ci) − α·N_t + N(0,σ²)
//
// When γ=0 this reduces to the standard Ricker model (allee=1 always).
// When γ>0, growth is suppressed at low density, creating a minimum
// viable population threshold below which recovery is unlikely.
type RickerIteration struct {
	rng *rand.Rand
}

func (r *RickerIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	r.rng = rand.New(rand.NewPCG(
		settings.Iterations[partitionIndex].Seed,
		settings.Iterations[partitionIndex].Seed,
	))
}

func (r *RickerIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	logN := stateHistories[partitionIndex].Values.At(0, 0)

	r0 := params.Map["growth_rate"][0]
	alpha := params.Map["density_dependence"][0]
	sigma := params.Map["process_noise_sd"][0]

	// Environmental covariate effect
	betas := params.Map["covariate_coefficients"]
	covs := params.Map["covariates"]
	envEffect := 0.0
	k := len(betas)
	if len(covs) < k {
		k = len(covs)
	}
	for i := 0; i < k; i++ {
		envEffect += betas[i] * covs[i]
	}

	// Allee effect: depensatory growth multiplier
	// allee = 1 - exp(-gamma * N), so growth → 0 as N → 0
	gamma := 0.0
	if v, ok := params.Map["allee_effect"]; ok && len(v) > 0 {
		gamma = v[0]
	}

	// Ricker dynamics in log space
	density := math.Exp(logN)
	alleeMultiplier := 1.0
	if gamma > 0 {
		alleeMultiplier = 1.0 - math.Exp(-gamma*density)
	}
	logNNext := logN + r0*alleeMultiplier + envEffect - alpha*density

	// Process noise
	if sigma > 0 {
		logNNext += r.rng.NormFloat64() * sigma
	}

	return []float64{logNNext}
}
