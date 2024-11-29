package continuous

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// DriftDiffusionIteration defines an iteration for any general
// drift-diffusion process.
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
		Src:   rand.NewSource(settings.Iterations[partitionIndex].Seed),
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
