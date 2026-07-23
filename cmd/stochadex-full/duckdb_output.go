//go:build duckdb_arrow

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/marcboeker/go-duckdb/v2"
	"github.com/umbralcalc/stochadex/pkg/arrowstore"
	"github.com/umbralcalc/stochadex/pkg/duckdbstore"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Registers `output_function: {type: duckdb, path: run.duckdb, table: results}`, landing
// the run in a DuckDB database for SQL analytics — zero-copy, via the same Arrow record the
// arrow sink writes. Compiled only under the duckdb_arrow tag (the driver is cgo), so the
// pure-Go build of this binary is unaffected.
func init() {
	simulator.RegisterComponent(
		"output_function",
		"duckdb",
		func(spec simulator.ComponentSpec) (interface{}, error) {
			path, err := stringField(spec, "path")
			if err != nil {
				return nil, err
			}
			table, err := stringField(spec, "table")
			if err != nil {
				return nil, err
			}
			store := arrowstore.NewArrowStateTimeStorage()
			return &duckdbOutput{
				store: store,
				inner: &arrowstore.ArrowStateTimeStorageOutputFunction{Store: store},
				path:  path,
				table: table,
			}, nil
		},
	)
}

// duckdbOutput accumulates the run in Arrow and ingests it into DuckDB in one shot at
// Finalize (IngestToTable is a single CREATE TABLE AS SELECT over the Arrow record).
type duckdbOutput struct {
	store *arrowstore.ArrowStateTimeStorage
	inner *arrowstore.ArrowStateTimeStorageOutputFunction
	path  string
	table string
}

func (d *duckdbOutput) Configure(settings *simulator.Settings) {
	d.inner.Configure(settings)
}

func (d *duckdbOutput) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
	d.inner.Output(partitionName, state, cumulativeTimesteps)
}

func (d *duckdbOutput) Finalize() {
	defer d.store.Release()

	db, err := sql.Open("duckdb", d.path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stochadex: opening DuckDB at %s: %v\n", d.path, err)
		return
	}
	defer db.Close()

	rows, err := duckdbstore.IngestToTable(context.Background(), db, d.store, d.table)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stochadex: ingesting into %s.%s: %v\n", d.path, d.table, err)
		return
	}
	fmt.Fprintf(os.Stderr, "stochadex: wrote %d rows to %s in %s\n", rows, d.table, d.path)
}
