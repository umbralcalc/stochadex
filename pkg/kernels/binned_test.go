package kernels

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

func TestBinnedIntegationKernel(t *testing.T) {
	t.Run(
		"test that the binned integration kernel runs",
		func(t *testing.T) {
			kernel := &BinnedIntegrationKernel{}
			params := simulator.NewParams(map[string][]float64{
				"bin_values":   {1.0, 2.0, 3.0},
				"bin_stepsize": {1.0},
			})
			kernel.Configure(0, &simulator.Settings{
				Iterations: []simulator.IterationSettings{
					{Name: "test", Params: params},
				},
			})
			kernel.SetParams(&params)
			valueOne := kernel.Evaluate(
				[]float64{0.3, 1.0, 0.0},
				[]float64{0.5, 1.1, 1.0},
				1.1,
				0.0,
			)
			params.Set("bin_values", []float64{4.0, 5.0, 6.0})
			params.SetIndex("bin_stepsize", 0, 2.0)
			kernel.SetParams(&params)
			valueTwo := kernel.Evaluate(
				[]float64{0.3, 1.0, 0.0},
				[]float64{0.5, 1.1, 1.0},
				3.2,
				1.4,
			)
			if floats.HasNaN([]float64{valueOne, valueTwo}) {
				t.Errorf("NaN present in values: %f, %f", valueOne, valueTwo)
			}
		},
	)
}
