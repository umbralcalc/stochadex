package simulator

import (
	"gonum.org/v1/gonum/mat"
)

type PartitionManager struct {
	broadcastingChannels [](chan *IteratorOutputMessage)
	newWorkChannels      [](chan *IteratorInputMessage)
	pendingStateUpdates  []*State
	iterators            []*StateIterator
	stateHistories       []*StateHistory
	partitionTimesteps   []int
	overallTimesteps     int
	numberOfPartitions   int
	timestepsHistory     *TimestepsHistory
	timestepFunction     TimestepFunction
	terminationCondition TerminationCondition
	outputCondition      OutputCondition
	outputFunction       OutputFunction
}

func (m *PartitionManager) UpdateHistory(partitionIndex int, state *State) {
	// reference this partition
	partition := m.stateHistories[partitionIndex]
	// iterate over the history (matrix columns) and shift them
	// back one timestep
	for i := 1; i < partition.StateHistoryDepth; i++ {
		partition.Values.SetRow(i, partition.Values.RawRowView(i-1))
	}
	// update the latest state in the history
	partition.Values.SetRow(0, state.Values.RawVector().Data)
	// update the count of how many steps this partition has received
	m.partitionTimesteps[partitionIndex] += 1
}

func (m *PartitionManager) Receive(message *IteratorOutputMessage) {
	// add to the pending state updates for whichever partition message arrives
	m.pendingStateUpdates[message.PartitionIndex] = message.State
	// iterate the count of updates for that partition
	m.partitionTimesteps[message.PartitionIndex] += 1
	// if we are yet to receive any of the latest partition messages for
	// this step, then do nothing
	for _, timesteps := range m.partitionTimesteps {
		if timesteps != m.overallTimesteps {
			return
		}
	}
	// otherwise, update the overall step count and implement the pending state updates
	// to the history
	m.overallTimesteps += 1
	m.timestepsHistory = m.timestepFunction.Iterate(m.timestepsHistory)
	for partitionIndex, state := range m.pendingStateUpdates {
		m.UpdateHistory(partitionIndex, state)
	}
	// also apply the output function if this step requires it
	if m.outputCondition.IsOutputStep(
		m.stateHistories,
		m.timestepsHistory,
		m.overallTimesteps,
	) {
		m.outputFunction.Output(m.stateHistories, m.timestepsHistory, m.overallTimesteps)
	}
	// terminate without launching another thread if the condition has been met
	if m.terminationCondition.Terminate(
		m.stateHistories,
		m.timestepsHistory,
		m.overallTimesteps,
	) {
		return
	}
	m.RequestMoreIterations()
}

func (m *PartitionManager) RequestMoreIterations() {
	// send messages on the new work channels to ask for the next iteration
	// in the case of each partition
	for index := 0; index < m.numberOfPartitions; index++ {
		m.newWorkChannels[index] <- &IteratorInputMessage{
			StateHistories:   m.stateHistories,
			TimestepsHistory: m.timestepsHistory,
		}
	}
	// setup iterators to receive an broadcast their iterations
	for index := 0; index < m.numberOfPartitions; index++ {
		m.iterators[index].ReceiveIterateAndBroadcast(
			<-m.newWorkChannels[index],
			m.broadcastingChannels,
		)
	}
	// setup channels to receive the messages from each job
	for _, channel := range m.broadcastingChannels {
		m.Receive(<-channel)
	}
}

func (m *PartitionManager) Run() {
	// begin by requesting iterations for the next step
	m.RequestMoreIterations()
}

func NewPartitionManager(config *StochadexConfig) *PartitionManager {
	timestepsHistory := &TimestepsHistory{
		Values:            mat.NewVecDense(config.Steps.TimestepsHistoryDepth, nil),
		StateHistoryDepth: config.Steps.TimestepsHistoryDepth,
	}
	broadcastingChannels := make([](chan *IteratorOutputMessage), 0)
	iterators := make([]*StateIterator, 0)
	stateHistories := make([]*StateHistory, 0)
	partitionTimesteps := make([]int, 0)
	for index, stateConfig := range config.Partitions {
		partitionTimesteps = append(partitionTimesteps, 0)
		stateHistoryValues := mat.NewDense(
			stateConfig.HistoryDepth,
			stateConfig.Width,
			nil,
		)
		for elementIndex, element := range stateConfig.Params.InitStateValues {
			stateHistoryValues.Set(0, elementIndex, element)
		}
		stateHistories = append(
			stateHistories,
			&StateHistory{
				Values:            stateHistoryValues,
				StateWidth:        stateConfig.Width,
				StateHistoryDepth: stateConfig.HistoryDepth,
			},
		)
		iterators = append(
			iterators,
			&StateIterator{
				partitionIndex: index,
				params:         stateConfig.Params,
				iteration:      *stateConfig.Iteration,
			},
		)
		broadcastingChannels = append(
			broadcastingChannels,
			make(chan *IteratorOutputMessage),
		)
	}
	return &PartitionManager{
		broadcastingChannels: broadcastingChannels,
		iterators:            iterators,
		stateHistories:       stateHistories,
		partitionTimesteps:   partitionTimesteps,
		overallTimesteps:     0,
		numberOfPartitions:   len(partitionTimesteps),
		timestepsHistory:     timestepsHistory,
		timestepFunction:     *config.Steps.TimestepFunction,
		terminationCondition: *config.Steps.TerminationCondition,
		outputCondition:      *config.Output.Condition,
		outputFunction:       *config.Output.Function,
	}
}
