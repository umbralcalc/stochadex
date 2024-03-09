package observations

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// BinomialStaticPartialStateObservationIteration observes each count value in the
// state with a binomial probability - emulating a sequence of Bernoulli trials.
type BinomialStaticPartialStateObservationIteration struct {
	binomialDist       *distuv.Binomial
	partitionToObserve int
}

func (b *BinomialStaticPartialStateObservationIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	b.binomialDist = &distuv.Binomial{
		N:   0,
		P:   1.0,
		Src: rand.NewSource(settings.Seeds[partitionIndex]),
	}
	b.partitionToObserve = int(settings.OtherParams[partitionIndex].
		IntParams["partition_to_observe"][0])
}

func (b *BinomialStaticPartialStateObservationIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	outputValues := make([]float64, 0)
	stateValues := stateHistories[b.partitionToObserve].NextValues
	probs := params.FloatParams["state_value_observation_probs"]
	for i, index := range params.IntParams["state_value_observation_indices"] {
		b.binomialDist.N = stateValues[index]
		b.binomialDist.P = probs[i]
		outputValues = append(outputValues, b.binomialDist.Rand())
	}
	return outputValues
}
