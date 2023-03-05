package simulator

import (
	"testing"

	"gonum.org/v1/gonum/mat"
)

type DummyProcessIteration struct{}

func (d *DummyProcessIteration) Iterate(
	otherParams *OtherParams,
	partitionIndex int,
	stateHistories []*StateHistory,
	timestepsHistory *TimestepsHistory,
) *State {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := 0; i < stateHistory.StateWidth; i++ {
		values[i] = float64(i)
	}
	return &State{
		Values: mat.NewVecDense(
			stateHistory.StateWidth,
			values,
		),
		StateWidth: stateHistory.StateWidth,
	}
}

func TestNewStochadexConfig(t *testing.T) {
	t.Run(
		"test config is initialised properly",
		func(t *testing.T) {
			settings := NewLoadSettingsConfigFromYaml("res/test_config.yaml")
			iterations := make([]Iteration, 0)
			for range settings.StateWidths {
				iterations = append(
					iterations,
					&DummyProcessIteration{},
				)
			}
			implementations := &LoadImplementationsConfig{
				Iterations:      iterations,
				OutputCondition: &EveryStepOutputCondition{},
				OutputFunction:  &NilOutputFunction{},
				TerminationCondition: &NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 1000,
				},
				TimestepFunction: &ConstantNoMemoryTimestepFunction{
					Stepsize: 1.0,
				},
			}
			_ = NewStochadexConfig(settings, implementations)
		},
	)
}
