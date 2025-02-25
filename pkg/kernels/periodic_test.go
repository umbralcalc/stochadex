package kernels

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

func TestPeriodicIntegationKernel(t *testing.T) {
	t.Run(
		"test that the periodic integration kernel runs",
		func(t *testing.T) {
			kernel := &PeriodicIntegrationKernel{}
			params := simulator.NewParams(map[string][]float64{
				"periodic_weighting_timescale": {1.0},
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
			params.SetIndex("periodic_weighting_timescale", 0, 2.0)
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
