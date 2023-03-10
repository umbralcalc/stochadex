package simulator

type Iteration interface {
	Iterate(
		params *OtherParams,
		partitionIndex int,
		stateHistories []*StateHistory,
		timestepsHistory *TimestepsHistory,
	) *State
}

type StateIterator struct {
	partitionIndex int
	params         *ParamsConfig
	iteration      Iteration
}

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
