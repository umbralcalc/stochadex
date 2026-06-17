package simulator

import (
	"testing"

	"math/rand/v2"
)

// harnessSuccessTestIteration is a basic iteration for testing the harness.
type harnessSuccessTestIteration struct {
}

func (h *harnessSuccessTestIteration) Configure(
	partitionIndex int,
	settings *Settings,
) {
}

func (h *harnessSuccessTestIteration) Iterate(
	params *Params,
	partitionIndex int,
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	newValues := make([]float64, stateHistory.StateWidth)
	copy(newValues, stateHistory.Values.RawRowView(0))
	for i, value := range newValues {
		newValues[i] = value + 1
	}
	return newValues
}

// harnessFailTestIteration is a basic iteration for testing the harness.
type harnessFailTestIteration struct {
}

func (h *harnessFailTestIteration) Configure(
	partitionIndex int,
	settings *Settings,
) {
}

func (h *harnessFailTestIteration) Iterate(
	params *Params,
	partitionIndex int,
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	newValues := stateHistory.Values.RawRowView(0)
	for i, value := range newValues {
		newValues[i] = value + 1
	}
	return newValues
}

// harnessFailStatefulTestIteration is a basic iteration for testing the harness.
type harnessFailStatefulTestIteration struct {
}

func (h *harnessFailStatefulTestIteration) Configure(
	partitionIndex int,
	settings *Settings,
) {
}

func (h *harnessFailStatefulTestIteration) Iterate(
	params *Params,
	partitionIndex int,
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	newValues := make([]float64, stateHistory.StateWidth)
	copy(newValues, stateHistory.Values.RawRowView(0))
	for i, value := range newValues {
		newValues[i] = value + float64(rand.IntN(100))
	}
	return newValues
}

// harnessAliasTestIteration returns a live history row directly, without
// mutating it. The other harness checks (NaN, width, depth-retention) all pass
// for this — only the output-aliasing check rejects it. At StateHistoryDepth ==
// 1 this is the pathology that was previously invisible.
type harnessAliasTestIteration struct {
}

func (h *harnessAliasTestIteration) Configure(
	partitionIndex int,
	settings *Settings,
) {
}

func (h *harnessAliasTestIteration) Iterate(
	params *Params,
	partitionIndex int,
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) []float64 {
	return stateHistories[partitionIndex].Values.RawRowView(0)
}

func TestIterationHarness(t *testing.T) {
	settings := &Settings{
		Iterations: []IterationSettings{{
			Name:              "test",
			Params:            NewParams(make(map[string][]float64)),
			InitStateValues:   []float64{0.0, 1.0, 2.0, 3.0},
			Seed:              0,
			StateWidth:        4,
			StateHistoryDepth: 10,
		}},
		InitTimeValue:         0.0,
		TimestepsHistoryDepth: 10,
	}

	t.Run("test that the test harness runs successfully", func(t *testing.T) {
		implementations := &Implementations{
			Iterations:      []Iteration{&harnessSuccessTestIteration{}},
			OutputCondition: &EveryStepOutputCondition{},
			OutputFunction:  &NilOutputFunction{},
			TerminationCondition: &NumberOfStepsTerminationCondition{
				MaxNumberOfSteps: 100,
			},
			TimestepFunction: &ConstantTimestepFunction{Stepsize: 1.0},
		}
		err := RunWithHarnesses(settings, implementations)
		if err == nil {
			t.Log("Test harness succeeded as expected.")
		} else {
			t.Errorf("Expected the harness to succeed, but it failed.")
		}
	})

	t.Run("test that the harness catches output aliasing live history at depth 1",
		func(t *testing.T) {
			depthOneSettings := &Settings{
				Iterations: []IterationSettings{{
					Name:              "test",
					Params:            NewParams(make(map[string][]float64)),
					InitStateValues:   []float64{0.0, 1.0, 2.0, 3.0},
					Seed:              0,
					StateWidth:        4,
					StateHistoryDepth: 1,
				}},
				InitTimeValue:         0.0,
				TimestepsHistoryDepth: 1,
			}
			implementations := &Implementations{
				Iterations:      []Iteration{&harnessAliasTestIteration{}},
				OutputCondition: &EveryStepOutputCondition{},
				OutputFunction:  &NilOutputFunction{},
				TerminationCondition: &NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				TimestepFunction: &ConstantTimestepFunction{Stepsize: 1.0},
			}
			err := RunWithHarnesses(depthOneSettings, implementations)
			if err != nil {
				t.Log("Test harness failed as expected.")
			} else {
				t.Errorf("Expected the harness to fail on output aliasing, but" +
					" it succeeded.")
			}
		},
	)

	t.Run("test that the test harness run fails as expected", func(t *testing.T) {
		implementations := &Implementations{
			Iterations:      []Iteration{&harnessFailTestIteration{}},
			OutputCondition: &EveryStepOutputCondition{},
			OutputFunction:  &NilOutputFunction{},
			TerminationCondition: &NumberOfStepsTerminationCondition{
				MaxNumberOfSteps: 100,
			},
			TimestepFunction: &ConstantTimestepFunction{Stepsize: 1.0},
		}
		err := RunWithHarnesses(settings, implementations)
		if err != nil {
			t.Log("Test harness failed as expected.")
		} else {
			t.Errorf("Expected the harness to fail, but it succeeded.")
		}
	})

	t.Run("test that the test harness run fails as expected due to a statefulness residue",
		func(t *testing.T) {
			implementations := &Implementations{
				Iterations:      []Iteration{&harnessFailStatefulTestIteration{}},
				OutputCondition: &EveryStepOutputCondition{},
				OutputFunction:  &NilOutputFunction{},
				TerminationCondition: &NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				TimestepFunction: &ConstantTimestepFunction{Stepsize: 1.0},
			}
			err := RunWithHarnesses(settings, implementations)
			if err != nil {
				t.Log("Test harness failed as expected.")
			} else {
				t.Errorf("Expected the harness to fail, but it succeeded.")
			}
		},
	)
}
