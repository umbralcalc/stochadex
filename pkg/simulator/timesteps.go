package simulator

import (
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// TimestepFunction is the interface that must be implemented for a function
// which evaluates the next increment to the time variable of the simulation.
type TimestepFunction interface {
	NextIncrement(
		timestepsHistory *CumulativeTimestepsHistory,
	) float64
}

// ConstantTimestepFunction iterates the timestep by a constant stepsize.
type ConstantTimestepFunction struct {
	Stepsize float64
}

func (t *ConstantTimestepFunction) NextIncrement(
	timestepsHistory *CumulativeTimestepsHistory,
) float64 {
	return t.Stepsize
}

// ExponentialDistributionTimestepFunction iterates the timestep by a new sample
// drawn from an exponential distribution with hyperparameters set by Mean and Seed.
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

// New ExponentialDistributionTimestepFunction creates a new
// ExponentialDistributionTimestepFunction given a mean and seed.
func NewExponentialDistributionTimestepFunction(
	mean float64,
	seed uint64,
) *ExponentialDistributionTimestepFunction {
	return &ExponentialDistributionTimestepFunction{
		Mean:         mean,
		Seed:         seed,
		distribution: distuv.Exponential{Rate: 1.0 / mean, Src: rand.NewSource(seed)},
	}
}
