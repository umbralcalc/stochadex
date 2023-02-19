package simulator

import "testing"

func iteratePartition(m *PartitionManager, partitionIndex int) *State {
	// iterate this partition by one step within the same thread
	return m.iterators[partitionIndex].Iterate(
		m.stateHistories,
		m.timestepsHistory,
	)
}

func iterateHistory(m *PartitionManager) {
	// update the state history for each job in turn within the same thread
	for index := 0; index < m.numberOfPartitions; index++ {
		m.UpdateHistory(index, iteratePartition(m, index))
		m.partitionTimesteps[index] += 1
	}
	m.overallTimesteps += 1
	m.timestepsHistory = m.timestepFunction.Iterate(m.timestepsHistory)
}

func run(m *PartitionManager) {
	// terminate without iterating again if the condition has not been met
	for !m.terminationCondition.Terminate(
		m.stateHistories,
		m.timestepsHistory,
		m.overallTimesteps,
	) {
		iterateHistory(m)
	}
}

func TestPartitionManager(t *testing.T) {

}
