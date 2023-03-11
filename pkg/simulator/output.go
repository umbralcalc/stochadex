package simulator

type OutputFunction interface {
	Output(partitionIndex int, state *State, timesteps int)
}

type NilOutputFunction struct{}

func (f *NilOutputFunction) Output(partitionIndex int, state *State, timesteps int) {
}

type OutputCondition interface {
	IsOutputStep(partitionIndex int, state *State, timesteps int) bool
}

type EveryStepOutputCondition struct{}

func (c *EveryStepOutputCondition) IsOutputStep(
	partitionIndex int,
	state *State,
	timesteps int,
) bool {
	return true
}

type EveryNStepsOutputCondition struct {
	N      int
	ticker int
}

func (c *EveryNStepsOutputCondition) IsOutputStep(
	partitionIndex int,
	state *State,
	timesteps int,
) bool {
	c.ticker += 1
	if c.ticker == c.N {
		c.ticker = 0
		return true
	}
	return false
}
