package arrowstore

import (
	"math"
	"sync"
	"sync/atomic"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ArrowStateTimeStorage is the Arrow-native, opt-in counterpart to
// simulator.StateTimeStorage. See the package doc for the rationale and the concurrency
// contract, both of which mirror simulator.StateTimeStorage.
type ArrowStateTimeStorage struct {
	pool        memory.Allocator
	indexByName map[string]int
	names       []string
	builders    []*array.FixedSizeListBuilder // one per partition index; created lazily on first row
	values      []*array.Float64Builder       // cached value builder per partition index
	widths      []int                         // fixed state width per partition (learned on first row)

	timeBuilder  *array.Float64Builder // shared time column
	timesMu      sync.Mutex
	lastTimeBits uint64 // atomic; math.Float64bits of the last appended time

	// Materialised arrays (populated once by Finalize; nil until then).
	mat     bool
	timeArr *array.Float64
	partArr []arrow.Array
}

// noTimeYet is the initial lastTimeBits sentinel. It must not equal Float64bits of any real
// timestamp — in particular NOT 0 (which is Float64bits(0.0)), or a first timestamp of exactly
// 0.0 would collide with the sentinel and be dropped from the time axis. All-ones is a NaN bit
// pattern, which no finite timestamp produces.
const noTimeYet = ^uint64(0)

// NewArrowStateTimeStorage returns an empty storage backed by the default Go allocator.
func NewArrowStateTimeStorage() *ArrowStateTimeStorage {
	pool := memory.NewGoAllocator()
	return &ArrowStateTimeStorage{
		pool:         pool,
		indexByName:  map[string]int{},
		timeBuilder:  array.NewFloat64Builder(pool),
		lastTimeBits: noTimeYet,
	}
}

// PreRegisterPartitions assigns a stable index and a (lazily-filled) builder slot to each
// name. Widths are learned from the first row each partition appends, so the fixed-width
// builder is created by the owning goroutine on its first AppendByIndex — safe because each
// index is written by exactly one goroutine.
func (s *ArrowStateTimeStorage) PreRegisterPartitions(names []string) {
	for _, name := range names {
		if _, ok := s.indexByName[name]; ok {
			continue
		}
		s.indexByName[name] = len(s.names)
		s.names = append(s.names, name)
		s.builders = append(s.builders, nil)
		s.values = append(s.values, nil)
		s.widths = append(s.widths, 0)
	}
}

// IndexOf returns the index and true if name is registered, or 0 and false.
func (s *ArrowStateTimeStorage) IndexOf(name string) (int, bool) {
	i, ok := s.indexByName[name]
	return i, ok
}

// GetNames returns the registered partition names in index order.
func (s *ArrowStateTimeStorage) GetNames() []string { return s.names }

// AppendByIndex appends one row for a pre-registered partition and records the time at most
// once per unique timestamp. Lock-free for the partition builder (one goroutine per index);
// the shared time column uses the same atomic-guarded dedup as simulator.StateTimeStorage.
func (s *ArrowStateTimeStorage) AppendByIndex(index int, time float64, values []float64) {
	b := s.builders[index]
	if b == nil {
		// First row for this partition: create the fixed-width builder now that the width
		// is known. Only the owning goroutine runs this for index, so no lock is needed.
		w := len(values)
		b = array.NewFixedSizeListBuilder(s.pool, int32(w), arrow.PrimitiveTypes.Float64)
		s.builders[index] = b
		s.values[index] = b.ValueBuilder().(*array.Float64Builder)
		s.widths[index] = w
	}
	b.Append(true)
	s.values[index].AppendValues(values, nil)
	s.appendTimeIfNew(time)
}

func (s *ArrowStateTimeStorage) appendTimeIfNew(time float64) {
	bits := math.Float64bits(time)
	if atomic.LoadUint64(&s.lastTimeBits) == bits {
		return
	}
	s.timesMu.Lock()
	// Re-check under the lock against the last appended time (only mutated here).
	if s.timeBuilder.Len() == 0 ||
		time > math.Float64frombits(atomic.LoadUint64(&s.lastTimeBits)) {
		s.timeBuilder.Append(time)
		atomic.StoreUint64(&s.lastTimeBits, bits)
	}
	s.timesMu.Unlock()
}

// Finalize materialises every builder into an immutable Arrow array. It is single-threaded,
// idempotent, and must be called after the simulation run (never concurrently with
// AppendByIndex). It consumes the builders. Call Release when done with the storage.
func (s *ArrowStateTimeStorage) Finalize() {
	if s.mat {
		return
	}
	s.timeArr = s.timeBuilder.NewFloat64Array()
	s.partArr = make([]arrow.Array, len(s.builders))
	for i, b := range s.builders {
		if b != nil {
			s.partArr[i] = b.NewArray()
		}
	}
	s.mat = true
}

// Release frees the materialised Arrow arrays. Safe to call once after Finalize.
func (s *ArrowStateTimeStorage) Release() {
	if s.timeArr != nil {
		s.timeArr.Release()
		s.timeArr = nil
	}
	for i, a := range s.partArr {
		if a != nil {
			a.Release()
			s.partArr[i] = nil
		}
	}
}

// TimeArray returns the shared time column as an Arrow array (Finalize is called if needed).
// The storage retains ownership; do not Release the returned array directly.
func (s *ArrowStateTimeStorage) TimeArray() *array.Float64 {
	s.Finalize()
	return s.timeArr
}

// ArrayForIndex returns partition index's rows as a FixedSizeList<float64>[width] Arrow
// array, or nil if the partition never produced a row. The storage retains ownership.
func (s *ArrowStateTimeStorage) ArrayForIndex(index int) *array.FixedSizeList {
	s.Finalize()
	if index < 0 || index >= len(s.partArr) || s.partArr[index] == nil {
		return nil
	}
	return s.partArr[index].(*array.FixedSizeList)
}

// GetTimes returns the deduplicated time axis as a plain slice, for compatibility with
// analysis code written against simulator.StateTimeStorage.
func (s *ArrowStateTimeStorage) GetTimes() []float64 {
	s.Finalize()
	if s.timeArr == nil {
		return nil
	}
	return append([]float64(nil), s.timeArr.Float64Values()...)
}

// GetValues returns partition name's rows as [][]float64, for compatibility with analysis
// code written against simulator.StateTimeStorage. Each row shares the underlying Arrow
// buffer (read-only); copy if you need to mutate.
func (s *ArrowStateTimeStorage) GetValues(name string) [][]float64 {
	idx, ok := s.indexByName[name]
	if !ok {
		return nil
	}
	arr := s.ArrayForIndex(idx)
	if arr == nil {
		return nil
	}
	w := s.widths[idx]
	flat := arr.ListValues().(*array.Float64).Float64Values()
	n := arr.Len()
	out := make([][]float64, n)
	for i := 0; i < n; i++ {
		out[i] = flat[i*w : (i+1)*w : (i+1)*w]
	}
	return out
}

// Record assembles a single Arrow record: the shared time column followed by one
// FixedSizeList column per partition. It is only well-defined when every partition produced
// the same number of rows as the time axis (the usual case: every partition outputs every
// step). It returns nil if that alignment does not hold. The caller owns the record (Release).
func (s *ArrowStateTimeStorage) Record() arrow.Record {
	s.Finalize()
	nTime := s.timeArr.Len()
	fields := []arrow.Field{{Name: "time", Type: arrow.PrimitiveTypes.Float64}}
	cols := []arrow.Array{s.timeArr}
	for i, a := range s.partArr {
		if a == nil || a.Len() != nTime {
			return nil
		}
		fields = append(fields, arrow.Field{Name: s.names[i], Type: a.DataType()})
		cols = append(cols, a)
	}
	return array.NewRecord(arrow.NewSchema(fields, nil), cols, int64(nTime))
}

// ArrowStateTimeStorageOutputFunction is the opt-in, Arrow-native counterpart to
// simulator.StateTimeStorageOutputFunction. It implements simulator.OutputFunction and writes
// each partition's state into an ArrowStateTimeStorage at the egress boundary.
type ArrowStateTimeStorageOutputFunction struct {
	Store       *ArrowStateTimeStorage
	nameToIndex map[string]int // populated by Configure; read-only during Output
}

// Configure pre-registers all partition names on Store and caches their indices for
// lock-free lookup in Output. Safe to call multiple times.
func (f *ArrowStateTimeStorageOutputFunction) Configure(settings *simulator.Settings) {
	if f == nil || f.Store == nil || settings == nil {
		return
	}
	names := make([]string, 0, len(settings.Iterations))
	for _, it := range settings.Iterations {
		names = append(names, it.Name)
	}
	f.Store.PreRegisterPartitions(names)
	nameToIndex := make(map[string]int, len(names))
	for _, name := range names {
		if index, ok := f.Store.IndexOf(name); ok {
			nameToIndex[name] = index
		}
	}
	f.nameToIndex = nameToIndex
}

// Output stores one partition's state row.
func (f *ArrowStateTimeStorageOutputFunction) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
	f.Store.AppendByIndex(f.nameToIndex[partitionName], cumulativeTimesteps, state)
}
