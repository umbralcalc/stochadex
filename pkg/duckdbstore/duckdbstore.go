//go:build duckdb_arrow

package duckdbstore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	duckdb "github.com/marcboeker/go-duckdb/v2"

	"github.com/umbralcalc/stochadex/pkg/arrowstore"
)

// ingestViewName is the transient DuckDB view the Arrow record is registered under while it
// is copied into the target table. It exists only for the duration of one IngestToTable call.
const ingestViewName = "__stochadex_ingest_view"

// IngestToTable materialises the simulation output held in s into a DuckDB table named table
// (created or replaced) on db, zero-copy via the driver's Arrow interface: s's finished Arrow
// record is registered as a view and copied into the table with a single CREATE TABLE AS
// SELECT — no [][]float64 round-trip. The table has a time column plus one fixed-size
// ARRAY<DOUBLE> column per partition (named by the partition). It requires s to be row-aligned
// (every partition produced the same number of rows as the time axis); otherwise it returns an
// error. Returns the number of rows ingested.
func IngestToTable(
	ctx context.Context,
	db *sql.DB,
	s *arrowstore.ArrowStateTimeStorage,
	table string,
) (int64, error) {
	rec := s.Record()
	if rec == nil {
		return 0, fmt.Errorf(
			"duckdbstore: storage is not row-aligned (partitions produced differing row " +
				"counts); cannot ingest as a single table")
	}
	defer rec.Release()

	reader, err := array.NewRecordReader(rec.Schema(), []arrow.Record{rec})
	if err != nil {
		return 0, fmt.Errorf("duckdbstore: build record reader: %w", err)
	}
	defer reader.Release()

	conn, err := db.Conn(ctx)
	if err != nil {
		return 0, fmt.Errorf("duckdbstore: acquire connection: %w", err)
	}
	defer conn.Close()

	var rows int64
	err = conn.Raw(func(driverConn any) error {
		ar, err := duckdb.NewArrowFromConn(driverConn.(driver.Conn))
		if err != nil {
			return fmt.Errorf("duckdbstore: open Arrow interface: %w", err)
		}
		release, err := ar.RegisterView(reader, ingestViewName)
		if err != nil {
			return fmt.Errorf("duckdbstore: register Arrow view: %w", err)
		}
		defer release()

		if err := execArrow(ctx, ar, fmt.Sprintf(
			"CREATE OR REPLACE TABLE %s AS SELECT * FROM %s",
			quoteIdent(table), ingestViewName)); err != nil {
			return fmt.Errorf("duckdbstore: materialise table: %w", err)
		}
		rows, err = queryScalarInt64(ctx, ar, fmt.Sprintf(
			"SELECT count(*) FROM %s", quoteIdent(table)))
		if err != nil {
			return fmt.Errorf("duckdbstore: count rows: %w", err)
		}
		return nil
	})
	return rows, err
}

// execArrow runs a statement that produces no meaningful result set (e.g. DDL), driving the
// reader to completion so the statement actually executes, then releasing it.
func execArrow(ctx context.Context, ar *duckdb.Arrow, query string) error {
	reader, err := ar.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer reader.Release()
	for reader.Next() {
	}
	return reader.Err()
}

// queryScalarInt64 runs a query expected to return a single int64 in the first column of the
// first row (e.g. a count).
func queryScalarInt64(ctx context.Context, ar *duckdb.Arrow, query string) (int64, error) {
	reader, err := ar.QueryContext(ctx, query)
	if err != nil {
		return 0, err
	}
	defer reader.Release()
	var out int64
	for reader.Next() {
		rec := reader.Record()
		if rec.NumRows() == 0 {
			continue
		}
		col, ok := rec.Column(0).(*array.Int64)
		if !ok {
			return 0, fmt.Errorf("duckdbstore: scalar query first column is %s, want int64",
				rec.Column(0).DataType())
		}
		out = col.Value(0)
	}
	return out, reader.Err()
}

// quoteIdent quotes a SQL identifier for DuckDB — double quotes, with any embedded double
// quote doubled — so partition/table names cannot break out into arbitrary SQL.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
