package observations

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// StaticPartialStateObservationIteration only observes the state partially
// throughout all time.
type StaticPartialStateObservationIteration struct {
}

func (p *StaticPartialStateObservationIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (p *StaticPartialStateObservationIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	outputValues := make([]float64, 0)
	stateValues := params.FloatParams["values_to_observe"]
	for _, index := range params.IntParams["state_value_observation_indices"] {
		outputValues = append(outputValues, stateValues[index])
	}
	return outputValues
}
