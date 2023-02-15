package stochadex

type PartitionManager struct {
	stateHistory *StateHistory
	iterator     StateIterator
}

func (m *PartitionManager) UpdateHistory(state *State) {
	// update the latest state in the history
	m.stateHistory.Values.SetRow(0, state.Values.RawVector().Data)
	// iterate over the history (matrix columns) and shift them
	// back one timestep
	for i := 0; i < m.stateHistory.StateHistoryDepth-1; i++ {
		m.stateHistory.Values.SetRow(i+1, m.stateHistory.Values.RawRowView(i))
	}
}

func (m *PartitionManager) Receive(state *State) {
	m.UpdateHistory(state)
}

func (m *PartitionManager) LaunchThread() {

}
