// Package onnx is an opt-in inference partition: it runs a frozen ONNX model
// behind the engine's simulator.Iteration interface, so a model trained upstream
// (in Python — sklearn, XGBoost, a small neural net — and exported to .onnx) can
// be a component in a stochadex simulation. Each step the partition reads a
// feature vector from its params, runs the model through a cgo ONNX Runtime
// session, and returns the model output as its next state.
//
// # Why a separate module
//
// This is a SEPARATE module on purpose, exactly like pkg/duckdbstore and
// pkg/s3store: it pulls in a cgo dependency (github.com/yalue/onnxruntime_go) and
// the ONNX Runtime shared library, neither of which the engine's own go.mod
// carries. The engine core therefore stays lean and CGO_ENABLED=0-clean (it
// cross-compiles and builds to WASM for everyone who imports it as a library),
// while consumers who want inference opt in by importing this module. The
// implementation is behind the `onnx` build tag and CGO must be enabled.
//
// # How the opt-in works
//
// Importing this module (under the onnx tag) self-registers the {type:
// onnx_inference} spelling with the engine's config surface through
// api.RegisterIteration — the same downstream-registration hook the Arrow, S3 and
// DuckDB spellings use (RegisterDataSource / simulator.RegisterComponent). The
// engine core never imports this package; a CLI or a downstream library reaches
// it with a blank import, e.g. cmd/stochadex does so under its onnx tag.
//
// # Config surface
//
// A partition uses it as:
//
//	iteration:
//	  type: onnx_inference
//	  model_path: model.onnx        # required
//	  input_param: input            # params key holding the feature vector (default "input")
//	  input_name: ...               # ONNX graph input name (default: the sole input)
//	  output_name: ...              # ONNX graph output name (default: the sole output)
//	  shared_library_path: ...      # ONNX Runtime library (see below)
//
// The feature vector is wired in like any other partition input — a static
// params: entry, or params_from_upstream from a producing partition. The model
// output becomes the partition's state, so its state_width must equal the
// flattened output length. This first cut binds a single float32 input tensor and
// a single float32 output tensor (the common export dtype); dynamic dimensions
// such as a symbolic batch size are pinned to 1 for single-step inference.
//
// # ONNX Runtime shared library
//
// The library is a system dependency (mirroring the cblas "CGO with a system
// BLAS" story rather than vendoring per-platform binaries), located at run time
// in this order: the spec's shared_library_path, then the ONNXRUNTIME_LIB_PATH
// environment variable, then a short list of conventional install locations, then
// the binding's own default search.
//
// # Scope — inference only
//
// The partition is a pure, allocation-free map from input to prediction: all
// tensors are allocated once in Configure and reused every step (copy into a
// pinned input slice, run, copy out). Fitting and refitting stay upstream, per
// the engine's generative/inferential repo boundary — a downstream repo hands the
// engine a new .onnx artifact; this package never trains.
package onnx
