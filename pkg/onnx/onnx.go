//go:build onnx

package onnx

import (
	"fmt"
	"math"
	"os"
	"sort"
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

// OnnxInput binds one params key to one named model input, so a partition can
// feed a multi-input model — e.g. a feature vector under one key and a tunable
// parameter vector under another, the latter driven by the framework's
// optimisation / SBI tools exactly like any other partition parameter.
type OnnxInput struct {
	// ParamKey is the params key the input vector is read from each step.
	ParamKey string
	// ModelInputName is the ONNX graph input to feed. Empty means the model's
	// sole input (only valid when the model has exactly one).
	ModelInputName string
}

// boundInput is a resolved OnnxInput: its params key plus the preallocated
// []float32 backing slice of its bound tensor, written each step.
type boundInput struct {
	paramKey string
	data     []float32
	tensor   *ort.Tensor[float32]
}

// OnnxInferenceIteration runs a frozen ONNX model as a partition. Each step it
// reads each bound input vector from params, runs the model, and returns the
// flattened model output as the next state. All tensors are allocated once in
// Configure and reused every step (Invariant B): Iterate copies into pinned
// []float32 inputs, runs, and copies the output back out — no per-step allocation.
type OnnxInferenceIteration struct {
	// Config (set by the builder, immutable across runs).
	ModelPath         string
	OutputName        string
	SharedLibraryPath string
	// Inputs binds params keys to named model inputs. When empty, the single-input
	// shorthand below is used instead. Multi-input unlocks parameter tuning: a
	// parameter vector arrives as its own input alongside the features.
	Inputs []OnnxInput
	// InputParam / InputName are the single-input shorthand, used only when Inputs
	// is empty. InputParam defaults to "input"; InputName defaults to the sole
	// model input.
	InputParam string
	InputName  string
	// IntraOpThreads / InterOpThreads size the session's ONNX Runtime thread pools.
	// <= 0 leaves the runtime default. For single-row inference across many
	// concurrent partitions, setting both to 1 avoids thread oversubscription (each
	// session otherwise spins up its own default pool); the caller tunes this
	// rather than the engine guessing.
	IntraOpThreads int
	InterOpThreads int

	// Runtime state (re-created in Configure so the iteration is stateless
	// between the harness's two runs).
	session      *ort.AdvancedSession
	boundInputs  []boundInput
	outputData   []float32 // backing slice of outputTensor, read each step
	outputTensor *ort.Tensor[float32]
	out          []float64 // reusable float64 output returned to the coordinator
}

// effectiveInputs returns the configured multi-input bindings, or the single-input
// shorthand (defaulting the params key to "input") when none are set.
func (o *OnnxInferenceIteration) effectiveInputs() []OnnxInput {
	if len(o.Inputs) > 0 {
		return o.Inputs
	}
	key := o.InputParam
	if key == "" {
		key = "input"
	}
	return []OnnxInput{{ParamKey: key, ModelInputName: o.InputName}}
}

func (o *OnnxInferenceIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	ensureOrtInitialized(o.SharedLibraryPath)

	// Tear down any session/tensors from a previous run so a second Configure
	// (the harness runs the simulation twice) starts clean and leaks nothing.
	o.destroy()

	inputInfos, outputInfos, err := ort.GetInputOutputInfo(o.ModelPath)
	if err != nil {
		panic(fmt.Sprintf(
			"onnx_inference: could not read model %q: %v", o.ModelPath, err,
		))
	}

	output := pickTensorInfo(outputInfos, o.OutputName, "output", o.ModelPath)
	outShape := concreteShape(output.Dimensions)
	o.outputData = make([]float32, outShape.FlattenedSize())
	o.out = make([]float64, len(o.outputData))
	o.outputTensor, err = ort.NewTensor(outShape, o.outputData)
	if err != nil {
		panic(fmt.Sprintf("onnx_inference: allocating output tensor: %v", err))
	}

	bindings := o.effectiveInputs()
	if len(bindings) != len(inputInfos) {
		panic(fmt.Sprintf(
			"onnx_inference: model %q has %d input(s) (%s) but the config binds %d; "+
				"bind every model input", o.ModelPath, len(inputInfos),
			tensorInfoNames(inputInfos), len(bindings),
		))
	}
	o.boundInputs = make([]boundInput, len(bindings))
	inputNames := make([]string, len(bindings))
	inputValues := make([]ort.Value, len(bindings))
	seen := make(map[string]bool, len(bindings))
	for i, binding := range bindings {
		info := pickTensorInfo(inputInfos, binding.ModelInputName, "input", o.ModelPath)
		if seen[info.Name] {
			panic(fmt.Sprintf(
				"onnx_inference: model input %q is bound more than once", info.Name,
			))
		}
		seen[info.Name] = true
		shape := concreteShape(info.Dimensions)
		data := make([]float32, shape.FlattenedSize())
		tensor, err := ort.NewTensor(shape, data)
		if err != nil {
			panic(fmt.Sprintf("onnx_inference: allocating input tensor: %v", err))
		}
		o.boundInputs[i] = boundInput{paramKey: binding.ParamKey, data: data, tensor: tensor}
		inputNames[i] = info.Name
		inputValues[i] = tensor
	}

	// Thread-pool sizing is opt-in: build SessionOptions only when asked, so the
	// default path is byte-for-byte the runtime default. Options are cloned into
	// the session at creation, so destroying them afterwards is safe.
	var options *ort.SessionOptions
	if o.IntraOpThreads > 0 || o.InterOpThreads > 0 {
		options, err = ort.NewSessionOptions()
		if err != nil {
			panic(fmt.Sprintf("onnx_inference: allocating session options: %v", err))
		}
		defer func() { _ = options.Destroy() }()
		if o.IntraOpThreads > 0 {
			if err = options.SetIntraOpNumThreads(o.IntraOpThreads); err != nil {
				panic(fmt.Sprintf("onnx_inference: setting intra-op threads: %v", err))
			}
		}
		if o.InterOpThreads > 0 {
			if err = options.SetInterOpNumThreads(o.InterOpThreads); err != nil {
				panic(fmt.Sprintf("onnx_inference: setting inter-op threads: %v", err))
			}
		}
	}

	o.session, err = ort.NewAdvancedSession(
		o.ModelPath,
		inputNames,
		[]string{output.Name},
		inputValues,
		[]ort.Value{o.outputTensor},
		options,
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
	for _, binding := range o.boundInputs {
		input := params.Get(binding.paramKey)
		if len(input) != len(binding.data) {
			panic(fmt.Sprintf(
				"onnx_inference: model %q input from params[%q] must have length %d, "+
					"got %d", o.ModelPath, binding.paramKey, len(binding.data), len(input),
			))
		}
		for i, v := range input {
			binding.data[i] = float32(v)
		}
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
	for i := range o.boundInputs {
		if o.boundInputs[i].tensor != nil {
			_ = o.boundInputs[i].tensor.Destroy()
		}
	}
	o.boundInputs = nil
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

// BuildIteration constructs an OnnxInferenceIteration from a data spec, validating
// fields strictly (an unknown key is an error, matching the rest of the config
// surface). Inputs are given either as the single-input shorthand
// (input_param / input_name) or as a multi-input map (inputs: {param: model_input});
// the two forms are mutually exclusive.
func BuildIteration(
	spec simulator.ComponentSpec,
) (simulator.Iteration, error) {
	iteration := &OnnxInferenceIteration{}
	var hasInputs, hasShorthand bool
	for key, value := range spec.Fields {
		var err error
		switch key {
		case "model_path":
			iteration.ModelPath, err = onnxString(key, value)
		case "input_param":
			iteration.InputParam, err = onnxString(key, value)
			hasShorthand = true
		case "input_name":
			iteration.InputName, err = onnxString(key, value)
			hasShorthand = true
		case "output_name":
			iteration.OutputName, err = onnxString(key, value)
		case "shared_library_path":
			iteration.SharedLibraryPath, err = onnxString(key, value)
		case "intra_op_threads":
			iteration.IntraOpThreads, err = onnxInt(key, value)
		case "inter_op_threads":
			iteration.InterOpThreads, err = onnxInt(key, value)
		case "inputs":
			iteration.Inputs, err = onnxInputMap(value)
			hasInputs = true
		default:
			return nil, fmt.Errorf("onnx_inference: unknown field %q", key)
		}
		if err != nil {
			return nil, err
		}
	}
	if iteration.ModelPath == "" {
		return nil, fmt.Errorf("onnx_inference: model_path is required")
	}
	if hasInputs && hasShorthand {
		return nil, fmt.Errorf(
			"onnx_inference: use either inputs (a {param: model_input} map) or " +
				"input_param/input_name, not both",
		)
	}
	if hasInputs && len(iteration.Inputs) == 0 {
		return nil, fmt.Errorf("onnx_inference: inputs must not be empty")
	}
	if iteration.IntraOpThreads < 0 || iteration.InterOpThreads < 0 {
		return nil, fmt.Errorf("onnx_inference: thread counts must be >= 0")
	}
	return iteration, nil
}

// onnxString reads a string-typed spec field.
func onnxString(key string, value interface{}) (string, error) {
	str, ok := value.(string)
	if !ok {
		return "", fmt.Errorf(
			"onnx_inference: field %q must be a string, got %T", key, value,
		)
	}
	return str, nil
}

// onnxInt reads an integer-typed spec field (YAML may deliver it as int or, if
// written with a decimal point, float64).
func onnxInt(key string, value interface{}) (int, error) {
	switch number := value.(type) {
	case int:
		return number, nil
	case int64:
		return int(number), nil
	case float64:
		if number != math.Trunc(number) {
			return 0, fmt.Errorf(
				"onnx_inference: field %q must be an integer, got %v", key, number,
			)
		}
		return int(number), nil
	default:
		return 0, fmt.Errorf(
			"onnx_inference: field %q must be an integer, got %T", key, value,
		)
	}
}

// onnxInputMap reads the inputs mapping ({param: model_input_name}) into a
// deterministic, param-key-sorted slice of bindings. Sorting makes the session's
// input order stable across the harness's two runs regardless of map iteration.
func onnxInputMap(value interface{}) ([]OnnxInput, error) {
	raw, err := onnxStringKeyMap(value)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	inputs := make([]OnnxInput, 0, len(keys))
	for _, key := range keys {
		name, ok := raw[key].(string)
		if !ok || name == "" {
			return nil, fmt.Errorf(
				"onnx_inference: inputs[%q] must be a non-empty model input name, got %v",
				key, raw[key],
			)
		}
		inputs = append(inputs, OnnxInput{ParamKey: key, ModelInputName: name})
	}
	return inputs, nil
}

// onnxStringKeyMap normalises a YAML mapping to string keys, accepting both the
// map[string]interface{} and map[interface{}]interface{} shapes yaml.v2 produces.
func onnxStringKeyMap(value interface{}) (map[string]interface{}, error) {
	switch mapping := value.(type) {
	case map[string]interface{}:
		return mapping, nil
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(mapping))
		for key, element := range mapping {
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf(
					"onnx_inference: inputs keys must be strings, got %T", key,
				)
			}
			out[name] = element
		}
		return out, nil
	default:
		return nil, fmt.Errorf(
			"onnx_inference: inputs must be a {param: model_input_name} mapping, got %T",
			value,
		)
	}
}
