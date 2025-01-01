package simulator

// TerminationCondition is the interface that must be implemented in
// order to create a new condition for ending the simulation.
type TerminationCondition interface {
	Terminate(
		stateHistories []*StateHistory,
		timestepsHistory *CumulativeTimestepsHistory,
	) bool
}

// NumberOfStepsTerminationCondition terminates the process when the
// overall number of steps performed has reached MaxNumberOfSteps.
type NumberOfStepsTerminationCondition struct {
	MaxNumberOfSteps int
}

func (t *NumberOfStepsTerminationCondition) Terminate(
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) bool {
	return timestepsHistory.CurrentStepNumber >= t.MaxNumberOfSteps
}

// TimeElapsedTerminationCondition terminates the process when the
// overall time elapsed has reached MaxTimeElapsed.
type TimeElapsedTerminationCondition struct {
	MaxTimeElapsed float64
}

func (t *TimeElapsedTerminationCondition) Terminate(
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) bool {
	return timestepsHistory.Values.AtVec(0) >= t.MaxTimeElapsed
}
