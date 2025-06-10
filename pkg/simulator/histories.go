package simulator

import (
	"gonum.org/v1/gonum/mat"
)

// StateHistory represents the information contained within a windowed
// history of []float64 state values.
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

// CumulativeTimestepsHistory is a windowed history of cumulative timestep values
// which includes the next value to increment time by and number of steps taken.
type CumulativeTimestepsHistory struct {
	NextIncrement     float64
	Values            *mat.VecDense
	CurrentStepNumber int
	StateHistoryDepth int
}

// IteratorInputMessage defines the message which is passed from the
// PartitionCoordinator to a StateIterator of a given partition when
// the former is requesting the latter to perform a job.
type IteratorInputMessage struct {
	StateHistories   []*StateHistory
	TimestepsHistory *CumulativeTimestepsHistory
}
