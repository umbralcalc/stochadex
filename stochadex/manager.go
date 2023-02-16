package stochadex

type PartitionManager struct {
	broadcastingChannels [](chan *IteratorOutputMessage)
	iterators            []*StateIterator
	stateHistories       []*StateHistory
	partitionTimesteps   []int
	overallTimesteps     int
	numberOfPartitions   int
	timestepsHistory     *TimestepsHistory
	timestepFunction     TimestepFunction
}

func (m *PartitionManager) UpdateHistory(partitionIndex int, state *State) {
	// reference this partition
	partition := m.stateHistories[partitionIndex]
	// update the latest state in the history
	partition.Values.SetRow(0, state.Values.RawVector().Data)
	// iterate over the history (matrix columns) and shift them
	// back one timestep
	for i := 0; i < partition.StateHistoryDepth-1; i++ {
		partition.Values.SetRow(i+1, partition.Values.RawRowView(i))
	}
	// update the count of how many steps this partition has received
	m.partitionTimesteps[partitionIndex] += 1
}

func (m *PartitionManager) Receive(message *IteratorOutputMessage) {
	// update the state history for whichever partition message arrives
	m.UpdateHistory(message.PartitionIndex, message.State)
	// iterate the count of updates for that partition
	m.partitionTimesteps[message.PartitionIndex] += 1
	// if we are yet to receive any of the latest partition messages for
	// this step, then do nothing
	for _, timesteps := range m.partitionTimesteps {
		if timesteps != m.overallTimesteps {
			return
		}
	}
	// otherwise, update the overall step count and launch new jobs to
	// to update for the next step
	m.overallTimesteps += 1
	m.timestepsHistory = m.timestepFunction.Iterate(m.timestepsHistory)
	for index := 0; index < m.numberOfPartitions; index++ {
		m.LaunchThread(index)
	}
}

func (m *PartitionManager) LaunchThread(partitionIndex int) {
	// instantiate a goroutine to iterate this partition by one step
	go m.iterators[partitionIndex].IterateAndBroadcast(
		m.stateHistories,
		m.timestepsHistory,
		m.broadcastingChannels,
	)
}

func (m *PartitionManager) Run() {
	// update the overall step count and launch new jobs to
	// to update for the next step
	m.overallTimesteps += 1
	m.timestepsHistory = m.timestepFunction.Iterate(m.timestepsHistory)
	for index := 0; index < m.numberOfPartitions; index++ {
		m.LaunchThread(index)
	}
	// setup channels to receive the messages from each job
	for _, channel := range m.broadcastingChannels {
		m.Receive(<-channel)
	}
}
