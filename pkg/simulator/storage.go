package simulator

import (
	"math"
	"strings"
	"sync"
	"sync/atomic"
)

// StateTimeStorage stores simulation time series data organised by partition
// name.
//
// Two append paths serve different use cases:
//   - AppendByIndex — the simulation hot path. Lock-free. Requires all names
//     pre-registered via PreRegisterPartitions (done automatically by
//     NewPartitionCoordinator), one goroutine per partition index, and no
//     concurrent reads during output.
//   - Append — for single-goroutine data loading (CSV, JSON log, database).
//     Not safe for concurrent use.
//
// GetValues, GetTimes, GetNames, SetValues, SetTimes and the registration
// methods (PreRegisterPartitions, GetIndex, IndexOf) are all intended for
// single-goroutine setup or post-simulation use.
//
// The only internal synchronisation that remains is a mutex guarding the
// shared times slice, since N partition goroutines may all call appendTimeIfNew
// with the same timestamp; an atomic fast-path skips the mutex in the common
// case where the timestamp is already recorded.
type StateTimeStorage struct {
	indexByName  map[string]int
	store        [][][]float64
	times        []float64
	timesMu      sync.Mutex
	lastTimeBits uint64 // atomic; math.Float64bits of last appended time
}

func (s *StateTimeStorage) getOrCreateIndex(name string) int {
	if index, ok := s.indexByName[name]; ok {
		return index
	}
	index := len(s.indexByName)
	s.indexByName[name] = index
	s.store = append(s.store, [][]float64{})
	return index
}

func (s *StateTimeStorage) appendTimeIfNew(time float64) {
	bits := math.Float64bits(time)
	if atomic.LoadUint64(&s.lastTimeBits) == bits {
		return
	}
	s.timesMu.Lock()
	if len(s.times) == 0 || time > s.times[len(s.times)-1] {
		s.times = append(s.times, time)
		atomic.StoreUint64(&s.lastTimeBits, bits)
	}
	s.timesMu.Unlock()
}

// PreRegisterPartitions ensures each name has a stable index and an empty row
// buffer before AppendByIndex is called concurrently. Idempotent.
func (s *StateTimeStorage) PreRegisterPartitions(names []string) {
	for _, name := range names {
		s.getOrCreateIndex(name)
	}
}

// GetIndex returns or creates the index for name.
func (s *StateTimeStorage) GetIndex(name string) int {
	return s.getOrCreateIndex(name)
}

// IndexOf returns the index and true if name is registered, or 0 and false.
func (s *StateTimeStorage) IndexOf(name string) (int, bool) {
	index, ok := s.indexByName[name]
	return index, ok
}

// AppendByIndex appends values for a pre-registered partition index and
// records time at most once per unique timestamp.
//
// Lock-free for the store. Preconditions (all hold under normal coordinator use):
//   - All names pre-registered via PreRegisterPartitions
//   - One goroutine per partition index
//   - No concurrent GetValues calls
func (s *StateTimeStorage) AppendByIndex(index int, time float64, values []float64) {
	s.store[index] = append(s.store[index], values)
	s.appendTimeIfNew(time)
}

// Append appends values for name and records time. Not safe for concurrent
// use; intended for single-goroutine data loading.
func (s *StateTimeStorage) Append(name string, time float64, values []float64) {
	index := s.getOrCreateIndex(name)
	s.store[index] = append(s.store[index], values)
	if len(s.times) == 0 || time > s.times[len(s.times)-1] {
		s.times = append(s.times, time)
	}
}

// GetNames returns all registered partition names.
func (s *StateTimeStorage) GetNames() []string {
	names := make([]string, 0, len(s.indexByName))
	for name := range s.indexByName {
		names = append(names, name)
	}
	return names
}

// GetValues returns a snapshot of all time series rows for name, panicking if absent.
func (s *StateTimeStorage) GetValues(name string) [][]float64 {
	index, ok := s.indexByName[name]
	if !ok {
		names := make([]string, 0, len(s.indexByName))
		for n := range s.indexByName {
			names = append(names, n)
		}
		panic("name: " + name +
			" not found in storage, choices are: " +
			strings.Join(names, ", "))
	}
	src := s.store[index]
	out := make([][]float64, len(src))
	for i, row := range src {
		cp := make([]float64, len(row))
		copy(cp, row)
		out[i] = cp
	}
	return out
}

// SetValues replaces the entire series for name.
func (s *StateTimeStorage) SetValues(name string, values [][]float64) {
	index := s.getOrCreateIndex(name)
	s.store[index] = values
}

// GetTimes returns a snapshot of the time axis.
func (s *StateTimeStorage) GetTimes() []float64 {
	s.timesMu.Lock()
	defer s.timesMu.Unlock()
	out := make([]float64, len(s.times))
	copy(out, s.times)
	return out
}

// SetTimes replaces the time axis.
func (s *StateTimeStorage) SetTimes(times []float64) {
	s.timesMu.Lock()
	defer s.timesMu.Unlock()
	s.times = times
}

// NewStateTimeStorage constructs a new StateTimeStorage.
func NewStateTimeStorage() *StateTimeStorage {
	return &StateTimeStorage{
		indexByName:  make(map[string]int),
		store:        make([][][]float64, 0),
		times:        make([]float64, 0),
		lastTimeBits: ^uint64(0), // sentinel: no real time recorded yet
	}
}
