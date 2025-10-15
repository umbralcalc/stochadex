package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// PartitionEventFunction emits an event value from the latest state of
// another partition.
//
// Usage hints:
//   - Provide: "event_partition_index" and "event_state_value_index".
func PartitionEventFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return []float64{stateHistories[int(
		params.GetIndex("event_partition_index", 0))].Values.At(
		0,
		int(params.GetIndex("event_state_value_index", 0)),
	)}
}

// ParamsEventFunction emits an event value from the "event" params.
func ParamsEventFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return params.Get("event")
}

// ValuesChangingEventsIteration calls and outputs from an iteration in the
// map keyed by an event. If no event key matches, it returns previous values
// or optional "default_values".
//
// Usage hints:
//   - Set EventIteration to produce the event key; provide IterationByEvent map.
//   - Optional: set "default_values" to override fallback behaviour.
type ValuesChangingEventsIteration struct {
	EventIteration   simulator.Iteration
	IterationByEvent map[float64]simulator.Iteration
}

func (v *ValuesChangingEventsIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	v.EventIteration.Configure(partitionIndex, settings)
	for _, iteration := range v.IterationByEvent {
		iteration.Configure(partitionIndex, settings)
	}
}

func (v *ValuesChangingEventsIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	if iteration, ok := v.IterationByEvent[v.EventIteration.Iterate(
		params,
		partitionIndex,
		stateHistories,
		timestepsHistory,
	)[0]]; ok {
		return iteration.Iterate(
			params,
			partitionIndex,
			stateHistories,
			timestepsHistory,
		)
	} else {
		if defaults, ok := params.GetOk("default_values"); ok {
			return defaults
		} else {
			return stateHistories[partitionIndex].Values.RawRowView(0)
		}
	}
}
