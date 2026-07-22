// The distributed stochadex CLI. This is a SEPARATE module on purpose: it bundles the
// opt-in egress modules (arrowstore, duckdbstore) that the engine's own go.mod
// deliberately excludes, so the engine stays lean and WASM-clean for everyone who
// imports it as a library while the shipped binary still carries the integrations.
//
// One main package, two builds:
//   - pure Go (no tags, CGO off) — engine + Postgres + Arrow; cross-compiles everywhere.
//   - CGO with `-tags "cblas duckdb_arrow"` — adds an optimised system BLAS and DuckDB.
//
// duckdbstore is required unconditionally so the module graph resolves, but its code is
// only compiled under the duckdb_arrow tag, which keeps the pure-Go build cgo-free.
module github.com/umbralcalc/stochadex/cmd/stochadex-full

go 1.25.0

require (
	github.com/apache/arrow-go/v18 v18.6.0
	github.com/marcboeker/go-duckdb/v2 v2.4.3
	github.com/umbralcalc/stochadex v0.5.3
	github.com/umbralcalc/stochadex/pkg/arrowstore v0.0.0
	github.com/umbralcalc/stochadex/pkg/duckdbstore v0.0.0
)

require (
	github.com/akamensky/argparse v1.4.0 // indirect
	github.com/duckdb/duckdb-go-bindings v0.1.21 // indirect
	github.com/duckdb/duckdb-go-bindings/darwin-amd64 v0.1.21 // indirect
	github.com/duckdb/duckdb-go-bindings/darwin-arm64 v0.1.21 // indirect
	github.com/duckdb/duckdb-go-bindings/linux-amd64 v0.1.21 // indirect
	github.com/duckdb/duckdb-go-bindings/linux-arm64 v0.1.21 // indirect
	github.com/duckdb/duckdb-go-bindings/windows-amd64 v0.1.21 // indirect
	github.com/go-echarts/go-echarts/v2 v2.6.3 // indirect
	github.com/go-gota/gota v0.12.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/google/flatbuffers v25.12.19+incompatible // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/marcboeker/go-duckdb/arrowmapping v0.0.21 // indirect
	github.com/marcboeker/go-duckdb/mapping v0.0.21 // indirect
	github.com/pierrec/lz4/v4 v4.1.26 // indirect
	github.com/scientificgo/special v0.0.2 // indirect
	github.com/zeebo/xxh3 v1.1.0 // indirect
	golang.org/x/exp v0.0.0-20260112195511-716be5621a96 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/tools v0.43.0 // indirect
	gonum.org/v1/gonum v0.17.0 // indirect
	gonum.org/v1/netlib v0.0.0-20230729102104-8b8060e7531f // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

// Local development builds against the sibling modules in this tree; external users get
// the pinned requires above. These replaces are ignored when consumed as a dependency.
replace github.com/umbralcalc/stochadex => ../../

replace github.com/umbralcalc/stochadex/pkg/arrowstore => ../../pkg/arrowstore

replace github.com/umbralcalc/stochadex/pkg/duckdbstore => ../../pkg/duckdbstore
