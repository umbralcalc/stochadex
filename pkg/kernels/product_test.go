package kernels

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

func TestProductIntegationKernel(t *testing.T) {
	t.Run(
		"test that the product integration kernel runs",
		func(t *testing.T) {
			params := simulator.NewParams(make(map[string][]float64))
			kernel := &ProductIntegrationKernel{
				KernelA: &ConstantIntegrationKernel{},
				KernelB: &ConstantIntegrationKernel{},
			}
			kernel.Configure(0, &simulator.Settings{
				Iterations: []simulator.IterationSettings{
					{Name: "test", Params: params},
				},
			})
			valueOne := kernel.Evaluate(
				[]float64{0.3, 1.0, 0.0},
				[]float64{0.5, 1.1, 1.0},
				1.0,
				0.0,
			)
			kernel.SetParams(&params)
			valueTwo := kernel.Evaluate(
				[]float64{0.3, 1.0, 0.0},
				[]float64{0.5, 1.1, 1.0},
				1.0,
				0.0,
			)
			if floats.HasNaN([]float64{valueOne, valueTwo}) {
				t.Errorf("NaN present in values: %f, %f", valueOne, valueTwo)
			}
		},
	)
}
