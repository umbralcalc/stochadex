package simulator

// TerminationCondition decides when the simulation should end.
type TerminationCondition interface {
	Terminate(
		stateHistories []*StateHistory,
		timestepsHistory *CumulativeTimestepsHistory,
	) bool
}

// NumberOfStepsTerminationCondition terminates after MaxNumberOfSteps.
type NumberOfStepsTerminationCondition struct {
	MaxNumberOfSteps int
}

func (t *NumberOfStepsTerminationCondition) Terminate(
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) bool {
	return timestepsHistory.CurrentStepNumber >= t.MaxNumberOfSteps
}

// TimeElapsedTerminationCondition terminates after MaxTimeElapsed.
type TimeElapsedTerminationCondition struct {
	MaxTimeElapsed float64
}

func (t *TimeElapsedTerminationCondition) Terminate(
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) bool {
	return timestepsHistory.Values.AtVec(0) >= t.MaxTimeElapsed
}
