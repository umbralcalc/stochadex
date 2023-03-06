package simulator

import (
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

type TimestepFunction interface {
	Iterate(timestepsHistory *TimestepsHistory) *TimestepsHistory
}

type ConstantNoMemoryTimestepFunction struct {
	Stepsize float64
}

func (t *ConstantNoMemoryTimestepFunction) Iterate(
	timestepsHistory *TimestepsHistory,
) *TimestepsHistory {
	// update only the latest state in the history
	timestepsHistory.Values.SetVec(0, t.Stepsize)
	return timestepsHistory
}

type ExponentialDistributionNoMemoryTimestepFunction struct {
	Mean         float64
	Seed         uint64
	distribution distuv.Exponential
}

func (t *ExponentialDistributionNoMemoryTimestepFunction) Iterate(
	timestepsHistory *TimestepsHistory,
) *TimestepsHistory {
	// update only the latest state in the history
	timestepsHistory.Values.SetVec(0, t.distribution.Rand())
	return timestepsHistory
}

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
	timestepsHistory.Values.SetVec(0, t.distribution.Rand())
	return timestepsHistory
}

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
