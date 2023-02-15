package stochadex

type Iteration interface {
	Iterate(
		partition PartitionName,
		stateHistoryByPartition map[PartitionName]*StateHistory,
	) *State
}

type IteratorOutputMessage struct {
	PartitionName PartitionName
	State         *State
}

type StateIterator struct {
	iteration Iteration
}

func (s *StateIterator) Iterate(
	partition PartitionName,
	stateHistoryByPartition map[PartitionName]*StateHistory,
) *State {
	return s.iteration.Iterate(partition, stateHistoryByPartition)
}

func (s *StateIterator) Broadcast(
	partition PartitionName,
	state *State,
	channels [](chan *IteratorOutputMessage),
) {
	for _, channel := range channels {
		channel <- &IteratorOutputMessage{PartitionName: partition, State: state}
	}
}

func (s *StateIterator) IterateAndBroadcast(
	partition PartitionName,
	stateHistoryByPartition map[PartitionName]*StateHistory,
	channels [](chan *IteratorOutputMessage),
) {
	s.Broadcast(partition, s.Iterate(partition, stateHistoryByPartition), channels)
}
