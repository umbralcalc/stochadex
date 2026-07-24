//go:build onnx

// Wires the opt-in ONNX inference partition into the distributed CLI. The blank
// import pulls in github.com/umbralcalc/stochadex/pkg/onnx, whose init()
// self-registers the {type: onnx_inference} spelling via api.RegisterIteration —
// the cgo ONNX Runtime dependency lives entirely in that separate module, so it
// is only compiled into this binary under the `onnx` build tag. Here we only add
// the feature label reported by --version.
package main

import (
	_ "github.com/umbralcalc/stochadex/pkg/onnx"
)

func init() { features = append(features, "onnx") }
