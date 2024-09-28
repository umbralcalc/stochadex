package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ValuesChangingEventsIteration defines an iteration which calls and
// outputs from an iteration in the map if its keyed event occurs.
// If none of the events happen (i.e., the event key doesn't exist in
// the map) either the previous values or some optionally-specified
// default values are used as the output.
type ValuesChangingEventsIteration struct {
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
	// Provide the optional alternative capability to set events using
	// the most recent value from the state history of another partition
	if p, ok := params["event_partition_index"]; ok {
		params["event"] = []float64{
			stateHistories[int(p[0])].Values.At(
				0,
				int(params["event_state_value_index"][0]),
			),
		}
	}
	if iteration, ok := v.IterationByEvent[int(params["event"][0])]; ok {
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
