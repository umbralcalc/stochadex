# duckdbstore — opt-in DuckDB analytical egress

Land simulation output in **DuckDB** for SQL analytics (aggregations, windows, joins, Parquet
export), fed **zero-copy** from [`arrowstore`](../arrowstore)'s Arrow output — no
`[][]float64` round-trip.

It is a **separate Go module**, and unlike the pure-Go engine and arrowstore it is **CGO and
not WASM-compatible** (the DuckDB Go driver statically links the DuckDB C++ library). So it is
categorically an edge/server capability — DuckDB and cgo stay entirely out of the engine's and
arrowstore's `go.mod`; you opt in only by importing this module.

```bash
go get github.com/umbralcalc/stochadex/pkg/duckdbstore
```

## Build requirements

- **CGO enabled** (the default) with a C toolchain available.
- The driver's Arrow interface is behind its own **`duckdb_arrow` build tag**, so this package
  is too. Build and test with:

```bash
CGO_ENABLED=1 go test -tags duckdb_arrow ./...
```

Without the tag, only the package doc compiles — nothing pulls in DuckDB or cgo.

## Usage

```go
store := arrowstore.NewArrowStateTimeStorage()
impl.OutputFunction = &arrowstore.ArrowStateTimeStorageOutputFunction{Store: store}
// ... run the coordinator ...

db, _ := sql.Open("duckdb", "")            // in-memory; or a file path for persistence
n, _ := duckdbstore.IngestToTable(ctx, db, store, "sim")

// The table is now ordinary SQL: a `time` column + one ARRAY<DOUBLE> column per partition.
db.QueryRow(`SELECT avg(sensor[1]) FROM sim WHERE time > 100`).Scan(&x)
```

`IngestToTable` registers the storage's finished Arrow record as a DuckDB view via the driver's
zero-copy `RegisterView` and materialises it with one `CREATE TABLE AS SELECT`. It requires the
storage to be row-aligned (every partition produced the same number of rows as the time axis);
otherwise it returns a clear error.

## How it stays zero-copy

Both this package and the DuckDB driver use `github.com/apache/arrow-go/v18`, so the Arrow
record produced by `arrowstore` crosses into DuckDB as shared Arrow arrays — no serialization,
no conversion. This is the interchange win arrowstore was built to enable.
