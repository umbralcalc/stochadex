package phenomena

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// CompoundPoissonProcessJumpDistribution defines the interface that
// must be implemented to provide a distribution to generate new
// 'jumps' from in the compound Poisson process.
type CompoundPoissonProcessJumpDistribution interface {
	NewJump(params *simulator.OtherParams, stateElement int) float64
}

// CompoundPoissonProcessIteration defines an iteration for a compound
// Poisson process.
type CompoundPoissonProcessIteration struct {
	unitUniformDist *distuv.Uniform
	jumpDist        CompoundPoissonProcessJumpDistribution
}

func (c *CompoundPoissonProcessIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	c.unitUniformDist = &distuv.Uniform{
		Min: 0.0,
		Max: 1.0,
		Src: rand.NewSource(settings.Seeds[partitionIndex]),
	}
}

func (c *CompoundPoissonProcessIteration) Iterate(
	otherParams *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		if otherParams.FloatParams["rates"][i] > (otherParams.FloatParams["rates"][i]+
			(1.0/timestepsHistory.NextIncrement))*c.unitUniformDist.Rand() {
			values[i] = stateHistory.Values.At(0, i) + c.jumpDist.NewJump(otherParams, i)
		} else {
			values[i] = stateHistory.Values.At(0, i)
		}
	}
	return values
}
