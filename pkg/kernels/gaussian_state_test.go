package kernels

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

func TestGaussianStateIntegrationKernel(t *testing.T) {
	t.Run(
		"test that the Gaussian state integration kernel runs",
		func(t *testing.T) {
			kernel := &GaussianStateIntegrationKernel{}
			params := simulator.NewParams(map[string][]float64{
				"covariance_matrix": {1.0, 0.0, 0.0, 0.0, 1.0, 0.0, 0.0, 0.0, 1.0},
			})
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
			params.Set("covariance_matrix", []float64{2.0, 0.0, 0.0, 0.0, 2.0, 0.0, 0.0, 0.0, 2.0})
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
