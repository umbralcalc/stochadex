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
	return s.iteration.Iterate(s.params.Other, s.partitionIndex, stateHistories, timestepsHistory)
}

func (s *StateIterator) Broadcast(
	state *State,
	channels [](chan *IteratorOutputMessage),
) {
	for _, channel := range channels {
		channel <- &IteratorOutputMessage{PartitionIndex: s.partitionIndex, State: state}
	}
}

func (s *StateIterator) IterateAndBroadcast(
	stateHistories []*StateHistory,
	timestepsHistory *TimestepsHistory,
	channels [](chan *IteratorOutputMessage),
) {
	s.Broadcast(s.Iterate(stateHistories, timestepsHistory), channels)
}
