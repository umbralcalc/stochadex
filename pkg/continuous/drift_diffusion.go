package continuous

import (
	"math"

	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// DriftDiffusionIteration steps a general drift–diffusion SDE per dimension.
//
// Usage hints:
//   - Provide per-dimension params: "drift_coefficients" and "diffusion_coefficients".
//   - The update uses x_{t+dt} = x_t + drift*dt + diffusion*sqrt(dt)*N(0,1).
//   - Ensure the timestep function is configured; diffusion scales with sqrt(dt).
//   - Seed is taken from the partition's Settings for reproducibility.
type DriftDiffusionIteration struct {
	unitNormalDist *distuv.Normal
}

func (d *DriftDiffusionIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	d.unitNormalDist = &distuv.Normal{
		Mu:    0.0,
		Sigma: 1.0,
		Src: rand.NewPCG(
			settings.Iterations[partitionIndex].Seed,
			settings.Iterations[partitionIndex].Seed,
		),
	}
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
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		values[i] = stateHistory.Values.At(0, i) +
			(driftCoefficients[i] * timestepsHistory.NextIncrement) +
			diffusionCoefficients[i]*math.Sqrt(
				timestepsHistory.NextIncrement)*d.unitNormalDist.Rand()
	}
	return values
}
