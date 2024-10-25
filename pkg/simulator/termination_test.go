package simulator

import (
	"testing"

	"gonum.org/v1/gonum/mat"
)

func TestTerminationConditions(t *testing.T) {
	t.Run(
		"test the basic termination conditions run",
		func(t *testing.T) {
			stateHistories := []*StateHistory{
				{
					Values:            mat.NewDense(1, 3, []float64{0.0, 1.0, 2.0}),
					NextValues:        []float64{4.0, 5.0, 6.0},
					StateWidth:        3,
					StateHistoryDepth: 1,
				},
			}
			timestepsHistory := &CumulativeTimestepsHistory{
				NextIncrement:     5.0,
				Values:            mat.NewVecDense(1, []float64{2.0}),
				CurrentStepNumber: 2,
				StateHistoryDepth: 1,
			}
			terminationConditionOne := &NumberOfStepsTerminationCondition{MaxNumberOfSteps: 3}
			if c := terminationConditionOne.Terminate(stateHistories, timestepsHistory); !(c == false) {
				t.Error("number of steps termination condition failed: stops early")
			}
			timestepsHistory.CurrentStepNumber = 3
			if c := terminationConditionOne.Terminate(stateHistories, timestepsHistory); !(c == true) {
				t.Error("number of steps termination condition failed: doesn't stop")
			}
			terminationConditionTwo := &TimeElapsedTerminationCondition{MaxTimeElapsed: 3.0}
			if c := terminationConditionTwo.Terminate(stateHistories, timestepsHistory); !(c == false) {
				t.Error("time elapsed termination condition failed: stops early")
			}
			timestepsHistory.Values.SetVec(0, 3.0)
			if c := terminationConditionTwo.Terminate(stateHistories, timestepsHistory); !(c == true) {
				t.Error("time elapsed termination condition failed: doesn't stop")
			}
		},
	)
}
