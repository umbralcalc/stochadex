package phenomena

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// OrnsteinUhlenbeckIteration defines an iteration for an Ornstein-Uhlenbeck
// process.
type OrnsteinUhlenbeckIteration struct {
	unitNormalDist *distuv.Normal
}

func (o *OrnsteinUhlenbeckIteration) Iterate(
	otherParams *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		values[i] = stateHistory.Values.At(0, i) +
			otherParams.FloatParams["thetas"][i]*(otherParams.FloatParams["mus"][i]-
				stateHistory.Values.At(0, i))*timestepsHistory.NextIncrement +
			otherParams.FloatParams["sigmas"][i]*math.Sqrt(
				timestepsHistory.NextIncrement)*o.unitNormalDist.Rand()
	}
	return values
}

// NewOrnsteinUhlenbeckIteration creates a new OrnsteinUhlenbeckIteration given a seed.
func NewOrnsteinUhlenbeckIteration(seed uint64) *OrnsteinUhlenbeckIteration {
	return &OrnsteinUhlenbeckIteration{
		unitNormalDist: &distuv.Normal{
			Mu:    0.0,
			Sigma: 1.0,
			Src:   rand.NewSource(seed),
		},
	}
}
