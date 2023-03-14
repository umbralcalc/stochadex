package simulator

import (
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// TimestepFunction is the interface that must be implemented for a function
// which gets the next increment to the time variable of the stochastic process.
type TimestepFunction interface {
	NextIncrement(timestepsHistory *TimestepsHistory) *TimestepsHistory
}

// ConstantTimestepFunction iterates the timestep by a constant stepsize.
type ConstantTimestepFunction struct {
	Stepsize float64
}

func (t *ConstantTimestepFunction) NextIncrement(
	timestepsHistory *TimestepsHistory,
) *TimestepsHistory {
	timestepsHistory.NextIncrement = t.Stepsize
	return timestepsHistory
}

// ExponentialDistributionTimestepFunction iterates the timestep by a new sample
// drawn from an exponential distribution with hyperparameters set by Mean and Seed.
type ExponentialDistributionTimestepFunction struct {
	Mean         float64
	Seed         uint64
	distribution distuv.Exponential
}

func (t *ExponentialDistributionTimestepFunction) NextIncrement(
	timestepsHistory *TimestepsHistory,
) *TimestepsHistory {
	timestepsHistory.NextIncrement = t.distribution.Rand()
	return timestepsHistory
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
