package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// PartitionEventFunction provides the capability to set events using
// the most recent value from the state history of another partition.
func PartitionEventFunction(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) int {
	return int(stateHistories[int(
		params["event_partition_index"][0])].Values.At(
		0,
		int(params["event_state_value_index"][0]),
	))
}

// ParamsEventFunction provides the capability to set events using
// the "event" params.
func ParamsEventFunction(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) int {
	return int(params["event"][0])
}

// ValuesChangingEventsIteration defines an iteration which calls and
// outputs from an iteration in the map if its keyed event occurs.
// If none of the events happen (i.e., the event key doesn't exist in
// the map) either the previous values or some optionally-specified
// default values are used as the output.
type ValuesChangingEventsIteration struct {
	EventFunction func(
		params simulator.Params,
		partitionIndex int,
		stateHistories []*simulator.StateHistory,
		timestepsHistory *simulator.CumulativeTimestepsHistory,
	) int
	IterationByEvent map[int]simulator.Iteration
}

func (v *ValuesChangingEventsIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	for _, iteration := range v.IterationByEvent {
		iteration.Configure(partitionIndex, settings)
	}
}

func (v *ValuesChangingEventsIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	if iteration, ok := v.IterationByEvent[v.EventFunction(
		params,
		partitionIndex,
		stateHistories,
		timestepsHistory,
	)]; ok {
		return iteration.Iterate(
			params,
			partitionIndex,
			stateHistories,
			timestepsHistory,
		)
	} else {
		if defaults, ok := params["default_values"]; ok {
			return defaults
		} else {
			return stateHistories[partitionIndex].Values.RawRowView(0)
		}
	}
}
