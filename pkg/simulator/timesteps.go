package simulator

import (
	"math/rand/v2"

	"gonum.org/v1/gonum/stat/distuv"
)

// TimestepFunction computes the next time increment.
type TimestepFunction interface {
	NextIncrement(
		timestepsHistory *CumulativeTimestepsHistory,
	) float64
}

// ConstantTimestepFunction uses a fixed stepsize.
type ConstantTimestepFunction struct {
	Stepsize float64
}

func (t *ConstantTimestepFunction) NextIncrement(
	timestepsHistory *CumulativeTimestepsHistory,
) float64 {
	return t.Stepsize
}

// ExponentialDistributionTimestepFunction draws dt from an exponential
// distribution parameterised by Mean and Seed.
type ExponentialDistributionTimestepFunction struct {
	Mean         float64
	Seed         uint64
	distribution distuv.Exponential
}

func (t *ExponentialDistributionTimestepFunction) NextIncrement(
	timestepsHistory *CumulativeTimestepsHistory,
) float64 {
	return t.distribution.Rand()
}

// NewExponentialDistributionTimestepFunction constructs an exponential-dt
// timestep function given mean and seed.
func NewExponentialDistributionTimestepFunction(
	mean float64,
	seed uint64,
) *ExponentialDistributionTimestepFunction {
	return &ExponentialDistributionTimestepFunction{
		Mean: mean,
		Seed: seed,
		distribution: distuv.Exponential{
			Rate: 1.0 / mean,
			Src:  rand.NewPCG(seed, seed),
		},
	}
}
