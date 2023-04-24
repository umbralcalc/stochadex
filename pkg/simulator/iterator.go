package simulator

// Iteration is the interface that must be implemented for any stochastic
// phenomenon within the stochadex. It reads in an OtherParams struct, a int
// partitionIndex, the full current history of the process defined by a slice
// []*StateHistory and a TimestepsHistory reference and outputs an updated
// State struct.
type Iteration interface {
	Iterate(
		params *OtherParams,
		partitionIndex int,
		stateHistories []*StateHistory,
		timestepsHistory *TimestepsHistory,
	) *State
}

// StateIterator handles iterations of a given state partition on a
// separate goroutine and writing output data to disk or some DB.
type StateIterator struct {
	Iteration          Iteration
	partitionIndex     int
	timesteps          int
	params             *ParamsConfig
	outputCondition    OutputCondition
	outputFunction     OutputFunction
	pendingStateUpdate *State
}

// Iterate takes the state and timesteps history and outputs an updated
// State struct using an implemented Iteration interface.
func (s *StateIterator) Iterate(
	stateHistories []*StateHistory,
	timestepsHistory *TimestepsHistory,
) *State {
	newState := s.Iteration.Iterate(
		s.params.Other,
		s.partitionIndex,
		stateHistories,
		timestepsHistory,
	)
	// also apply the output function if this step requires it
	if s.outputCondition.IsOutputStep(s.partitionIndex, newState, s.timesteps) {
		s.outputFunction.Output(s.partitionIndex, newState, s.timesteps)
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
	s.pendingStateUpdate = s.Iterate(
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
	// iterate over the history (matrix columns) and shift them
	// back one timestep
	for i := 1; i < partition.StateHistoryDepth; i++ {
		partition.Values.SetRow(i, partition.Values.RawRowView(i-1))
	}
	// update the latest state in the history
	partition.Values.SetRow(0, s.pendingStateUpdate.Values.RawVector().Data)
}
