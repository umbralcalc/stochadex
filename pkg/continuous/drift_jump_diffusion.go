package continuous

import (
	"math"

	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// DriftJumpDiffusionIteration steps a general drift–jump–diffusion process.
//
// Usage hints:
//   - Provide per-dimension params: "drift_coefficients", "diffusion_coefficients",
//     and "jump_rates"; also set a JumpDistribution implementation (e.g. Gamma).
//   - Uses x_{t+dt} = x_t + drift*dt + diffusion*sqrt(dt)*N(0,1) + jumps.
//   - Jumps occur with Poisson hazard approx. rate*dt; set dt via timestep config.
//   - Seed for RNGs derives from the partition's Settings for reproducibility.
type DriftJumpDiffusionIteration struct {
	JumpDist       JumpDistribution
	normalSampler  *rng.Sampler
	uniformSampler *rng.Sampler
}

func (d *DriftJumpDiffusionIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	r := rand.New(rand.NewPCG(
		settings.Iterations[partitionIndex].Seed,
		settings.Iterations[partitionIndex].Seed,
	))
	d.normalSampler = rng.NewFromSource(rand.NewPCG(uint64(r.IntN(1e8)), uint64(r.IntN(1e8))))
	d.uniformSampler = rng.NewFromSource(rand.NewPCG(uint64(r.IntN(1e8)), uint64(r.IntN(1e8))))
	d.JumpDist.Configure(partitionIndex, settings)
}

func (d *DriftJumpDiffusionIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	driftCoefficients := params.Get("drift_coefficients")
	diffusionCoefficients := params.Get("diffusion_coefficients")
	jumpRates := params.Get("jump_rates")
	values := stateHistory.GetNextStateRowToUpdate()
	for i := range values {
		values[i] += (driftCoefficients[i] * timestepsHistory.NextIncrement) +
			diffusionCoefficients[i]*math.Sqrt(
				timestepsHistory.NextIncrement)*d.normalSampler.NormFloat64()
		if jumpRates[i] > (jumpRates[i]+
			(1.0/timestepsHistory.NextIncrement))*d.uniformSampler.Float64() {
			values[i] += d.JumpDist.NewJump(params, i)
		}
	}
	return values
}
