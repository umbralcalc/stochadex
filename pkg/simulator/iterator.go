package simulator

import "gonum.org/v1/gonum/mat"

// Iteration is the interface that must be implemented for any stochastic
// phenomenon within the stochadex. Its .Iterate method reads in an OtherParams
// struct, a int partitionIndex, the full current history of the process defined
// by a slice []*StateHistory and a TimestepsHistory reference and outputs an
// updated state history row in the form of a float64 slice.
type Iteration interface {
	Configure(partitionIndex int, settings *Settings)
	Iterate(
		params *OtherParams,
		partitionIndex int,
		stateHistories []*StateHistory,
		timestepsHistory *CumulativeTimestepsHistory,
	) []float64
}

// ConstantValuesIteration leaves the values set by the initial conditions
// unchanged for all time.
type ConstantValuesIteration struct {
}

func (c *ConstantValuesIteration) Configure(
	partitionIndex int,
	settings *Settings,
) {
}

func (c *ConstantValuesIteration) Iterate(
	params *OtherParams,
	partitionIndex int,
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) []float64 {
	return stateHistories[partitionIndex].Values.RawRowView(0)
}

// StateIterator handles iterations of a given state partition on a
// separate goroutine and writing output data to somewhere.
type StateIterator struct {
	Iteration       Iteration
	Params          *OtherParams
	partitionIndex  int
	outputCondition OutputCondition
	outputFunction  OutputFunction
}

// Iterate takes the state and timesteps history and outputs an updated
// State struct using an implemented Iteration interface.
func (s *StateIterator) Iterate(
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) []float64 {
	newState := s.Iteration.Iterate(
		s.Params,
		s.partitionIndex,
		stateHistories,
		timestepsHistory,
	)
	// get the new time for output
	time := timestepsHistory.Values.AtVec(0) + timestepsHistory.NextIncrement
	// also apply the output function if this step requires it
	if s.outputCondition.IsOutputStep(s.partitionIndex, newState, time) {
		s.outputFunction.Output(s.partitionIndex, newState, time)
	}
	return newState
}

// ReceiveAndIteratePending listens for input messages sent to the input
// channel, runs Iterate when an IteratorInputMessage has been received on the
// provided inputChannel and then updates an internal pending state update object.
func (s *StateIterator) ReceiveAndIteratePending(
	inputChannel <-chan *IteratorInputMessage,
) {
	inputMessage := <-inputChannel
	inputMessage.StateHistories[s.partitionIndex].NextValues = s.Iterate(
		inputMessage.StateHistories,
		inputMessage.TimestepsHistory,
	)
}

// UpdateHistory should always follow a call to ReceiveAndIteratePending as it
// enacts the internal pending state update on the StateHistory object passed over
// the provided inputChannel.
func (s *StateIterator) UpdateHistory(inputChannel <-chan *IteratorInputMessage) {
	inputMessage := <-inputChannel
	// reference this partition
	partition := inputMessage.StateHistories[s.partitionIndex]
	// make a temporary copy of this partition's previous values
	var partitionValuesCopy mat.Dense
	partitionValuesCopy.CloneFrom(inputMessage.StateHistories[s.partitionIndex].Values)
	// iterate over the history (matrix columns) and shift them
	// back one timestep
	for i := 1; i < partition.StateHistoryDepth; i++ {
		partition.Values.SetRow(i, partitionValuesCopy.RawRowView(i-1))
	}
	// update the latest state in the history
	partition.Values.SetRow(0, partition.NextValues)
}
