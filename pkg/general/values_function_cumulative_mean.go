package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ValuesFunctionCumulativeMeanIteration is the running mean of a function's
// values over every row of the data it reads: the data partition's initial
// row and each step's row after it, all weighted equally. It costs the same
// each step however long the run, so it suits estimation over long data, where
// a rolling window as long as the data would cost a pass over the window every
// step.
//
// Usage hints:
//   - Provide Function that accepts a history index (-1 for latest) and returns
//     a vector matching the partition state width; only -1 is asked for.
//   - Set init_state_values to the function's value on the data's initial row:
//     the mean of that one row. Each step then folds in one more row, the
//     latest, so at step k the state is the mean of k+1 rows.
//   - The count comes from the run's step number, so the run must start at
//     step 0 with the data at its initial row.
type ValuesFunctionCumulativeMeanIteration struct {
	Function func(
		params *simulator.Params,
		partitionIndex int,
		stateHistories []*simulator.StateHistory,
		stateHistoryDepthIndex int,
	) []float64
}

// Configure implements simulator.Iteration.
func (v *ValuesFunctionCumulativeMeanIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

// Iterate implements simulator.Iteration.
func (v *ValuesFunctionCumulativeMeanIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	latest := v.Function(params, partitionIndex, stateHistories, -1)
	previous := stateHistories[partitionIndex].Values.RawRowView(0)
	rows := float64(timestepsHistory.CurrentStepNumber + 1)
	mean := stateHistories[partitionIndex].GetNextStateRowToUpdate()
	for i, value := range latest {
		mean[i] = previous[i] + (value-previous[i])/rows
	}
	return mean
}
