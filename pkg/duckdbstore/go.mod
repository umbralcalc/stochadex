// Separate opt-in module: keeps the DuckDB Go driver (CGO, statically-linked DuckDB C++ lib,
// not WASM-compatible) entirely out of the engine's and arrowstore's go.mod. Build with
// `-tags duckdb_arrow` and CGO enabled. Consumers opt in by importing this module.
module github.com/umbralcalc/stochadex/pkg/duckdbstore

go 1.25.0

require (
	github.com/apache/arrow-go/v18 v18.6.0
	github.com/marcboeker/go-duckdb/v2 v2.4.3
	github.com/umbralcalc/stochadex/pkg/arrowstore v0.0.0
)

require (
	github.com/duckdb/duckdb-go-bindings v0.1.21 // indirect
	github.com/duckdb/duckdb-go-bindings/darwin-amd64 v0.1.21 // indirect
	github.com/duckdb/duckdb-go-bindings/darwin-arm64 v0.1.21 // indirect
	github.com/duckdb/duckdb-go-bindings/linux-amd64 v0.1.21 // indirect
	github.com/duckdb/duckdb-go-bindings/linux-arm64 v0.1.21 // indirect
	github.com/duckdb/duckdb-go-bindings/windows-amd64 v0.1.21 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/google/flatbuffers v25.12.19+incompatible // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/marcboeker/go-duckdb/arrowmapping v0.0.21 // indirect
	github.com/marcboeker/go-duckdb/mapping v0.0.21 // indirect
	github.com/pierrec/lz4/v4 v4.1.26 // indirect
	github.com/rogpeppe/go-internal v1.15.0 // indirect
	github.com/umbralcalc/stochadex v0.2.0 // indirect
	github.com/zeebo/xxh3 v1.1.0 // indirect
	golang.org/x/exp v0.0.0-20260112195511-716be5621a96 // indirect
	golang.org/x/sys v0.43.0 // indirect
	gonum.org/v1/gonum v0.17.0 // indirect
	gonum.org/v1/netlib v0.0.0-20230729102104-8b8060e7531f // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

// Local development builds against the sibling modules in the tree; external users get the
// pinned requires above. These replaces are ignored when the module is consumed as a dependency.
replace github.com/umbralcalc/stochadex => ../../

replace github.com/umbralcalc/stochadex/pkg/arrowstore => ../arrowstore
