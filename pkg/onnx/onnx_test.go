//go:build onnx

package onnx

import (
	"math"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/api"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// modelWeights / modelBias mirror testdata/gen_model.py exactly: the model
// computes sigmoid(W @ x + b). The test recomputes this reference in Go and
// asserts the ONNX Runtime output agrees to float tolerance — the differential
// test for the frozen-inference partition.
var (
	modelWeights = [][]float64{{1.0, 2.0, 3.0}, {4.0, 5.0, 6.0}}
	modelBias    = []float64{0.5, -0.5}
)

const onnxTestModel = "testdata/affine_sigmoid.onnx"

// referenceOutput is the hand-computed sigmoid(W @ x + b).
func referenceOutput(x []float64) []float64 {
	out := make([]float64, len(modelBias))
	for i := range out {
		sum := modelBias[i]
		for j, w := range modelWeights[i] {
			sum += w * x[j]
		}
		out[i] = 1.0 / (1.0 + math.Exp(-sum))
	}
	return out
}

// skipIfNoORT skips when the ONNX Runtime shared library cannot be located, so
// CI without the runtime stays green rather than failing to link at runtime.
func skipIfNoORT(t *testing.T) {
	t.Helper()
	if resolveOrtLibraryPath("") == "" {
		t.Skip("ONNX Runtime shared library not found; set ONNXRUNTIME_LIB_PATH")
	}
}

func TestOnnxInference(t *testing.T) {
	skipIfNoORT(t)

	t.Run("resolves through the RegisterIteration hook and infers", func(t *testing.T) {
		// Go through api.ResolveIteration to exercise the whole path: the
		// downstream registration, the builder's field validation, and Configure.
		iteration, err := api.ResolveIteration(simulator.ComponentSpec{
			Type:   "onnx_inference",
			Fields: map[string]interface{}{"model_path": onnxTestModel},
		})
		if err != nil {
			t.Fatalf("ResolveIteration: %v", err)
		}
		settings := simulator.LoadSettingsFromYaml("./onnx_settings.yaml")
		iteration.Configure(0, settings)

		cases := [][]float64{
			{1.0, 0.0, 0.0},
			{0.0, 1.0, 0.0},
			{-1.0, 2.0, -0.5},
			{0.3, 0.3, 0.3},
		}
		for _, x := range cases {
			params := simulator.NewParams(map[string][]float64{"input": x})
			got := iteration.Iterate(&params, 0, nil, nil)
			want := referenceOutput(x)
			if !floats.EqualApprox(got, want, 1e-6) {
				t.Errorf("input %v: ONNX output %v, reference %v", x, got, want)
			}
		}
	})

	t.Run("unknown field is rejected", func(t *testing.T) {
		_, err := api.ResolveIteration(simulator.ComponentSpec{
			Type:   "onnx_inference",
			Fields: map[string]interface{}{"model_path": onnxTestModel, "nope": "x"},
		})
		if err == nil {
			t.Fatal("expected an error for an unknown field")
		}
	})

	t.Run("missing model_path is rejected", func(t *testing.T) {
		_, err := api.ResolveIteration(simulator.ComponentSpec{
			Type:   "onnx_inference",
			Fields: map[string]interface{}{},
		})
		if err == nil {
			t.Fatal("expected an error when model_path is absent")
		}
	})

	t.Run("runs through the test harness", func(t *testing.T) {
		// RunWithHarnesses runs the simulation twice and checks for NaNs, wrong
		// state widths, params mutation, history integrity, and statefulness
		// residue — so this also proves Configure fully re-initialises the cgo
		// session and the reused output buffer stays deterministic.
		settings := simulator.LoadSettingsFromYaml("./onnx_settings.yaml")
		iteration := &OnnxInferenceIteration{
			ModelPath:  onnxTestModel,
			InputParam: "input",
		}
		implementations := &simulator.Implementations{
			Iterations:      []simulator.Iteration{iteration},
			OutputCondition: &simulator.EveryStepOutputCondition{},
			OutputFunction:  &simulator.NilOutputFunction{},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
				MaxNumberOfSteps: 20,
			},
			TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		}
		if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
			t.Errorf("test harness failed: %v", err)
		}
	})

	t.Run("single-thread session options still infer correctly", func(t *testing.T) {
		// The threading knobs are opt-in; setting both pools to one thread (the
		// sensible choice for single-row inference across many partitions) must not
		// change the result — only the runtime's internal parallelism.
		iteration, err := api.ResolveIteration(simulator.ComponentSpec{
			Type: "onnx_inference",
			Fields: map[string]interface{}{
				"model_path":       onnxTestModel,
				"intra_op_threads": 1,
				"inter_op_threads": 1,
			},
		})
		if err != nil {
			t.Fatalf("ResolveIteration: %v", err)
		}
		iteration.Configure(0, simulator.LoadSettingsFromYaml("./onnx_settings.yaml"))
		x := []float64{0.3, 0.3, 0.3}
		params := simulator.NewParams(map[string][]float64{"input": x})
		got := iteration.Iterate(&params, 0, nil, nil)
		if want := referenceOutput(x); !floats.EqualApprox(got, want, 1e-6) {
			t.Errorf("single-thread output %v, reference %v", got, want)
		}
	})
}

