package arrowstore

import (
	"fmt"
	"sync"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// TestAppendMaterialize checks that append → Finalize round-trips the values and the
// deduplicated time axis, and that Record aligns when every partition outputs every step.
func TestAppendMaterialize(t *testing.T) {
	s := NewArrowStateTimeStorage()
	s.PreRegisterPartitions([]string{"a", "b"})
	// two partitions, width 3 and 2, three shared timesteps
	rows := []struct {
		idx  int
		t    float64
		vals []float64
	}{
		{0, 1.0, []float64{1, 2, 3}}, {1, 1.0, []float64{10, 20}},
		{0, 2.0, []float64{4, 5, 6}}, {1, 2.0, []float64{30, 40}},
		{0, 3.0, []float64{7, 8, 9}}, {1, 3.0, []float64{50, 60}},
	}
	for _, r := range rows {
		s.AppendByIndex(r.idx, r.t, r.vals)
	}

	if got := s.GetTimes(); fmt.Sprint(got) != "[1 2 3]" {
		t.Fatalf("GetTimes = %v, want [1 2 3]", got)
	}
	a := s.GetValues("a")
	if fmt.Sprint(a) != "[[1 2 3] [4 5 6] [7 8 9]]" {
		t.Fatalf("GetValues(a) = %v", a)
	}
	b := s.GetValues("b")
	if fmt.Sprint(b) != "[[10 20] [30 40] [50 60]]" {
		t.Fatalf("GetValues(b) = %v", b)
	}
	rec := s.Record()
	if rec == nil {
		t.Fatal("Record() = nil, want aligned record")
	}
	defer rec.Release()
	if rec.NumRows() != 3 || rec.NumCols() != 3 { // time + a + b
		t.Fatalf("Record shape = %dx%d, want 3x3", rec.NumRows(), rec.NumCols())
	}
	s.Release()
}

// TestTimeDedup verifies the shared time column records each unique timestamp once, even
// when many partitions report the same step (the common case).
func TestTimeDedup(t *testing.T) {
	s := NewArrowStateTimeStorage()
	s.PreRegisterPartitions([]string{"a", "b", "c"})
	for _, tm := range []float64{1, 1, 1, 2, 2, 2, 3} { // 3 partitions per step, one straggler
		s.AppendByIndex(0, tm, []float64{tm})
	}
	if got := s.GetTimes(); fmt.Sprint(got) != "[1 2 3]" {
		t.Fatalf("dedup GetTimes = %v, want [1 2 3]", got)
	}
	s.Release()
}

// TestConcurrentPartitions mirrors the coordinator: one goroutine per partition index,
// all appending the same increasing timesteps. Run with -race to check the lock-free
// per-partition path and the atomic-guarded shared time column.
func TestConcurrentPartitions(t *testing.T) {
	const parts, steps, width = 8, 500, 4
	s := NewArrowStateTimeStorage()
	names := make([]string, parts)
	for i := range names {
		names[i] = fmt.Sprintf("p%d", i)
	}
	s.PreRegisterPartitions(names)

	// Mirror the coordinator's per-step barrier: all partitions emit step t (concurrently,
	// exercising the shared-time dedup race) before any emits step t+1.
	for step := 0; step < steps; step++ {
		var wg sync.WaitGroup
		for p := 0; p < parts; p++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				row := make([]float64, width)
				for k := range row {
					row[k] = float64(idx*1000 + step)
				}
				s.AppendByIndex(idx, float64(step), row)
			}(p)
		}
		wg.Wait()
	}

	if got := len(s.GetTimes()); got != steps {
		t.Fatalf("GetTimes len = %d, want %d", got, steps)
	}
	for p := 0; p < parts; p++ {
		vals := s.GetValues(names[p])
		if len(vals) != steps {
			t.Fatalf("partition %d rows = %d, want %d", p, len(vals), steps)
		}
		if vals[steps-1][0] != float64(p*1000+steps-1) {
			t.Fatalf("partition %d last row = %v", p, vals[steps-1])
		}
	}
	s.Release()
}

var benchGrid = []struct{ w, rows int }{
	{4, 10}, {4, 100}, {4, 2000}, {64, 100}, {64, 2000}, {256, 2000},
}

func benchRow(w int) []float64 {
	v := make([]float64, w)
	for i := range v {
		v[i] = float64(i) + 0.5
	}
	return v
}

// BenchmarkAppend isolates the append HOT PATH — current jagged AppendByIndex (per-row
// heap alloc) vs the Arrow builder append (no materialization). This is the apples-to-apples
// the plan asks for. Run: go test -tags arrow -run=x -bench=Append -benchmem
func BenchmarkAppend(b *testing.B) {
	for _, g := range benchGrid {
		v := benchRow(g.w)
		b.Run(fmt.Sprintf("current/w%d_rows%d", g.w, g.rows), func(b *testing.B) {
			b.ReportAllocs()
			for n := 0; n < b.N; n++ {
				st := simulator.NewStateTimeStorage()
				st.PreRegisterPartitions([]string{"p"})
				for r := 0; r < g.rows; r++ {
					st.AppendByIndex(0, float64(r), v)
				}
			}
		})
		b.Run(fmt.Sprintf("arrow/w%d_rows%d", g.w, g.rows), func(b *testing.B) {
			b.ReportAllocs()
			for n := 0; n < b.N; n++ {
				st := NewArrowStateTimeStorage()
				st.PreRegisterPartitions([]string{"p"})
				for r := 0; r < g.rows; r++ {
					st.AppendByIndex(0, float64(r), v)
				}
			}
		})
	}
}

// BenchmarkToArrow is the INTERCHANGE comparison — the whole point of the storage: getting
// output into Arrow. Current path = jagged append then convert [][]float64 → Arrow; Arrow
// path = builder append then Finalize (materialize). This is where Arrow should win, because
// it skips the [][]float64 → Arrow conversion.
func BenchmarkToArrow(b *testing.B) {
	for _, g := range benchGrid {
		v := benchRow(g.w)
		b.Run(fmt.Sprintf("current+convert/w%d_rows%d", g.w, g.rows), func(b *testing.B) {
			pool := memory.NewGoAllocator()
			b.ReportAllocs()
			for n := 0; n < b.N; n++ {
				st := simulator.NewStateTimeStorage()
				st.PreRegisterPartitions([]string{"p"})
				for r := 0; r < g.rows; r++ {
					st.AppendByIndex(0, float64(r), v)
				}
				// convert the jagged rows into an Arrow FixedSizeList (interchange).
				fslb := array.NewFixedSizeListBuilder(pool, int32(g.w), arrow.PrimitiveTypes.Float64)
				vb := fslb.ValueBuilder().(*array.Float64Builder)
				for _, row := range st.GetValues("p") {
					fslb.Append(true)
					vb.AppendValues(row, nil)
				}
				fslb.NewArray().Release()
				fslb.Release()
			}
		})
		b.Run(fmt.Sprintf("arrow/w%d_rows%d", g.w, g.rows), func(b *testing.B) {
			b.ReportAllocs()
			for n := 0; n < b.N; n++ {
				st := NewArrowStateTimeStorage()
				st.PreRegisterPartitions([]string{"p"})
				for r := 0; r < g.rows; r++ {
					st.AppendByIndex(0, float64(r), v)
				}
				st.Finalize()
				st.Release()
			}
		})
	}
}
