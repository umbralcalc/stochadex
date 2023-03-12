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

// StateIterator uses an implemented Iteration interface on a given state
// partition, the latter of which is referenced by an index.
type StateIterator struct {
	partitionIndex  int
	timesteps       int
	params          *ParamsConfig
	iteration       Iteration
	outputCondition OutputCondition
	outputFunction  OutputFunction
}

// Iterate takes the state and timesteps history and outputs an updated
// State struct using an implemented Iteration interface.
func (s *StateIterator) Iterate(
	stateHistories []*StateHistory,
	timestepsHistory *TimestepsHistory,
) *State {
	newState := s.iteration.Iterate(
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

// ReceiveIterateAndSend listens for input messages sent to the input
// channel, runs Iterate when an IteratorInputMessage has been received on the
// provided inputChannel then sends the resulting IteratorOutputMessage on
// the provided outputChannel.
func (s *StateIterator) ReceiveIterateAndSend(
	inputChannel <-chan *IteratorInputMessage,
	outputChannel chan<- *IteratorOutputMessage,
) {
	inputMessage := <-inputChannel
	outputChannel <- &IteratorOutputMessage{
		PartitionIndex: s.partitionIndex,
		State: s.Iterate(
			inputMessage.StateHistories,
			inputMessage.TimestepsHistory,
		),
	}
}
