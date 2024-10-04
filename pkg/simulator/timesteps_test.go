package simulator

import (
	"fmt"
	"testing"

	"gonum.org/v1/gonum/mat"
)

func TestTimestepFunctions(t *testing.T) {
	t.Run(
		"test the basic timestep functions run",
		func(t *testing.T) {
			timestepsHistory := &CumulativeTimestepsHistory{
				NextIncrement:     5.0,
				Values:            mat.NewVecDense(1, []float64{2.0}),
				CurrentStepNumber: 2,
				StateHistoryDepth: 1,
			}
			timestepFunctionOne := &ConstantTimestepFunction{Stepsize: 1.0}
			if t := timestepFunctionOne.NextIncrement(timestepsHistory); t != 1.0 {
				panic(fmt.Sprintf("constant timestep failed: %f", t))
			}
			timestepFunctionTwo := NewExponentialDistributionTimestepFunction(1.0, 1234)
			if t := timestepFunctionTwo.NextIncrement(timestepsHistory); t < 0.0 {
				panic(fmt.Sprintf("exponential timestep failed: %f", t))
			}
		},
	)
}
