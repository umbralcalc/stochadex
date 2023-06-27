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
	StateHistories       []*StateHistory
	TimestepsHistory     *CumulativeTimestepsHistory
	newWorkChannels      [](chan *IteratorInputMessage)
	overallTimesteps     int
	numberOfPartitions   int
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
			StateHistories:   c.StateHistories,
			TimestepsHistory: c.TimestepsHistory,
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
			StateHistories:   c.StateHistories,
			TimestepsHistory: c.TimestepsHistory,
		}
	}

	// iterate over the history of timesteps and shift them back one
	timestepsHistoryValuesCopy := *c.TimestepsHistory.Values
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
	c.overallTimesteps += 1
	c.TimestepsHistory = c.timestepFunction.SetNextIncrement(c.TimestepsHistory)

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
		c.StateHistories,
		c.TimestepsHistory,
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
	timestepsHistory := &CumulativeTimestepsHistory{
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
				Iteration:       stateConfig.Iteration,
				Params:          stateConfig.Params,
				partitionIndex:  index,
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
		StateHistories:       stateHistories,
		TimestepsHistory:     timestepsHistory,
		newWorkChannels:      newWorkChannels,
		overallTimesteps:     0,
		numberOfPartitions:   len(config.Partitions),
		timestepFunction:     config.Steps.TimestepFunction,
		terminationCondition: config.Steps.TerminationCondition,
	}
}
