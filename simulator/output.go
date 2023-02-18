package simulator

type OutputFunction interface {
	Output(
		stateHistories []*StateHistory,
		timestepsHistory *TimestepsHistory,
		overallTimesteps int,
	)
}

type OutputCondition interface {
	IsOutputStep(
		stateHistories []*StateHistory,
		timestepsHistory *TimestepsHistory,
		overallTimesteps int,
	) bool
}

type EveryStepOutputCondition struct{}

func (c *EveryStepOutputCondition) IsOutputStep(
	stateHistories []*StateHistory,
	timestepsHistory *TimestepsHistory,
	overallTimesteps int,
) bool {
	return true
}

type EveryNStepsOutputCondition struct {
	N      int
	ticker int
}

func (c *EveryNStepsOutputCondition) IsOutputStep(
	stateHistories []*StateHistory,
	timestepsHistory *TimestepsHistory,
	overallTimesteps int,
) bool {
	c.ticker += 1
	if c.ticker == c.N {
		c.ticker = 0
		return true
	}
	return false
}
