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
	// should be of length = StateWidth
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

// GetNextStateRowToUpdate determines whether or not it is necessary
// to copy the previous row or simply expose it based on whether a history
// longer than 1 is needed.
func (s *StateHistory) GetNextStateRowToUpdate() []float64 {
	if s.StateHistoryDepth == 1 {
		return s.Values.RawRowView(0)
	}
	return s.CopyStateRow(0)
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
