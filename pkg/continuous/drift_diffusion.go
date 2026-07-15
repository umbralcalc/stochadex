package continuous

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// DriftDiffusionIteration steps a general drift–diffusion SDE per dimension.
//
// Usage hints:
//   - Provide per-dimension params: "drift_coefficients" and "diffusion_coefficients".
//   - The update uses x_{t+dt} = x_t + drift*dt + diffusion*sqrt(dt)*N(0,1).
//   - Ensure the timestep function is configured; diffusion scales with sqrt(dt).
//   - Seed is taken from the partition's Settings for reproducibility.
type DriftDiffusionIteration struct {
	sampler *rng.Sampler
}

func (d *DriftDiffusionIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	d.sampler = rng.New(settings.Iterations[partitionIndex].Seed)
}

func (d *DriftDiffusionIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	driftCoefficients := params.Get("drift_coefficients")
	diffusionCoefficients := params.Get("diffusion_coefficients")
	values := stateHistory.GetNextStateRowToUpdate()
	for i := range values {
		values[i] += (driftCoefficients[i] * timestepsHistory.NextIncrement) +
			diffusionCoefficients[i]*math.Sqrt(
				timestepsHistory.NextIncrement)*d.sampler.NormFloat64()
	}
	return values
}
