package simulator

// TerminationCondition is the interface that must be implemented in
// order to create a new condition for ending the stochastic process.
type TerminationCondition interface {
	Terminate(
		stateHistories []*StateHistory,
		timestepsHistory *TimestepsHistory,
		overallTimesteps int,
	) bool
}

// NumberOfStepsTerminationCondition terminates the process when the
// overall number of steps performed has reached MaxNumberOfSteps.
type NumberOfStepsTerminationCondition struct {
	MaxNumberOfSteps int
}

func (t *NumberOfStepsTerminationCondition) Terminate(
	stateHistories []*StateHistory,
	timestepsHistory *TimestepsHistory,
	overallTimesteps int,
) bool {
	if overallTimesteps >= t.MaxNumberOfSteps {
		return true
	}
	return false
}

// TimeElapsedTerminationCondition terminates the process when the
// overall time elapsed has reached MaxTimeElapsed.
type TimeElapsedTerminationCondition struct {
	MaxTimeElapsed float64
}

func (t *TimeElapsedTerminationCondition) Terminate(
	stateHistories []*StateHistory,
	timestepsHistory *TimestepsHistory,
	overallTimesteps int,
) bool {
	if timestepsHistory.Values.AtVec(0) >= t.MaxTimeElapsed {
		return true
	}
	return false
}
