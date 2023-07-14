package simulator

// OutputFunction is the interface that must be implemented for any function
// which can be used to outputs data from the stochastic process when the provided
// OutputCondition is met.
type OutputFunction interface {
	Output(partitionIndex int, state []float64, cumulativeTimesteps float64)
}

// NilOutputFunction outputs nothing from the stochastic process.
type NilOutputFunction struct{}

func (f *NilOutputFunction) Output(
	partitionIndex int,
	state []float64,
	cumulativeTimesteps float64,
) {
}

// VariableStoreOutputFunction stores the data from the stochastic process in a provided
// Store variable on the steps when the OutputCondition is met
type VariableStoreOutputFunction struct {
	Store [][][]float64
}

func (f *VariableStoreOutputFunction) Output(
	partitionIndex int,
	state []float64,
	cumulativeTimesteps float64,
) {
	f.Store[partitionIndex] = append(f.Store[partitionIndex], state)
}

// OutputCondition is the interface that must be implemented to define when the
// stochastic process calls the OutputFunction.
type OutputCondition interface {
	IsOutputStep(partitionIndex int, state []float64, cumulativeTimesteps float64) bool
}

// NilOutputCondition never outputs.
type NilOutputCondition struct{}

func (c *NilOutputCondition) IsOutputStep(
	partitionIndex int,
	state []float64,
	cumulativeTimesteps float64,
) bool {
	return false
}

// EveryStepOutputCondition calls the OutputFunction at every step.
type EveryStepOutputCondition struct{}

func (c *EveryStepOutputCondition) IsOutputStep(
	partitionIndex int,
	state []float64,
	cumulativeTimesteps float64,
) bool {
	return true
}

// EveryStepOutputCondition calls the OutputFunction once for every N
// steps that occur.
type EveryNStepsOutputCondition struct {
	N      int
	ticker int
}

func (c *EveryNStepsOutputCondition) IsOutputStep(
	partitionIndex int,
	state []float64,
	cumulativeTimesteps float64,
) bool {
	c.ticker += 1
	if c.ticker == c.N {
		c.ticker = 0
		return true
	}
	return false
}
