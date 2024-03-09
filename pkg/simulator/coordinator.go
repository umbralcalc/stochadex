package simulator

import (
	"sync"

	"gonum.org/v1/gonum/mat"
)

// PartitionCoordinator coordinates the assignment of iteration work to
// separate StateIterator objects on separate goroutines and when to enact
// these updates on the state history.
type PartitionCoordinator struct {
	Iterators            [][]*StateIterator
	StateHistories       []*StateHistory
	TimestepsHistory     *CumulativeTimestepsHistory
	TimestepFunction     TimestepFunction
	TerminationCondition TerminationCondition
	PartitionIndices     [][]int
	newWorkChannels      [](chan *IteratorInputMessage)
}

// RequestMoreIterations spawns a goroutine for each state partition to
// carry out a ReceiveAndIteratePending job.
func (c *PartitionCoordinator) RequestMoreIterations(wg *sync.WaitGroup) {
	// setup iterators to receive and send their iteration results
	for parallelIndex, serialPartitions := range c.PartitionIndices {
		wg.Add(1)
		i := parallelIndex
		parts := serialPartitions
		go func() {
			defer wg.Done()
			for serialIndex, iterator := range c.Iterators[i] {
				partitionIndex := parts[serialIndex]
				iterator.ReceiveAndIteratePending(c.newWorkChannels[partitionIndex])
			}
		}()
	}
	// send messages on the new work channels to ask for the next iteration
	// in the case of each partition
	for _, channel := range c.newWorkChannels {
		channel <- &IteratorInputMessage{
			StateHistories:   c.StateHistories,
			TimestepsHistory: c.TimestepsHistory,
		}
	}
}

// RequestMoreIterations spawns a goroutine for each state partition to
// carry out an UpdateHistory job.
func (c *PartitionCoordinator) UpdateHistory(wg *sync.WaitGroup) {
	// setup iterators to receive and send their iteration results
	for parallelIndex, serialPartitions := range c.PartitionIndices {
		for serialIndex, index := range serialPartitions {
			wg.Add(1)
			i := index
			iterator := c.Iterators[parallelIndex][serialIndex]
			go func() {
				defer wg.Done()
				iterator.UpdateHistory(c.newWorkChannels[i])
			}()
		}
	}
	// send messages on the new work channels to ask for the next iteration
	// in the case of each partition
	for _, channel := range c.newWorkChannels {
		channel <- &IteratorInputMessage{
			StateHistories:   c.StateHistories,
			TimestepsHistory: c.TimestepsHistory,
		}
	}

	// iterate over the history of timesteps and shift them back one
	var timestepsHistoryValuesCopy mat.VecDense
	timestepsHistoryValuesCopy.CloneFromVec(c.TimestepsHistory.Values)
	for i := 1; i < c.TimestepsHistory.StateHistoryDepth; i++ {
		c.TimestepsHistory.Values.SetVec(i, timestepsHistoryValuesCopy.AtVec(i-1))
	}
	// now update the history with the next time increment
	c.TimestepsHistory.Values.SetVec(
		0,
		timestepsHistoryValuesCopy.AtVec(0)+c.TimestepsHistory.NextIncrement,
	)
}

// Step is the main method call of PartitionCoordinator - call this proceeding
// a new configuration of the latter to run the desired process for a single step.
func (c *PartitionCoordinator) Step(wg *sync.WaitGroup) {
	// update the overall step count and get the next time increment
	c.TimestepsHistory.CurrentStepNumber += 1
	c.TimestepsHistory = c.TimestepFunction.SetNextIncrement(c.TimestepsHistory)

	// begin by requesting iterations for the next step and waiting
	c.RequestMoreIterations(wg)
	wg.Wait()

	// then implement the pending state and time updates to the histories
	c.UpdateHistory(wg)
	wg.Wait()
}

// ReadyToTerminate returns whether or not the process has met the TerminationCondition.
func (c *PartitionCoordinator) ReadyToTerminate() bool {
	return c.TerminationCondition.Terminate(
		c.StateHistories,
		c.TimestepsHistory,
	)
}

// Run runs multiple Step calls up until the TerminationCondition has been met.
func (c *PartitionCoordinator) Run() {
	var wg sync.WaitGroup

	// terminate the for loop if the condition has been met
	for !c.ReadyToTerminate() {
		c.Step(&wg)
	}
}

// NewPartitionCoordinator creates a new PartitionCoordinator given a
// StochadexConfig.
func NewPartitionCoordinator(
	settings *Settings,
	implementations *Implementations,
) *PartitionCoordinator {
	timestepsHistory := &CumulativeTimestepsHistory{
		NextIncrement:     0.0,
		Values:            mat.NewVecDense(settings.TimestepsHistoryDepth, nil),
		CurrentStepNumber: 0,
		StateHistoryDepth: settings.TimestepsHistoryDepth,
	}
	timestepsHistory.Values.SetVec(0, settings.InitTimeValue)
	iterators := make([][]*StateIterator, 0)
	stateHistories := make([]*StateHistory, 0)
	newWorkChannels := make([](chan *IteratorInputMessage), 0)
	partitionIndices := make([][]int, 0)
	index := 0
	for parallelIndex, iterations := range implementations.Iterations {
		iterators = append(iterators, make([]*StateIterator, 0))
		partitionIndices = append(partitionIndices, make([]int, 0))
		for _, iteration := range iterations {
			partitionIndices[parallelIndex] = append(
				partitionIndices[parallelIndex],
				index,
			)
			stateHistoryValues := mat.NewDense(
				settings.StateHistoryDepths[index],
				settings.StateWidths[index],
				nil,
			)
			for elementIndex, element := range settings.InitStateValues[index] {
				stateHistoryValues.Set(0, elementIndex, element)
			}
			stateHistories = append(
				stateHistories,
				&StateHistory{
					Values:            stateHistoryValues,
					StateWidth:        settings.StateWidths[index],
					StateHistoryDepth: settings.StateHistoryDepths[index],
				},
			)
			iterators[parallelIndex] = append(
				iterators[parallelIndex],
				&StateIterator{
					Iteration:       iteration,
					Params:          settings.OtherParams[index],
					partitionIndex:  index,
					outputCondition: implementations.OutputCondition,
					outputFunction:  implementations.OutputFunction,
				},
			)
			newWorkChannels = append(
				newWorkChannels,
				make(chan *IteratorInputMessage),
			)
			index += 1
		}
	}
	return &PartitionCoordinator{
		Iterators:            iterators,
		StateHistories:       stateHistories,
		TimestepsHistory:     timestepsHistory,
		TimestepFunction:     implementations.TimestepFunction,
		TerminationCondition: implementations.TerminationCondition,
		PartitionIndices:     partitionIndices,
		newWorkChannels:      newWorkChannels,
	}
}
