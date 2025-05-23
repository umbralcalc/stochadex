package continuous

import (
	"math"

	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// OrnsteinUhlenbeckIteration defines an iteration for an Ornstein-Uhlenbeck
// process.
type OrnsteinUhlenbeckIteration struct {
	unitNormalDist *distuv.Normal
}

func (o *OrnsteinUhlenbeckIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	o.unitNormalDist = &distuv.Normal{
		Mu:    0.0,
		Sigma: 1.0,
		Src: rand.NewPCG(
			settings.Iterations[partitionIndex].Seed,
			settings.Iterations[partitionIndex].Seed,
		),
	}
}

func (o *OrnsteinUhlenbeckIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		values[i] = stateHistory.Values.At(0, i) +
			params.GetIndex("thetas", i)*(params.GetIndex("mus", i)-
				stateHistory.Values.At(0, i))*timestepsHistory.NextIncrement +
			params.GetIndex("sigmas", i)*math.Sqrt(
				timestepsHistory.NextIncrement)*o.unitNormalDist.Rand()
	}
	return values
}
