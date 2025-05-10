package continuous

import (
	"math"

	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// DriftJumpDiffusionIteration defines an iteration for any general
// drift-jump-diffusion process.
type DriftJumpDiffusionIteration struct {
	JumpDist        JumpDistribution
	unitNormalDist  *distuv.Normal
	unitUniformDist *distuv.Uniform
}

func (d *DriftJumpDiffusionIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	r := rand.New(rand.NewPCG(
		settings.Iterations[partitionIndex].Seed,
		settings.Iterations[partitionIndex].Seed,
	))
	d.unitNormalDist = &distuv.Normal{
		Mu:    0.0,
		Sigma: 1.0,
		Src:   rand.NewPCG(uint64(r.IntN(1e8)), uint64(r.IntN(1e8))),
	}
	d.unitUniformDist = &distuv.Uniform{
		Min: 0.0,
		Max: 1.0,
		Src: rand.NewPCG(uint64(r.IntN(1e8)), uint64(r.IntN(1e8))),
	}
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
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		values[i] = stateHistory.Values.At(0, i) +
			(driftCoefficients[i] * timestepsHistory.NextIncrement) +
			diffusionCoefficients[i]*math.Sqrt(
				timestepsHistory.NextIncrement)*d.unitNormalDist.Rand()
		if params.GetIndex("jump_rates", i) > (params.GetIndex("jump_rates", i)+
			(1.0/timestepsHistory.NextIncrement))*d.unitUniformDist.Rand() {
			values[i] += d.JumpDist.NewJump(params, i)
		}
	}
	return values
}
