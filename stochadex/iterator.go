package stochadex

type Iteration interface {
	Iterate(
		params *OtherParams,
		partition PartitionName,
		stateHistoryByPartition map[PartitionName]*StateHistory,
		timestepsHistory *TimestepsHistory,
	) *State
}

type IteratorOutputMessage struct {
	PartitionName PartitionName
	State         *State
}

type StateIterator struct {
	partitionName PartitionName
	params        *ParamsConfig
	iteration     Iteration
}

func (s *StateIterator) Iterate(
	stateHistoryByPartition map[PartitionName]*StateHistory,
	timestepsHistory *TimestepsHistory,
) *State {
	return s.iteration.Iterate(
		&s.params.Other,
		s.partitionName,
		stateHistoryByPartition,
		timestepsHistory,
	)
}

func (s *StateIterator) Broadcast(
	state *State,
	channels [](chan *IteratorOutputMessage),
) {
	for _, channel := range channels {
		channel <- &IteratorOutputMessage{PartitionName: s.partitionName, State: state}
	}
}

func (s *StateIterator) IterateAndBroadcast(
	stateHistoryByPartition map[PartitionName]*StateHistory,
	timestepsHistory *TimestepsHistory,
	channels [](chan *IteratorOutputMessage),
) {
	s.Broadcast(s.Iterate(stateHistoryByPartition, timestepsHistory), channels)
}
