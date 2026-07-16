// Package duckdbstore is an opt-in, DuckDB analytical egress for simulation output. It takes
// the Arrow output produced by pkg/arrowstore and lands it in DuckDB with no intermediate
// conversion, so a finished simulation is immediately queryable with SQL (aggregations,
// windows, joins, Parquet export, …).
//
// It is a SEPARATE Go module (github.com/umbralcalc/stochadex/pkg/duckdbstore) and — unlike
// the pure-Go engine and the pure-Go arrowstore — it is **CGO and not WASM-compatible**: the
// DuckDB Go driver statically links the DuckDB C++ library (a large native dependency). So it
// is categorically an edge/server capability, never core and never on the default path. This
// isolation keeps DuckDB and cgo entirely out of both the engine's and arrowstore's go.mod.
//
// Two build requirements, both intentional:
//   - CGO must be enabled (the default), with a C toolchain available;
//   - the driver's Arrow interface lives behind ITS OWN duckdb_arrow build tag, so this
//     package's implementation carries a //go:build duckdb_arrow constraint too. Build/test with:
//
//	CGO_ENABLED=1 go test -tags duckdb_arrow ./...
//
// Without the tag only this doc compiles — nothing pulls in DuckDB or cgo.
//
// How it works:
// arrowstore.ArrowStateTimeStorage finishes into a single Arrow Record (a time column plus
// one fixed-size ARRAY<DOUBLE> column per partition). IngestToTable wraps that record in an
// arrow.RecordReader and hands it to the driver's zero-copy RegisterView, then materialises it
// into a DuckDB table with one CREATE TABLE AS SELECT. The Arrow types are shared (both this
// package and the driver use github.com/apache/arrow-go/v18), so the record crosses into
// DuckDB without a copy or a [][]float64 round-trip — the interchange win arrowstore was built
// to enable.
package duckdbstore
