package simulator

import (
	"sync"

	"gonum.org/v1/gonum/mat"
)

// PartitionCoordinator coordinates the assignment of iteration work to
// separate StateIterator objects on separate goroutines and when to enact
// these updates on the state history.
type PartitionCoordinator struct {
	Iterators            []*StateIterator
	newWorkChannels      [](chan *IteratorInputMessage)
	stateHistories       []*StateHistory
	overallTimesteps     int
	numberOfPartitions   int
	timestepsHistory     *TimestepsHistory
	timestepFunction     TimestepFunction
	terminationCondition TerminationCondition
}

// RequestMoreIterations spawns a goroutine for each state partition to
// carry out a ReceiveAndIteratePending job.
func (c *PartitionCoordinator) RequestMoreIterations(wg *sync.WaitGroup) {
	// setup iterators to receive and send their iteration results
	for index := 0; index < c.numberOfPartitions; index++ {
		wg.Add(1)
		i := index
		go func() {
			defer wg.Done()
			c.Iterators[i].ReceiveAndIteratePending(c.newWorkChannels[i])
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
}

// RequestMoreIterations spawns a goroutine for each state partition to
// carry out an UpdateHistory job.
func (c *PartitionCoordinator) UpdateHistory(wg *sync.WaitGroup) {
	// setup iterators to receive and send their iteration results
	for index := 0; index < c.numberOfPartitions; index++ {
		wg.Add(1)
		i := index
		go func() {
			defer wg.Done()
			c.Iterators[i].UpdateHistory(c.newWorkChannels[i])
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

	// iterate over the history of timesteps and shift them back one
	for i := 1; i < c.timestepsHistory.StateHistoryDepth; i++ {
		c.timestepsHistory.Values.SetVec(i, c.timestepsHistory.Values.AtVec(i-1))
	}
	// now update the history with the next time increment
	c.timestepsHistory.Values.SetVec(
		0,
		c.timestepsHistory.Values.AtVec(0)+c.timestepsHistory.NextIncrement,
	)
}

// Step is the main method call of PartitionCoordinator - call this proceeding
// a new configuration of the latter to run the desired process for a single step.
func (c *PartitionCoordinator) Step(wg *sync.WaitGroup) {
	// update the overall step count and get the next time increment
	c.overallTimesteps += 1
	c.timestepsHistory = c.timestepFunction.NextIncrement(c.timestepsHistory)

	// begin by requesting iterations for the next step and waiting
	c.RequestMoreIterations(wg)
	wg.Wait()

	// then implement the pending state and time updates to the histories
	c.UpdateHistory(wg)
	wg.Wait()
}

// ReadyToTerminate returns whether or not the process has met the TerminationCondition.
func (c *PartitionCoordinator) ReadyToTerminate() bool {
	return c.terminationCondition.Terminate(
		c.stateHistories,
		c.timestepsHistory,
		c.overallTimesteps,
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
func NewPartitionCoordinator(config *StochadexConfig) *PartitionCoordinator {
	timestepsHistory := &TimestepsHistory{
		NextIncrement:     0.0,
		Values:            mat.NewVecDense(config.Steps.TimestepsHistoryDepth, nil),
		StateHistoryDepth: config.Steps.TimestepsHistoryDepth,
	}
	iterators := make([]*StateIterator, 0)
	stateHistories := make([]*StateHistory, 0)
	newWorkChannels := make([](chan *IteratorInputMessage), 0)
	for index, stateConfig := range config.Partitions {
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
				iteration:       stateConfig.Iteration,
				outputCondition: config.Output.Condition,
				outputFunction:  config.Output.Function,
			},
		)
		newWorkChannels = append(
			newWorkChannels,
			make(chan *IteratorInputMessage),
		)
	}
	return &PartitionCoordinator{
		Iterators:            iterators,
		newWorkChannels:      newWorkChannels,
		stateHistories:       stateHistories,
		overallTimesteps:     0,
		numberOfPartitions:   len(config.Partitions),
		timestepsHistory:     timestepsHistory,
		timestepFunction:     config.Steps.TimestepFunction,
		terminationCondition: config.Steps.TerminationCondition,
	}
}
