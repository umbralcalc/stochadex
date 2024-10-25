package kernels

import (
	"fmt"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

func TestExponentialIntegationKernel(t *testing.T) {
	t.Run(
		"test that the exponential integration kernel runs",
		func(t *testing.T) {
			kernel := &ExponentialIntegrationKernel{}
			params := simulator.NewParams(map[string][]float64{
				"exponential_weighting_timescale": {1.0},
			})
			kernel.Configure(0, &simulator.Settings{
				Params: []simulator.Params{params},
			})
			valueOne := kernel.Evaluate(
				[]float64{0.3, 1.0, 0.0},
				[]float64{0.5, 1.1, 1.0},
				1.0,
				0.0,
			)
			params.SetIndex("exponential_weighting_timescale", 0, 2.0)
			kernel.SetParams(&params)
			valueTwo := kernel.Evaluate(
				[]float64{0.3, 1.0, 0.0},
				[]float64{0.5, 1.1, 1.0},
				1.0,
				0.0,
			)
			if floats.HasNaN([]float64{valueOne, valueTwo}) {
				t.Errorf(fmt.Sprintf("NaN present in values: %f, %f", valueOne, valueTwo))
			}
		},
	)
}
