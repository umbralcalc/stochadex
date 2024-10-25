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
			if time := timestepFunctionOne.NextIncrement(timestepsHistory); time != 1.0 {
				t.Error(fmt.Sprintf("constant timestep failed: %f", time))
			}
			timestepFunctionTwo := NewExponentialDistributionTimestepFunction(1.0, 1234)
			if time := timestepFunctionTwo.NextIncrement(timestepsHistory); time < 0.0 {
				t.Error(fmt.Sprintf("exponential timestep failed: %f", time))
			}
		},
	)
}
