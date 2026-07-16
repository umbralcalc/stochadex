//go:build duckdb_arrow

package duckdbstore

import (
	"context"
	"database/sql"
	"math"
	"testing"

	_ "github.com/marcboeker/go-duckdb/v2"

	"github.com/umbralcalc/stochadex/pkg/arrowstore"
)

// buildStorage produces a small aligned storage: 2 partitions (widths 3 and 2), 3 timesteps.
func buildStorage() *arrowstore.ArrowStateTimeStorage {
	s := arrowstore.NewArrowStateTimeStorage()
	s.PreRegisterPartitions([]string{"alpha", "beta"})
	rows := []struct {
		idx  int
		t    float64
		vals []float64
	}{
		{0, 0.0, []float64{1, 2, 3}}, {1, 0.0, []float64{10, 20}},
		{0, 1.0, []float64{4, 5, 6}}, {1, 1.0, []float64{30, 40}},
		{0, 2.0, []float64{7, 8, 9}}, {1, 2.0, []float64{50, 60}},
	}
	for _, r := range rows {
		s.AppendByIndex(r.idx, r.t, r.vals)
	}
	return s
}

func TestIngestToTable(t *testing.T) {
	s := buildStorage()
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open duckdb: %v", err)
	}
	defer db.Close()

	n, err := IngestToTable(context.Background(), db, s, "sim")
	if err != nil {
		t.Fatalf("IngestToTable: %v", err)
	}
	if n != 3 {
		t.Fatalf("ingested rows = %d, want 3", n)
	}

	// The table is now queryable with ordinary database/sql. Verify the time column, the
	// per-partition ARRAY columns (1-based indexing in DuckDB), and an aggregation.
	var cnt int
	var avgT, sumA0, maxB1 float64
	err = db.QueryRow(`
		SELECT count(*), avg(time), sum(alpha[1]), max(beta[2])
		FROM sim`).Scan(&cnt, &avgT, &sumA0, &maxB1)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if cnt != 3 {
		t.Fatalf("count = %d, want 3", cnt)
	}
	if math.Abs(avgT-1.0) > 1e-9 { // (0+1+2)/3
		t.Fatalf("avg(time) = %v, want 1.0", avgT)
	}
	if math.Abs(sumA0-12.0) > 1e-9 { // 1+4+7
		t.Fatalf("sum(alpha[1]) = %v, want 12", sumA0)
	}
	if math.Abs(maxB1-60.0) > 1e-9 { // max(20,40,60)
		t.Fatalf("max(beta[2]) = %v, want 60", maxB1)
	}
}

func TestIngestReplaceIsIdempotent(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open duckdb: %v", err)
	}
	defer db.Close()
	for i := 0; i < 2; i++ { // CREATE OR REPLACE: ingesting twice must not error or duplicate
		if _, err := IngestToTable(context.Background(), db, buildStorage(), "sim"); err != nil {
			t.Fatalf("ingest %d: %v", i, err)
		}
	}
	var cnt int
	if err := db.QueryRow(`SELECT count(*) FROM sim`).Scan(&cnt); err != nil {
		t.Fatalf("query: %v", err)
	}
	if cnt != 3 {
		t.Fatalf("after re-ingest count = %d, want 3", cnt)
	}
}

func TestIngestUnalignedErrors(t *testing.T) {
	// beta emits one fewer row than alpha → not row-aligned → Record() is nil → clear error.
	s := arrowstore.NewArrowStateTimeStorage()
	s.PreRegisterPartitions([]string{"alpha", "beta"})
	s.AppendByIndex(0, 0.0, []float64{1})
	s.AppendByIndex(1, 0.0, []float64{2})
	s.AppendByIndex(0, 1.0, []float64{3}) // alpha has 2 rows, beta has 1

	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open duckdb: %v", err)
	}
	defer db.Close()
	if _, err := IngestToTable(context.Background(), db, s, "sim"); err == nil {
		t.Fatal("expected an error ingesting unaligned storage, got nil")
	}
}
