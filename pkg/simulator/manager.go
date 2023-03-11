package simulator

import (
	"sync"

	"gonum.org/v1/gonum/mat"
)

// PartitionManager
type PartitionManager struct {
	broadcastingChannel  chan *IteratorOutputMessage
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

func (m *PartitionManager) Receive(inputChannel <-chan *IteratorOutputMessage) {
	message := <-inputChannel
	// add to the pending state updates for whichever partition message arrives
	m.pendingStateUpdates[message.PartitionIndex] = message.State
	// iterate the count of updates for that partition
	m.partitionTimesteps[message.PartitionIndex] += 1
}

func (m *PartitionManager) RequestMoreIterations(wg *sync.WaitGroup) {
	// update the overall step count
	m.overallTimesteps += 1
	// setup iterators to receive an broadcast their iterations
	for index := 0; index < m.numberOfPartitions; index++ {
		wg.Add(1)
		i := index
		go func() {
			defer wg.Done()
			m.iterators[i].ReceiveIterateAndBroadcast(
				m.newWorkChannels[i],
				m.broadcastingChannel,
			)
		}()
	}
	// send messages on the new work channels to ask for the next iteration
	// in the case of each partition
	for index := 0; index < m.numberOfPartitions; index++ {
		m.newWorkChannels[index] <- &IteratorInputMessage{
			StateHistories:   m.stateHistories,
			TimestepsHistory: m.timestepsHistory,
		}
	}
	// setup to receive the messages from each job
	for index := 0; index < m.numberOfPartitions; index++ {
		m.Receive(m.broadcastingChannel)
	}
}

func (m *PartitionManager) Run() {
	var wg sync.WaitGroup

	// terminate the for loop if the condition has been met
	for !m.terminationCondition.Terminate(
		m.stateHistories,
		m.timestepsHistory,
		m.overallTimesteps,
	) {
		// begin by requesting iterations for the next step and waiting
		m.RequestMoreIterations(&wg)
		wg.Wait()

		// otherwise, implement the pending state updates to the history
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
			m.outputFunction.Output(
				m.stateHistories,
				m.timestepsHistory,
				m.overallTimesteps,
			)
		}
	}
}

func NewPartitionManager(config *StochadexConfig) *PartitionManager {
	timestepsHistory := &TimestepsHistory{
		Values:            mat.NewVecDense(config.Steps.TimestepsHistoryDepth, nil),
		StateHistoryDepth: config.Steps.TimestepsHistoryDepth,
	}
	iterators := make([]*StateIterator, 0)
	stateHistories := make([]*StateHistory, 0)
	partitionTimesteps := make([]int, 0)
	newWorkChannels := make([](chan *IteratorInputMessage), 0)
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
		newWorkChannels = append(
			newWorkChannels,
			make(chan *IteratorInputMessage),
		)
	}
	return &PartitionManager{
		broadcastingChannel:  make(chan *IteratorOutputMessage),
		newWorkChannels:      newWorkChannels,
		pendingStateUpdates:  make([]*State, len(config.Partitions)),
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
