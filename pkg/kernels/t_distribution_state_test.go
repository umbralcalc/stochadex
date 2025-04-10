package kernels

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

func TestTDistributionStateIntegrationKernel(t *testing.T) {
	t.Run(
		"test that the t-distribution state integration kernel runs",
		func(t *testing.T) {
			kernel := &TDistributionStateIntegrationKernel{}
			params := simulator.NewParams(map[string][]float64{
				"degrees_of_freedom": {5.2},
				"scale_matrix":       {1.0, 0.0, 0.0, 0.0, 1.0, 0.0, 0.0, 0.0, 1.0},
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
				1.0,
				0.0,
			)
			params.Set("degrees_of_freedom", []float64{3.4})
			params.Set("scale_matrix", []float64{2.0, 0.0, 0.0, 0.0, 2.0, 0.0, 0.0, 0.0, 2.0})
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
