package observations

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// GaussianStaticStateObservationIteration simply returns the state
// exactly how it is but with a Gaussian noise applied on top of the values.
type GaussianStaticStateObservationIteration struct {
	unitNormalDist *distuv.Normal
}

func (g *GaussianStaticStateObservationIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	g.unitNormalDist = &distuv.Normal{
		Mu:    0.0,
		Sigma: 1.0,
		Src:   rand.NewSource(settings.Seeds[partitionIndex]),
	}
}

func (g *GaussianStaticStateObservationIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	noisyValues := make([]float64, 0)
	stateValues := params["values_to_observe"]
	for i, stateValue := range stateValues {
		g.unitNormalDist.Sigma = params["observation_noise_variances"][i]
		noisyValues = append(noisyValues, stateValue+g.unitNormalDist.Rand())
	}
	return noisyValues
}
