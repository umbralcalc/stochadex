// Separate opt-in module: keeps Apache Arrow (and its dependency tree, incl. the gonum
// v0.17 requirement) entirely out of the core engine's go.mod. The engine stays lean and
// WASM-clean; consumers opt in by importing this module.
module github.com/umbralcalc/stochadex/pkg/arrowstore

go 1.25.0

require (
	github.com/apache/arrow-go/v18 v18.6.0
	github.com/umbralcalc/stochadex v0.2.0
)

require (
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/google/flatbuffers v25.12.19+incompatible // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/zeebo/xxh3 v1.1.0 // indirect
	golang.org/x/exp v0.0.0-20260112195511-716be5621a96 // indirect
	golang.org/x/sys v0.43.0 // indirect
	gonum.org/v1/gonum v0.17.0 // indirect
	gonum.org/v1/netlib v0.0.0-20230729102104-8b8060e7531f // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

// Local development builds against the engine in the parent directory. External users get
// the pinned require above; this replace is ignored when the module is consumed as a dependency.
replace github.com/umbralcalc/stochadex => ../../
