package continuous

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// JumpDistribution defines the interface that must be implemented
// to provide a distribution to generate sudden 'jumps' from. This
// is used in compound Poisson processes and drift-jump-diffusions.
type JumpDistribution interface {
	Configure(partitionIndex int, settings *simulator.Settings)
	NewJump(params *simulator.Params, valueIndex int) float64
}

// GammaJumpDistribution jumps with samples drawn from a gamma distribution.
type GammaJumpDistribution struct {
	dist *distuv.Gamma
}

func (g *GammaJumpDistribution) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	g.dist = &distuv.Gamma{
		Alpha: 1.0,
		Beta:  1.0,
		Src: rand.NewSource(
			settings.Iterations[partitionIndex].Seed,
		),
	}
}

func (g *GammaJumpDistribution) NewJump(
	params *simulator.Params,
	valueIndex int,
) float64 {
	g.dist.Alpha = params.GetIndex("gamma_alphas", valueIndex)
	g.dist.Beta = params.GetIndex("gamma_betas", valueIndex)
	return g.dist.Rand()
}

// CompoundPoissonProcessIteration defines an iteration for a compound
// Poisson process.
type CompoundPoissonProcessIteration struct {
	JumpDist        JumpDistribution
	unitUniformDist *distuv.Uniform
}

func (c *CompoundPoissonProcessIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	c.JumpDist.Configure(partitionIndex, settings)
	c.unitUniformDist = &distuv.Uniform{
		Min: 0.0,
		Max: 1.0,
		Src: rand.NewSource(settings.Iterations[partitionIndex].Seed),
	}
}

func (c *CompoundPoissonProcessIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		if params.GetIndex("rates", i) > (params.GetIndex("rates", i)+
			(1.0/timestepsHistory.NextIncrement))*c.unitUniformDist.Rand() {
			values[i] = stateHistory.Values.At(0, i) + c.JumpDist.NewJump(params, i)
		} else {
			values[i] = stateHistory.Values.At(0, i)
		}
	}
	return values
}
