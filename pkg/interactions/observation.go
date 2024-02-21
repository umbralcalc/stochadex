package interactions

import (
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// GaussianNoiseStateObservationIteration simply returns the state
// exactly how it is but with a Gaussian noise applied on top of the values.
type GaussianNoiseStateObservationIteration struct {
	unitNormalDist     *distuv.Normal
	partitionToObserve int
}

func (g *GaussianNoiseStateObservationIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	g.unitNormalDist = &distuv.Normal{
		Mu:    0.0,
		Sigma: 1.0,
		Src:   rand.NewSource(settings.Seeds[partitionIndex]),
	}
	g.partitionToObserve = int(settings.OtherParams[partitionIndex].
		IntParams["partition_to_observe"][0])
}

func (g *GaussianNoiseStateObservationIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	noisyValues := make([]float64, 0)
	stateValues := params.FloatParams["partition_"+strconv.Itoa(g.partitionToObserve)]
	for i, stateValue := range stateValues {
		g.unitNormalDist.Sigma = params.FloatParams["observation_noise_variances"][i]
		noisyValues = append(noisyValues, stateValue+g.unitNormalDist.Rand())
	}
	return noisyValues
}