const multiInputModel = "testdata/affine_theta_sigmoid.onnx"

// referenceTheta is the hand-computed sigmoid(W @ features + theta) for the
// two-input model, where theta is a *parameter* vector fed as its own input.
func referenceTheta(features, theta []float64) []float64 {
	out := make([]float64, len(theta))
	for i := range out {
		sum := theta[i]
		for j, w := range modelWeights[i] {
			sum += w * features[j]
		}
		out[i] = 1.0 / (1.0 + math.Exp(-sum))
	}
	return out
}

// TestOnnxMultiInput covers the {param -> model input} mapping: a feature vector
// and a separate parameter vector (theta) feed a two-input model. This is the
// shape that lets the framework's optimisation / SBI tools tune a model's
// parameters — theta arrives as an ordinary params vector, so a sampler perturbs
// it exactly like any other partition parameter.
func TestOnnxMultiInput(t *testing.T) {
	skipIfNoORT(t)

	spec := func() simulator.ComponentSpec {
		return simulator.ComponentSpec{
			Type: "onnx_inference",
			Fields: map[string]interface{}{
				"model_path": multiInputModel,
				// yaml.v2 delivers a nested map as map[interface{}]interface{};
				// use that shape here so the builder's normalisation is exercised.
				"inputs": map[interface{}]interface{}{
					"features": "features",
					"theta":    "theta",
				},
			},
		}
	}

	t.Run("binds two inputs and infers; tuning theta tracks the reference", func(t *testing.T) {
		iteration, err := api.ResolveIteration(spec())
		if err != nil {
			t.Fatalf("ResolveIteration: %v", err)
		}
		iteration.Configure(0, simulator.LoadSettingsFromYaml("./onnx_multi_input_settings.yaml"))

		cases := []struct{ features, theta []float64 }{
			{[]float64{1, 0, 0}, []float64{0.5, -0.5}},
			{[]float64{1, 0, 0}, []float64{-2.0, 3.0}}, // same features, tuned theta
			{[]float64{0.3, 0.3, 0.3}, []float64{0.0, 0.0}},
			{[]float64{-1, 2, -0.5}, []float64{1.0, 1.0}},
		}
		for _, c := range cases {
			params := simulator.NewParams(map[string][]float64{
				"features": c.features, "theta": c.theta,
			})
			got := iteration.Iterate(&params, 0, nil, nil)
			want := referenceTheta(c.features, c.theta)
			if !floats.EqualApprox(got, want, 1e-6) {
				t.Errorf("features %v theta %v: output %v, reference %v",
					c.features, c.theta, got, want)
			}
		}
	})

	t.Run("runs through the test harness", func(t *testing.T) {
		iteration, err := api.ResolveIteration(spec())
		if err != nil {
			t.Fatalf("ResolveIteration: %v", err)
		}
		implementations := &simulator.Implementations{
			Iterations:      []simulator.Iteration{iteration},
			OutputCondition: &simulator.EveryStepOutputCondition{},
			OutputFunction:  &simulator.NilOutputFunction{},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
				MaxNumberOfSteps: 20,
			},
			TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		}
		settings := simulator.LoadSettingsFromYaml("./onnx_multi_input_settings.yaml")
		if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
			t.Errorf("test harness failed: %v", err)
		}
	})

	t.Run("binding fewer inputs than the model has is rejected", func(t *testing.T) {
		iteration, err := api.ResolveIteration(simulator.ComponentSpec{
			Type: "onnx_inference",
			Fields: map[string]interface{}{
				"model_path": multiInputModel,
				"inputs":     map[interface{}]interface{}{"features": "features"},
			},
		})
		if err != nil {
			t.Fatalf("ResolveIteration: %v", err)
		}
		defer func() {
			if recover() == nil {
				t.Error("expected Configure to panic when an input is unbound")
			}
		}()
		iteration.Configure(0, simulator.LoadSettingsFromYaml("./onnx_multi_input_settings.yaml"))
	})
}

// TestOnnxBuilderValidation covers config validation that fails before any model
// is loaded, so it needs no ONNX Runtime library.
func TestOnnxBuilderValidation(t *testing.T) {
	cases := map[string]map[string]interface{}{
		"inputs and shorthand are mutually exclusive": {
			"model_path":  "m.onnx",
			"inputs":      map[interface{}]interface{}{"a": "a"},
			"input_param": "x",
		},
		"empty inputs map": {
			"model_path": "m.onnx",
			"inputs":     map[interface{}]interface{}{},
		},
		"non-string input target": {
			"model_path": "m.onnx",
			"inputs":     map[interface{}]interface{}{"a": 3},
		},
		"negative thread count": {
			"model_path":       "m.onnx",
			"intra_op_threads": -1,
		},
		"non-integer thread count": {
			"model_path":       "m.onnx",
			"intra_op_threads": "lots",
		},
	}
	for name, fields := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := BuildIteration(simulator.ComponentSpec{
				Type: "onnx_inference", Fields: fields,
			}); err == nil {
				t.Errorf("expected a build error for %s", name)
			}
		})
	}
}
