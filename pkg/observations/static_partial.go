package observations

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// StaticPartialStateObservationIteration only observes the state partially
// throughout all time.
type StaticPartialStateObservationIteration struct {
	partitionToObserve int
}

func (p *StaticPartialStateObservationIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	p.partitionToObserve = int(settings.OtherParams[partitionIndex].
		IntParams["partition_to_observe"][0])
}

func (p *StaticPartialStateObservationIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	outputValues := make([]float64, 0)
	stateValues := stateHistories[p.partitionToObserve].NextValues
	for _, index := range params.IntParams["state_value_observation_indices"] {
		outputValues = append(outputValues, stateValues[index])
	}
	return outputValues
}
