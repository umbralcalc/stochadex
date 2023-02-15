package stochadex

type PartitionManager struct {
	PartitionName           PartitionName
	broadcastingChannels    [](chan *IteratorOutputMessage)
	stateHistoryByPartition map[PartitionName]*StateHistory
	updatesByPartition      map[PartitionName]int
	iterator                StateIterator
	updatesCount            int
}

func (m *PartitionManager) UpdateHistory(partitionName PartitionName, state *State) {
	// reference this partition
	partition := m.stateHistoryByPartition[partitionName]
	// update the latest state in the history
	partition.Values.SetRow(0, state.Values.RawVector().Data)
	// iterate over the history (matrix columns) and shift them
	// back one timestep
	for i := 0; i < partition.StateHistoryDepth-1; i++ {
		partition.Values.SetRow(i+1, partition.Values.RawRowView(i))
	}
	// update the count of how many updates this partition has received
	m.updatesByPartition[partitionName] += 1
}

func (m *PartitionManager) Receive(message *IteratorOutputMessage) {
	m.UpdateHistory(message.PartitionName, message.State)
	m.updatesByPartition[message.PartitionName] += 1
	for _, updates := range m.updatesByPartition {
		if updates != m.updatesCount {
			return
		}
	}
	m.LaunchThread()
}

func (m *PartitionManager) LaunchThread() {
	m.updatesCount += 1
	go m.iterator.IterateAndBroadcast(
		m.PartitionName,
		m.stateHistoryByPartition,
		m.broadcastingChannels,
	)
}

func (m *PartitionManager) Run() {
	m.LaunchThread()
	for _, channel := range m.broadcastingChannels {
		m.Receive(<-channel)
	}
}
