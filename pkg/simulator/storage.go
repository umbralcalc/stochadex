package simulator

import (
	"strings"
	"sync"
)

// StateTimeStorage stores per-partition time series with minimal contention.
//
// Usage hints:
//   - Use ConcurrentAppend to add samples safely; times are deduplicated.
//   - GetValues/GetTimes retrieve stored series; Set* replace series entirely.
type StateTimeStorage struct {
	indexByName map[string]int
	store       [][][]float64
	times       []float64
	mutex       *sync.Mutex
}

// GetIndex returns or creates the index for a name.
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

// GetNames returns all partition names present in the store.
func (s *StateTimeStorage) GetNames() []string {
	names := make([]string, 0)
	for name := range s.indexByName {
		names = append(names, name)
	}
	return names
}

// GetValues returns all time series for name, panicking if absent.
func (s *StateTimeStorage) GetValues(name string) [][]float64 {
	index, ok := s.indexByName[name]
	if !ok {
		panic("name: " + name +
			" not found in storage, choices are: " +
			strings.Join(s.GetNames(), ", "))
	}
	return s.store[index]
}

// SetValues replaces the entire series for name.
func (s *StateTimeStorage) SetValues(name string, values [][]float64) {
	s.store[s.GetIndex(name)] = values
}

// GetTimes returns the time axis.
func (s *StateTimeStorage) GetTimes() []float64 {
	return s.times
}

// SetTimes replaces the time axis.
func (s *StateTimeStorage) SetTimes(times []float64) {
	s.times = times
}

// ConcurrentAppend appends values for name and updates the time axis at most
// once per unique timestamp. Safe for concurrent use.
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

// NewStateTimeStorage constructs a new StateTimeStorage.
func NewStateTimeStorage() *StateTimeStorage {
	var mutex sync.Mutex
	return &StateTimeStorage{
		indexByName: make(map[string]int),
		store:       make([][][]float64, 0),
		times:       make([]float64, 0),
		mutex:       &mutex,
	}
}
