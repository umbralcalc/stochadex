package phenomena

import (
	"math"
	"strconv"

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
		Src:   rand.NewSource(settings.Seeds[partitionIndex]),
	}
}

func (d *DriftDiffusionIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	driftCoefficients := params.FloatParams["partition_"+
		strconv.Itoa(int(params.IntParams["drift_coefficients_partition"][0]))]
	diffusionCoefficients := params.FloatParams["partition_"+
		strconv.Itoa(int(params.IntParams["diffusion_coefficients_partition"][0]))]
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		values[i] = stateHistory.Values.At(0, i) +
			(driftCoefficients[i] * timestepsHistory.NextIncrement) +
			diffusionCoefficients[i]*math.Sqrt(
				timestepsHistory.NextIncrement)*d.unitNormalDist.Rand()
	}
	return values
}
