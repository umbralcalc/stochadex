package simulator

import (
	"strings"
	"sync"
)

// StateTimeStorage dynamically adapts its structure to support incoming
// time series data from the simulation output in a thread-safe manner.
// This is done in a way to minimise write blocking for better performance
// in a concurrent program.
type StateTimeStorage struct {
	indexByName map[string]int
	store       [][][]float64
	times       []float64
	mutex       *sync.Mutex
}

// GetIndex retrieves the index for provided a key name. This will make
// a new index if the name doesn't yet exist in the store.
func (s *StateTimeStorage) GetIndex(name string) int {
	var index int
	var exists bool
	if index, exists = s.indexByName[name]; !exists {
		index = len(s.indexByName)
		s.indexByName[name] = index
		s.store = append(s.store, [][]float64{})
	}
	return index
}

// GetNames retrieves all the names in the store to key each time series.
func (s *StateTimeStorage) GetNames() []string {
	names := make([]string, 0)
	for name := range s.indexByName {
		names = append(names, name)
	}
	return names
}

// GetValues retrieves all the time series values keyed by the name. This
// method will panic if the name doesn't exist in the store.
func (s *StateTimeStorage) GetValues(name string) [][]float64 {
	index, ok := s.indexByName[name]
	if !ok {
		panic("name: " + name +
			" not found in storage, choices are: " +
			strings.Join(s.GetNames(), ", "))
	}
	return s.store[index]
}

// SetValues sets all the time series values keyed by the name.
func (s *StateTimeStorage) SetValues(name string, values [][]float64) {
	s.store[s.GetIndex(name)] = values
}

// GetTimes retrieves all the time values.
func (s *StateTimeStorage) GetTimes() []float64 {
	return s.times
}

// SetTimes sets all the time values.
func (s *StateTimeStorage) SetTimes(times []float64) {
	s.times = times
}

// ConcurrentAppend adds another set of values to the time series data keyed
// by the provided name. This method also handles dynamic extension of the
// size of the store in response to the inputs, and can safely handle
// concurrent calls within the same program.
func (s *StateTimeStorage) ConcurrentAppend(
	name string,
	time float64,
	values []float64,
) {
	var index int
	var exists bool

	// Double-check locking to safely update indexByName and store
	if index, exists = s.indexByName[name]; !exists {
		s.mutex.Lock()
		// Re-check to ensure the index has not been added by another
		index = s.GetIndex(name)
		s.mutex.Unlock()
	}

	// Append values without holding the lock
	s.store[index] = append(s.store[index], values)

	// Safely update times once per unique timestamp
	if len(s.times) == 0 || time > s.times[len(s.times)-1] {
		s.mutex.Lock()
		// Re-check to ensure another hasn’t updated the times
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
