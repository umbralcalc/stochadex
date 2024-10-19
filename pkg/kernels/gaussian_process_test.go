package kernels

import (
	"fmt"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

func TestGaussianProcessIntegationKernel(t *testing.T) {
	t.Run(
		"test that the Gaussian process integration kernel runs",
		func(t *testing.T) {
			kernel := &GaussianProcessIntegrationKernel{
				Covariance: &ConstantGaussianCovarianceKernel{},
			}
			kernel.Configure(0, &simulator.Settings{
				Params: []simulator.Params{
					simulator.NewParams(map[string][]float64{
						"target_state":             {0.7, 1.2, 1.8},
						"current_covariance_state": {0.7, 1.2, 1.8},
						"past_covariance_state":    {0.7, 1.2, 1.8},
						"covariance_matrix":        {1.0, 0.0, 0.0, 0.0, 1.0, 0.0, 0.0, 0.0, 1.0},
					}),
				},
			})
			valueOne := kernel.Evaluate(
				[]float64{0.3, 1.0, 0.0},
				[]float64{0.5, 1.1, 1.0},
				1.0,
				0.0,
			)
			kernel.SetParams(simulator.NewParams(map[string][]float64{
				"target_state":             {0.1, 0.6, 0.2},
				"current_covariance_state": {0.7, 1.2, 1.8},
				"past_covariance_state":    {0.7, 1.2, 1.8},
				"covariance_matrix":        {2.0, 0.0, 0.0, 0.0, 2.0, 0.0, 0.0, 0.0, 2.0},
			}))
			valueTwo := kernel.Evaluate(
				[]float64{0.3, 1.0, 0.0},
				[]float64{0.5, 1.1, 1.0},
				1.0,
				0.0,
			)
			if floats.HasNaN([]float64{valueOne, valueTwo}) {
				panic(fmt.Sprintf("NaN present in values: %f, %f", valueOne, valueTwo))
			}
		},
	)
}