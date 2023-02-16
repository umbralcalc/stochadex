package stochadex

type PartitionManager struct {
	broadcastingChannels    [](chan *IteratorOutputMessage)
	iteratorByPartition     map[PartitionName]*StateIterator
	stateHistoryByPartition map[PartitionName]*StateHistory
	timestepsByPartition    map[PartitionName]int
	timestepsHistory        *TimestepsHistory
	timestepFunction        TimestepFunction
	overallTimesteps        int
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
	// update the count of how many steps this partition has received
	m.timestepsByPartition[partitionName] += 1
}

func (m *PartitionManager) Receive(message *IteratorOutputMessage) {
	// update the state history for whichever partition message arrives
	m.UpdateHistory(message.PartitionName, message.State)
	// iterate the count of updates for that partition
	m.timestepsByPartition[message.PartitionName] += 1
	// if we are yet to receive any of the latest partition messages for
	// this step, then do nothing
	for _, updates := range m.timestepsByPartition {
		if updates != m.overallTimesteps {
			return
		}
	}
	// otherwise, update the overall step count and launch new jobs to
	// to update for the next step
	m.overallTimesteps += 1
	m.timestepsHistory = m.timestepFunction.Iterate(m.timestepsHistory)
	for partition := range m.timestepsByPartition {
		m.LaunchThread(partition)
	}
}

func (m *PartitionManager) LaunchThread(partitionName PartitionName) {
	// instantiate a goroutine to iterate this partition by one step
	go m.iteratorByPartition[partitionName].IterateAndBroadcast(
		m.stateHistoryByPartition,
		m.timestepsHistory,
		m.broadcastingChannels,
	)
}

func (m *PartitionManager) Run() {
	// update the overall step count and launch new jobs to
	// to update for the next step
	m.overallTimesteps += 1
	m.timestepsHistory = m.timestepFunction.Iterate(m.timestepsHistory)
	for partition := range m.timestepsByPartition {
		m.LaunchThread(partition)
	}
	// setup channels to receive the messages from each job
	for _, channel := range m.broadcastingChannels {
		m.Receive(<-channel)
	}
}
