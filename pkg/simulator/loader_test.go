package simulator

import (
	"testing"
)

// dummyProcessIteration defines an iteration which is only for
// testing - the process simply sets the values to be their
// element indices at each timestep.
type dummyProcessIteration struct{}

func (d *dummyProcessIteration) Configure(
	partitionIndex int,
	settings *LoadSettingsConfig,
) {
}

func (d *dummyProcessIteration) Iterate(
	otherParams *OtherParams,
	partitionIndex int,
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := 0; i < stateHistory.StateWidth; i++ {
		values[i] = float64(i)
	}
	return values
}

func TestNewStochadexConfig(t *testing.T) {
	t.Run(
		"test config is initialised properly",
		func(t *testing.T) {
			settings := NewLoadSettingsConfigFromYaml("test_config.yaml")
			iterations := make([]Iteration, 0)
			for range settings.StateWidths {
				iterations = append(
					iterations,
					&dummyProcessIteration{},
				)
			}
			implementations := &LoadImplementationsConfig{
				Iterations:      iterations,
				OutputCondition: &EveryStepOutputCondition{},
				OutputFunction:  &NilOutputFunction{},
				TerminationCondition: &NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 1000,
				},
				TimestepFunction: &ConstantTimestepFunction{Stepsize: 1.0},
			}
			_ = NewStochadexConfig(settings, implementations)
		},
	)
}
