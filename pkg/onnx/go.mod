// Separate opt-in module: keeps the ONNX Runtime Go binding (CGO, dlopens the
// ONNX Runtime shared library) entirely out of the core engine's go.mod, exactly
// as arrowstore / duckdbstore / s3store do for their heavy dependencies. The
// engine stays lean and CGO_ENABLED=0-clean; consumers who want to run a frozen
// ONNX model behind an Iteration opt in by importing this module and building
// with `-tags onnx` and CGO enabled.
module github.com/umbralcalc/stochadex/pkg/onnx

go 1.25.0

require (
	github.com/umbralcalc/stochadex v0.5.3
	github.com/yalue/onnxruntime_go v1.31.0
	gonum.org/v1/gonum v0.17.0
)

require (
	github.com/akamensky/argparse v1.4.0 // indirect
	github.com/go-echarts/go-echarts/v2 v2.6.3 // indirect
	github.com/go-gota/gota v0.12.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/scientificgo/special v0.0.2 // indirect
	golang.org/x/net v0.44.0 // indirect
	golang.org/x/tools v0.37.0 // indirect
	gonum.org/v1/netlib v0.0.0-20230729102104-8b8060e7531f // indirect
	google.golang.org/protobuf v1.36.9 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

// Local development builds against the sibling engine module in the tree; external
// users get the pinned require above. This replace is ignored when the module is
// consumed as a dependency.
replace github.com/umbralcalc/stochadex => ../../
