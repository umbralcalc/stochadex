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
}
