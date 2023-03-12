package simulator

import (
	"sync"

	"gonum.org/v1/gonum/mat"
)

// Coordinator
type PartitionCoordinator struct {
	receivingChannel     chan *IteratorOutputMessage
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
}

func (c *PartitionCoordinator) UpdateHistory(partitionIndex int, state *State) {
	// reference this partition
	partition := c.stateHistories[partitionIndex]
	// iterate over the history (matrix columns) and shift them
	// back one timestep
	for i := 1; i < partition.StateHistoryDepth; i++ {
		partition.Values.SetRow(i, partition.Values.RawRowView(i-1))
	}
	// update the latest state in the history
	partition.Values.SetRow(0, state.Values.RawVector().Data)
	// update the count of how many steps this partition has received
	c.partitionTimesteps[partitionIndex] += 1
}

func (c *PartitionCoordinator) Receive(inputChannel <-chan *IteratorOutputMessage) {
	message := <-inputChannel
	// add to the pending state updates for whichever partition message arrives
	c.pendingStateUpdates[message.PartitionIndex] = message.State
	// iterate the count of updates for that partition
	c.partitionTimesteps[message.PartitionIndex] += 1
}

func (c *PartitionCoordinator) RequestMoreIterations(wg *sync.WaitGroup) {
	// update the overall step count
	c.overallTimesteps += 1
	// setup iterators to receive and send their iteration results
	for index := 0; index < c.numberOfPartitions; index++ {
		wg.Add(1)
		i := index
		go func() {
			defer wg.Done()
			c.iterators[i].ReceiveIterateAndSend(
				c.newWorkChannels[i],
				c.receivingChannel,
			)
		}()
	}
	// send messages on the new work channels to ask for the next iteration
	// in the case of each partition
	for index := 0; index < c.numberOfPartitions; index++ {
		c.newWorkChannels[index] <- &IteratorInputMessage{
			StateHistories:   c.stateHistories,
			TimestepsHistory: c.timestepsHistory,
		}
	}
	// setup to receive the messages from each job
	for index := 0; index < c.numberOfPartitions; index++ {
		c.Receive(c.receivingChannel)
	}
}

func (c *PartitionCoordinator) Run() {
	var wg sync.WaitGroup

	// terminate the for loop if the condition has been met
	for !c.terminationCondition.Terminate(
		c.stateHistories,
		c.timestepsHistory,
		c.overallTimesteps,
	) {
		// begin by requesting iterations for the next step and waiting
		c.RequestMoreIterations(&wg)
		wg.Wait()

		// otherwise, implement the pending state updates to the history
		c.timestepsHistory = c.timestepFunction.Iterate(c.timestepsHistory)
		for partitionIndex, state := range c.pendingStateUpdates {
			c.UpdateHistory(partitionIndex, state)
		}
	}
}

func NewPartitionCoordinator(config *StochadexConfig) *PartitionCoordinator {
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
				partitionIndex:  index,
				params:          stateConfig.Params,
				iteration:       *stateConfig.Iteration,
				outputCondition: *config.Output.Condition,
				outputFunction:  *config.Output.Function,
			},
		)
		newWorkChannels = append(
			newWorkChannels,
			make(chan *IteratorInputMessage),
		)
	}
	return &PartitionCoordinator{
		receivingChannel:     make(chan *IteratorOutputMessage),
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
	}
}
