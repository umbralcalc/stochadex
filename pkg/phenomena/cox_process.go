package phenomena

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// CoxProcessIteration defines an iteration for a Cox process.
type CoxProcessIteration struct {
	unitUniformDist           *distuv.Uniform
	rateProcessPartitionIndex int
}

func (c *CoxProcessIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	c.unitUniformDist = &distuv.Uniform{
		Min: 0.0,
		Max: 1.0,
		Src: rand.NewSource(settings.Seeds[partitionIndex]),
	}
	c.rateProcessPartitionIndex = int(
		settings.OtherParams[partitionIndex].
			IntParams["rate_process_partition_index"][0],
	)
}

func (c *CoxProcessIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	rates := stateHistories[c.rateProcessPartitionIndex].NextValues
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		if rates[i] > (rates[i]+
			(1.0/timestepsHistory.NextIncrement))*c.unitUniformDist.Rand() {
			values[i] = stateHistory.Values.At(0, i) + 1.0
		} else {
			values[i] = stateHistory.Values.At(0, i)
		}
	}
	return values
}
