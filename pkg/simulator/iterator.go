package simulator

// Iteration is the interface that must be implemented for any stochastic
// phenomenon within the stochadex. It reads in a params struct, an index
// corresponding to which state partition is being iterated, a state history
// and a timesteps history and outputs an updated State struct.
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
	partitionIndex int
	params         *ParamsConfig
	iteration      Iteration
}

// Iterate takes the state and timesteps history and outputs an updated
// State struct using an implemented Iteration interface.
func (s *StateIterator) Iterate(
	stateHistories []*StateHistory,
	timestepsHistory *TimestepsHistory,
) *State {
	return s.iteration.Iterate(
		s.params.Other,
		s.partitionIndex,
		stateHistories,
		timestepsHistory,
	)
}

// ReceiveIterateAndBroadcast listens for input messages sent to the input
// channel, processes the iteration once one has been received and then
// broadcasts the resulting state update output back.
func (s *StateIterator) ReceiveIterateAndBroadcast(
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
