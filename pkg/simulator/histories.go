package simulator

import (
	"gonum.org/v1/gonum/mat"
)

// StateHistory is a rolling window of state vectors.
//
// Usage hints:
//   - Values holds rows of state (row 0 is most recent by convention).
//   - Use GetNextStateRowToUpdate when updating in multi-row histories.
type StateHistory struct {
	// each row is a different state in the history, by convention,
	// starting with the most recent at index = 0
	Values *mat.Dense
	// NextValues is a per-partition reusable scratch buffer of length =
	// StateWidth, pre-allocated when the history is constructed. An Iteration
	// may write its next state into this buffer and return it to avoid
	// allocating a fresh row every step (GetNextStateRowToUpdate hands it back
	// pre-filled with the current state).
	NextValues        []float64
	StateWidth        int
	StateHistoryDepth int
}

// CopyStateRow copies a row from the state history given the index.
func (s *StateHistory) CopyStateRow(index int) []float64 {
	valuesCopy := make([]float64, s.StateWidth)
	copy(valuesCopy, s.Values.RawRowView(index))
	return valuesCopy
}

// GetNextStateRowToUpdate returns the partition's reusable NextValues buffer
// pre-filled with a copy of the current state (row 0), ready for the iteration
// to mutate and return.
//
// It always copies — never exposes a row of Values directly — so that mutating
// and returning the result cannot corrupt live history or any retained output.
func (s *StateHistory) GetNextStateRowToUpdate() []float64 {
	if s.NextValues == nil {
		s.NextValues = make([]float64, s.StateWidth)
	}
	copy(s.NextValues, s.Values.RawRowView(0))
	return s.NextValues
}

// CumulativeTimestepsHistory is a rolling window of cumulative timesteps with
// NextIncrement and CurrentStepNumber.
type CumulativeTimestepsHistory struct {
	NextIncrement     float64
	Values            *mat.VecDense
	CurrentStepNumber int
	StateHistoryDepth int
}

// IteratorInputMessage carries shared histories into iterator jobs.
type IteratorInputMessage struct {
	StateHistories   []*StateHistory
	TimestepsHistory *CumulativeTimestepsHistory
}
