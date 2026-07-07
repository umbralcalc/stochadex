package rugby

import (
	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// ConversionIteration models conversion attempts as Bernoulli trials
// that occur after each new try. State is cumulative:
// [home_conversions, away_conversions].
//
// Params:
//   - "conversion_probs": [home_prob, away_prob]
//   - "try_values": upstream from score_events (all 4 values; uses indices 0,1 for try counts)
//   - "score_events_partition": partition index (via params_as_partitions) for reading previous try counts
type ConversionIteration struct {
	uniformDist               *distuv.Uniform
	scoreEventsPartitionIndex int
}

func (c *ConversionIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	c.uniformDist = &distuv.Uniform{
		Min: 0.0,
		Max: 1.0,
		Src: rand.NewPCG(
			settings.Iterations[partitionIndex].Seed,
			settings.Iterations[partitionIndex].Seed,
		),
	}
	c.scoreEventsPartitionIndex = int(
		settings.Iterations[partitionIndex].Params.Get(
			"score_events_partition",
		)[0],
	)
}

func (c *ConversionIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	convProbs := params.Get("conversion_probs")
	tryValues := params.Get("try_values")

	currentHomeTries := tryValues[0]
	currentAwayTries := tryValues[1]

	// Previous try counts from score_events state history (row 0 = previous step,
	// since history hasn't been updated yet when Iterate runs).
	scoreHistory := stateHistories[c.scoreEventsPartitionIndex]
	prevHomeTries := scoreHistory.Values.At(0, 0)
	prevAwayTries := scoreHistory.Values.At(0, 1)

	newHomeTries := int(currentHomeTries - prevHomeTries)
	newAwayTries := int(currentAwayTries - prevAwayTries)

	// Previous cumulative conversions from own state history.
	prevHomeConv := stateHistories[partitionIndex].Values.At(0, 0)
	prevAwayConv := stateHistories[partitionIndex].Values.At(0, 1)

	homeConv := prevHomeConv
	awayConv := prevAwayConv

	for i := 0; i < newHomeTries; i++ {
		if c.uniformDist.Rand() < convProbs[0] {
			homeConv += 1.0
		}
	}
	for i := 0; i < newAwayTries; i++ {
		if c.uniformDist.Rand() < convProbs[1] {
			awayConv += 1.0
		}
	}

	return []float64{homeConv, awayConv}
}
