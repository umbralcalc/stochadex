package simulator

import (
	"slices"
	"sync"

	"gonum.org/v1/gonum/mat"
)

// StateTimeStorage dynamically adapts its structure to support incoming time series
// data from the simulation output in a thread-safe manner. This is done in a way to
// minimise write blocking for better concurrent performance.
type StateTimeStorage struct {
	indexByName map[string]int
	store       [][][]float64
	times       []float64
	mutex       *sync.Mutex
}

// GetNames retrieves all the names in the store to key each time series.
func (s *StateTimeStorage) GetNames() []string {
	names := make([]string, 0)
	for name := range s.indexByName {
		names = append(names, name)
	}
	return names
}

// GetValues retrieves all the time series values keyed by the name.
func (s *StateTimeStorage) GetValues(name string) [][]float64 {
	return s.store[s.indexByName[name]]
}

// GetTimes retrieves all the time values for any of the time series.
func (s *StateTimeStorage) GetTimes() []float64 {
	return s.times
}

// Append adds another set of values to the time series data keyed
// by the provided name. This method also handles dynamic extension
// of the size of the store in response to the inputs.
func (s *StateTimeStorage) Append(
	name string,
	time float64,
	values []float64,
) {
	var index int
	var exists bool

	// Double-check locking to safely update indexByName and store
	if _, exists = s.indexByName[name]; !exists {
		s.mutex.Lock()
		// Re-check to ensure the index has not been added by another partition
		if index, exists = s.indexByName[name]; !exists {
			index = len(s.indexByName)
			s.indexByName[name] = index
			s.store = append(s.store, [][]float64{})
		}
		s.mutex.Unlock()
	}

	// Append values without holding the lock
	s.store[index] = append(s.store[index], values)

	// Safely update times once per unique timestamp
	if len(s.times) == 0 || time > s.times[len(s.times)-1] {
		s.mutex.Lock()
		// Re-check to ensure another partition hasn’t updated the times
		if len(s.times) == 0 || time > s.times[len(s.times)-1] {
			s.times = append(s.times, time)
		}
		s.mutex.Unlock()
	}
}

// NewStateTimeStorage creates a new StateTimeStorage.
func NewStateTimeStorage() *StateTimeStorage {
	var mutex sync.Mutex
	return &StateTimeStorage{
		indexByName: make(map[string]int),
		store:       make([][][]float64, 0),
		times:       make([]float64, 0),
		mutex:       &mutex,
	}
}

// NewStateTimeHistoriesFromStateTimeStorage creates a new StateTimeHistories
// from all of the data held in the StateTimeStorage.
func NewStateTimeHistoriesFromStateTimeStorage(
	store StateTimeStorage,
) *StateTimeHistories {
	stateHistories := make(map[string]*StateHistory)
	for _, name := range store.GetNames() {
		stateHistoryValues := make([]float64, 0)
		var stateWidth int
		for _, stateValues := range store.GetValues(name) {
			stateWidth = len(stateValues)
			for i := 0; i < stateWidth; i++ {
				stateHistoryValues = append(
					stateHistoryValues,
					stateValues[stateWidth-i-1],
				)
			}
		}
		slices.Reverse(stateHistoryValues)
		stateHistoryDepth := int(len(stateHistoryValues) / stateWidth)
		stateHistories[name] = &StateHistory{
			Values: mat.NewDense(
				stateHistoryDepth,
				stateWidth,
				stateHistoryValues,
			),
			StateWidth:        stateWidth,
			StateHistoryDepth: stateHistoryDepth,
		}
	}
	times := store.GetTimes()
	slices.Reverse(times)
	stateHistoryDepth := len(times)
	return &StateTimeHistories{
		StateHistories: stateHistories,
		TimestepsHistory: &CumulativeTimestepsHistory{
			Values:            mat.NewVecDense(stateHistoryDepth, times),
			StateHistoryDepth: stateHistoryDepth,
		},
	}
}
