package observations

import (
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// GaussianStaticPartialStateObservationIteration partially observes the state
// values with a Gaussian noise applied to them.
type GaussianStaticPartialStateObservationIteration struct {
	unitNormalDist     *distuv.Normal
	partitionToObserve int
}

func (g *GaussianStaticPartialStateObservationIteration) Configure(
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

func (g *GaussianStaticPartialStateObservationIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	outputValues := make([]float64, 0)
	stateValues := params.FloatParams["partition_"+strconv.Itoa(g.partitionToObserve)]
	for i, index := range params.IntParams["state_value_observation_indices"] {
		g.unitNormalDist.Sigma = params.FloatParams["observation_noise_variances"][i]
		outputValues = append(outputValues, stateValues[index]+g.unitNormalDist.Rand())
	}
	return outputValues
}
