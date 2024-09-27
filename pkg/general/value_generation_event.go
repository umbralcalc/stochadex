package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ValueGenerationEventIteration defines an iteration which calls and
// outputs from the user-specified ValueIteration according to an boolean
// param event trigger. If the event doesn't happen, the specified default
// values are used as the output.
type ValueGenerationEventIteration struct {
	ValueIteration simulator.Iteration
}

func (v *ValueGenerationEventIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	v.ValueIteration.Configure(partitionIndex, settings)
}

func (v *ValueGenerationEventIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	// Provide the optional alternative capability to set event trigger using
	// the most recent value from the state history of another partition
	if p, ok := params["event_occurred_partition_index"]; ok {
		params["event_occurred"] = []float64{
			stateHistories[int(p[0])].Values.At(
				0,
				int(params["event_occurred_state_value_index"][0]),
			),
		}
	}
	switch params["event_occurred"][0] {
	case 0:
		return params["default_values"]
	case 1:
		return v.ValueIteration.Iterate(
			params,
			partitionIndex,
			stateHistories,
			timestepsHistory,
		)
	default:
		panic("boolean 'event_occurred' param was not 0 or 1")
	}
}
