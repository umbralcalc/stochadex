//go:build onnx

package onnx

import (
	"fmt"
	"os"
	"sync"

	"github.com/umbralcalc/stochadex/pkg/api"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	ort "github.com/yalue/onnxruntime_go"
)

// init self-registers the onnx_inference spelling with the engine's config
// surface. Importing this module (under the onnx build tag) is the opt-in: a CLI
// or a downstream library that blank-imports it gains {type: onnx_inference}
// without the engine core ever depending on the cgo ONNX Runtime.
func init() {
	api.RegisterIteration("onnx_inference", BuildIteration)
}

// ortInitOnce guards the process-global ONNX Runtime environment initialisation.
// SetSharedLibraryPath and InitializeEnvironment are global to the process, so
// the first onnx_inference partition to Configure wins the library path; a second
// partition naming a different path is ignored (documented, not an error).
var ortInitOnce sync.Once

// ensureOrtInitialized points the binding at the ONNX Runtime shared library and
// initialises the global environment exactly once. It panics on failure because
// Iteration.Configure cannot return an error and a missing runtime is a setup
// (config) fault, surfaced loudly at startup rather than mid-run.
func ensureOrtInitialized(sharedLibraryPath string) {
	ortInitOnce.Do(func() {
		if path := resolveOrtLibraryPath(sharedLibraryPath); path != "" {
			ort.SetSharedLibraryPath(path)
		}
		if err := ort.InitializeEnvironment(); err != nil {
			panic(fmt.Sprintf(
				"onnx_inference: could not initialise the ONNX Runtime "+
					"(is the shared library present? set shared_library_path or "+
					"ONNXRUNTIME_LIB_PATH): %v", err,
			))
		}
	})
}

