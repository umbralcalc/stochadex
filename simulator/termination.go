package simulator

type TerminationCondition interface {
	Terminate(
		stateHistories []*StateHistory,
		timestepsHistory *TimestepsHistory,
		overallTimesteps int,
	) bool
}

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
