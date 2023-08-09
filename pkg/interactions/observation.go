package interactions

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// StateObservation is the interface that must be implemented in order
// for the Agent to learn what the state is from the simulation.
type StateObservation interface {
	Configure(partitionIndex int, settings *simulator.LoadSettingsConfig)
	Observe(
		params *simulator.OtherParams,
		partitionIndex int,
		stateHistories []*simulator.StateHistory,
		timestepsHistory *simulator.CumulativeTimestepsHistory,
	) []float64
}

// ExactStateObservation simply returns the state exactly how it is.
type ExactStateObservation struct{}

func (e *ExactStateObservation) Configure(
	partitionIndex int,
	settings *simulator.LoadSettingsConfig,
) {
}

func (e *ExactStateObservation) Observe(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return stateHistories[partitionIndex].Values.RawRowView(0)
}

// GaussianNoiseStateObservation simply returns the state exactly how it is
// but with a Gaussian noise applied on top of the values.
type GaussianNoiseStateObservation struct {
	unitNormalDist *distuv.Normal
}

func (g *GaussianNoiseStateObservation) Configure(
	partitionIndex int,
	settings *simulator.LoadSettingsConfig,
) {
	g.unitNormalDist = &distuv.Normal{
		Mu:    0.0,
		Sigma: 1.0,
		Src:   rand.NewSource(settings.Seeds[partitionIndex]),
	}
}

func (g *GaussianNoiseStateObservation) Observe(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	noisyValues := make([]float64, 0)
	for i := 0; i < stateHistories[partitionIndex].StateWidth; i++ {
		g.unitNormalDist.Sigma = params.FloatParams["observation_noise_variances"][i]
		noisyValues = append(
			noisyValues,
			stateHistories[partitionIndex].Values.At(0, i)+g.unitNormalDist.Rand(),
		)
	}
	return noisyValues
}
