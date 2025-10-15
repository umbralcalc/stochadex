package continuous

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// GradientDescentIteration performs a gradient-based parameter update.
//
// Usage hints:
//   - Provide params: "gradient" (vector) and "learning_rate" (scalar).
//   - Optional flag: "ascent" == 1 switches to gradient ascent.
//   - Update uses x_{t+1} = x_t - lr * gradient (or + for ascent).
type GradientDescentIteration struct{}

func (g *GradientDescentIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (g *GradientDescentIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	gradient := params.Get("gradient")
	learningRate := params.Get("learning_rate")[0]
	if a, ok := params.GetOk("ascent"); ok {
		if a[0] == 1 {
			learningRate *= -1.0
		}
	}
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		values[i] = stateHistory.Values.At(0, i) - learningRate*gradient[i]
	}
	return values
}
