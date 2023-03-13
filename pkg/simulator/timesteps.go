package simulator

import (
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// TimestepFunction is the interface that must be implemented for a function
// which evolves the time variable of the stochastic process.
type TimestepFunction interface {
	Iterate(timestepsHistory *TimestepsHistory) *TimestepsHistory
}

// ConstantNoMemoryTimestepFunction iterates the timestep by a constant stepsize
// and records no memory of previous steps.
type ConstantNoMemoryTimestepFunction struct {
	Stepsize float64
}

func (t *ConstantNoMemoryTimestepFunction) Iterate(
	timestepsHistory *TimestepsHistory,
) *TimestepsHistory {
	// update only the latest state in the history
	timestepsHistory.Values.SetVec(0, timestepsHistory.Values.AtVec(0)+t.Stepsize)
	return timestepsHistory
}

// ExponentialDistributionNoMemoryTimestepFunction iterates the timestep by a
// new sample drawn from an exponential distribution with hyperparameters set by
// Mean and Seed. This version records no memory of previous steps.
type ExponentialDistributionNoMemoryTimestepFunction struct {
	Mean         float64
	Seed         uint64
	distribution distuv.Exponential
}

func (t *ExponentialDistributionNoMemoryTimestepFunction) Iterate(
	timestepsHistory *TimestepsHistory,
) *TimestepsHistory {
	// update only the latest state in the history
	timestepsHistory.Values.SetVec(
		0,
		timestepsHistory.Values.AtVec(0)+t.distribution.Rand(),
	)
	return timestepsHistory
}

// New ExponentialDistributionNoMemoryTimestepFunction creates a new
// ExponentialDistributionNoMemoryTimestepFunction given a mean and seed.
func NewExponentialDistributionNoMemoryTimestepFunction(
	mean float64,
	seed uint64,
) *ExponentialDistributionNoMemoryTimestepFunction {
	return &ExponentialDistributionNoMemoryTimestepFunction{
		Mean:         mean,
		Seed:         seed,
		distribution: distuv.Exponential{Rate: 1.0 / mean, Src: rand.NewSource(seed)},
	}
}

// ExponentialDistributionTimestepFunction iterates the timestep by a new sample
// drawn from an exponential distribution with hyperparameters set by Mean and Seed.
// This version updates a memory of previous steps in the provided TimestepsHistory.
type ExponentialDistributionTimestepFunction struct {
	Mean         float64
	Seed         uint64
	distribution distuv.Exponential
}

func (t *ExponentialDistributionTimestepFunction) Iterate(
	timestepsHistory *TimestepsHistory,
) *TimestepsHistory {
	// iterate over the history of timesteps and shift them back one
	for i := 1; i < timestepsHistory.StateHistoryDepth; i++ {
		timestepsHistory.Values.SetVec(i, timestepsHistory.Values.AtVec(i-1))
	}
	// now update the latest state in the history
	timestepsHistory.Values.SetVec(
		0,
		timestepsHistory.Values.AtVec(0)+t.distribution.Rand(),
	)
	return timestepsHistory
}

// NewExponentialDistributionTimestepFunction creates a new
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
