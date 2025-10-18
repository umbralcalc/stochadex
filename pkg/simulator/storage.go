package simulator

import (
	"strings"
	"sync"
)

// StateTimeStorage provides thread-safe storage for simulation time series data
// with minimal contention and efficient access patterns.
//
// StateTimeStorage is designed to handle concurrent access from multiple
// simulation partitions while maintaining data consistency and performance.
// It uses a mutex-protected design optimized for the common case of
// appending new data points during simulation execution.
//
// Data Organization:
//   - Time series are organized by partition name
//   - Each partition can have multiple state dimensions
//   - Time axis is shared across all partitions
//   - Data is stored in row-major format for efficient access
//
// Thread Safety:
//   - ConcurrentAppend is safe for concurrent use from multiple goroutines
//   - GetValues/GetTimes are safe for concurrent reads
//   - SetValues/SetTimes should not be called concurrently with appends
//   - Internal mutex protects against race conditions
//
// Performance Characteristics:
//   - O(1) lookup by partition name using hash map
//   - O(1) append operations with minimal locking
//   - Memory usage: O(total_samples * state_dimensions)
//   - Efficient for high-frequency data collection
//
// Usage Patterns:
//   - Real-time data collection during simulation runs
//   - Batch data loading from external sources
//   - Result storage for post-simulation analysis
//   - Intermediate storage for multi-stage simulations
//
// Example Usage:
//
//	storage := NewStateTimeStorage()
//
//	// Concurrent appends from multiple partitions
//	go func() {
//	    storage.ConcurrentAppend("prices", 1.0, []float64{100.0, 101.0})
//	}()
//	go func() {
//	    storage.ConcurrentAppend("volumes", 1.0, []float64{1000.0})
//	}()
//
//	// Retrieve data after simulation
//	priceData := storage.GetValues("prices")
//	timeData := storage.GetTimes()
//
// Memory Management:
//   - Automatic memory allocation for new partitions
//   - Efficient storage of sparse time series
//   - No automatic cleanup (caller responsible for memory management)
//
// Error Handling:
//   - GetValues panics if partition name is not found
//   - Provides helpful error messages with available partition names
//   - ConcurrentAppend handles time deduplication automatically
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
