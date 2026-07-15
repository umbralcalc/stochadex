package discrete

import (
	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// CoxProcessIteration defines an iteration for a Cox process.
type CoxProcessIteration struct {
	sampler *rng.Sampler
}

func (c *CoxProcessIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	c.sampler = rng.New(settings.Iterations[partitionIndex].Seed)
}

func (c *CoxProcessIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	rates := params.Get("rates")
	values := stateHistory.GetNextStateRowToUpdate()
	for i := range values {
		if rates[i] > (rates[i]+
			(1.0/timestepsHistory.NextIncrement))*c.sampler.Float64() {
			values[i] += 1.0
		}
	}
	return values
}