// resolveOrtLibraryPath finds the ONNX Runtime shared library: the spec field
// first, then ONNXRUNTIME_LIB_PATH, then conventional install locations. Returns
// "" to let the binding fall back to its own default search (so a system-wide
// install still works with no configuration).
func resolveOrtLibraryPath(fromSpec string) string {
	if fromSpec != "" {
		return fromSpec
	}
	if env := os.Getenv("ONNXRUNTIME_LIB_PATH"); env != "" {
		return env
	}
	for _, candidate := range []string{
		"/opt/homebrew/lib/libonnxruntime.dylib",
		"/usr/local/lib/libonnxruntime.dylib",
		"/usr/local/lib/libonnxruntime.so",
		"/usr/lib/libonnxruntime.so",
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// OnnxInferenceIteration runs a frozen ONNX model as a partition. Each step it
// reads the feature vector from params[InputParam], runs the model, and returns
// the flattened model output as the next state. All tensors are allocated once in
// Configure and reused every step (Invariant B): Iterate copies into a pinned
// []float32 input, runs, and copies the output back out — no per-step allocation.
type OnnxInferenceIteration struct {
	// Config (set by the builder, immutable across runs).
	ModelPath         string
	InputParam        string
	InputName         string
	OutputName        string
	SharedLibraryPath string

	// Runtime state (re-created in Configure so the iteration is stateless
	// between the harness's two runs).
	session      *ort.AdvancedSession
	inputData    []float32 // backing slice of inputTensor, written each step
	outputData   []float32 // backing slice of outputTensor, read each step
	inputTensor  *ort.Tensor[float32]
	outputTensor *ort.Tensor[float32]
	out          []float64 // reusable float64 output returned to the coordinator
}

func (o *OnnxInferenceIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	ensureOrtInitialized(o.SharedLibraryPath)

	// Tear down any session/tensors from a previous run so a second Configure
	// (the harness runs the simulation twice) starts clean and leaks nothing.
	o.destroy()

	inputs, outputs, err := ort.GetInputOutputInfo(o.ModelPath)
	if err != nil {
		panic(fmt.Sprintf(
			"onnx_inference: could not read model %q: %v", o.ModelPath, err,
		))
	}
	input := pickTensorInfo(inputs, o.InputName, "input", o.ModelPath)
	output := pickTensorInfo(outputs, o.OutputName, "output", o.ModelPath)

	inShape := concreteShape(input.Dimensions)
	outShape := concreteShape(output.Dimensions)

	o.inputData = make([]float32, inShape.FlattenedSize())
	o.outputData = make([]float32, outShape.FlattenedSize())
	o.out = make([]float64, len(o.outputData))

	o.inputTensor, err = ort.NewTensor(inShape, o.inputData)
	if err != nil {
		panic(fmt.Sprintf("onnx_inference: allocating input tensor: %v", err))
	}
	o.outputTensor, err = ort.NewTensor(outShape, o.outputData)
	if err != nil {
		panic(fmt.Sprintf("onnx_inference: allocating output tensor: %v", err))
	}
	o.session, err = ort.NewAdvancedSession(
		o.ModelPath,
		[]string{input.Name},
		[]string{output.Name},
		[]ort.Value{o.inputTensor},
		[]ort.Value{o.outputTensor},
		nil,
	)
	if err != nil {
		panic(fmt.Sprintf(
			"onnx_inference: creating session for %q: %v", o.ModelPath, err,
		))
	}
}

func (o *OnnxInferenceIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	input := params.Get(o.InputParam)
	if len(input) != len(o.inputData) {
		panic(fmt.Sprintf(
			"onnx_inference: model %q expects an input of length %d, but params[%q] "+
				"has length %d", o.ModelPath, len(o.inputData), o.InputParam, len(input),
		))
	}
	for i, v := range input {
		o.inputData[i] = float32(v)
	}
	if err := o.session.Run(); err != nil {
		panic(fmt.Sprintf("onnx_inference: running model %q: %v", o.ModelPath, err))
	}
	for i, v := range o.outputData {
		o.out[i] = float64(v)
	}
	return o.out
}

// destroy releases the cgo session and tensor handles, tolerating a not-yet- or
// already-configured iteration (nil handles).
func (o *OnnxInferenceIteration) destroy() {
	if o.session != nil {
		_ = o.session.Destroy()
		o.session = nil
	}
	if o.inputTensor != nil {
		_ = o.inputTensor.Destroy()
		o.inputTensor = nil
	}
	if o.outputTensor != nil {
		_ = o.outputTensor.Destroy()
		o.outputTensor = nil
	}
}

// pickTensorInfo resolves a named model input/output, defaulting to the sole
// entry when no name is configured. A model with several inputs/outputs and no
// name given is a config error, reported with the available names.
func pickTensorInfo(
	infos []ort.InputOutputInfo,
	wantName, role, modelPath string,
) ort.InputOutputInfo {
	if wantName != "" {
		for _, info := range infos {
			if info.Name == wantName {
				requireFloatTensor(info, role, modelPath)
				return info
			}
		}
		panic(fmt.Sprintf(
			"onnx_inference: model %q has no %s named %q (available: %s)",
			modelPath, role, wantName, tensorInfoNames(infos),
		))
	}
	if len(infos) != 1 {
		panic(fmt.Sprintf(
			"onnx_inference: model %q has %d %ss (%s); name one with %s_name",
			modelPath, len(infos), role, tensorInfoNames(infos), role,
		))
	}
	requireFloatTensor(infos[0], role, modelPath)
	return infos[0]
}

// requireFloatTensor rejects models whose input/output is not a float32 tensor;
// this first cut binds float32 I/O (the common export dtype) exactly.
func requireFloatTensor(info ort.InputOutputInfo, role, modelPath string) {
	if info.OrtValueType != ort.ONNXTypeTensor ||
		info.DataType != ort.TensorElementDataTypeFloat {
		panic(fmt.Sprintf(
			"onnx_inference: model %q %s %q must be a float32 tensor (got %s); "+
				"re-export with float32 I/O or use a bespoke Go iteration",
			modelPath, role, info.Name, info.String(),
		))
	}
}

func tensorInfoNames(infos []ort.InputOutputInfo) string {
	names := make([]string, len(infos))
	for i, info := range infos {
		names[i] = info.Name
	}
	return fmt.Sprintf("%v", names)
}

// concreteShape replaces dynamic dimensions (<= 0, e.g. a symbolic batch size)
// with 1, giving the fixed single-step shape this partition binds its buffers to.
func concreteShape(shape ort.Shape) ort.Shape {
	concrete := shape.Clone()
	for i, dim := range concrete {
		if dim <= 0 {
			concrete[i] = 1
		}
	}
	return concrete
}

// buildOnnxInference constructs an OnnxInferenceIteration from a data spec,
// validating fields strictly (an unknown key is an error, matching the rest of
// the config surface).
func BuildIteration(
	spec simulator.ComponentSpec,
) (simulator.Iteration, error) {
	iteration := &OnnxInferenceIteration{InputParam: "input"}
	for key, value := range spec.Fields {
		target := ""
		switch key {
		case "model_path":
			target = "model_path"
		case "input_param":
			target = "input_param"
		case "input_name":
			target = "input_name"
		case "output_name":
			target = "output_name"
		case "shared_library_path":
			target = "shared_library_path"
		default:
			return nil, fmt.Errorf("onnx_inference: unknown field %q", key)
		}
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf(
				"onnx_inference: field %q must be a string, got %T", key, value,
			)
		}
		switch target {
		case "model_path":
			iteration.ModelPath = str
		case "input_param":
			iteration.InputParam = str
		case "input_name":
			iteration.InputName = str
		case "output_name":
			iteration.OutputName = str
		case "shared_library_path":
			iteration.SharedLibraryPath = str
		}
	}
	if iteration.ModelPath == "" {
		return nil, fmt.Errorf("onnx_inference: model_path is required")
	}
	return iteration, nil
}
