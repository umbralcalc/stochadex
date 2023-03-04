package simulator

import "gonum.org/v1/gonum/mat"

type PartitionManager struct {
	broadcastingChannels [](chan *IteratorOutputMessage)
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
	for i := 0; i < partition.StateHistoryDepth-1; i++ {
		partition.Values.SetRow(i+1, partition.Values.RawRowView(i))
	}
	// update the latest state in the history
	partition.Values.SetRow(0, state.Values.RawVector().Data)
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
	// apply the output function if this step requires it
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
	// otherwise, launch more threads for the next step
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
	// launch new jobs to update for the next step
	for index := 0; index < m.numberOfPartitions; index++ {
		m.LaunchThread(index)
	}
	// setup channels to receive the messages from each job
	for _, channel := range m.broadcastingChannels {
		m.Receive(<-channel)
	}
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
		stateHistories = append(
			stateHistories,
			&StateHistory{
				Values: mat.NewDense(
					stateConfig.HistoryDepth,
					stateConfig.Width,
					stateConfig.Params.InitStateValues,
				),
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
