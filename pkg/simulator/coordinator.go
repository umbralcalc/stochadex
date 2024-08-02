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
	TimestepFunction     TimestepFunction
	TerminationCondition TerminationCondition
	newWorkChannels      [](chan *IteratorInputMessage)
}

// RequestMoreIterations spawns a goroutine for each state partition to
// carry out a ReceiveAndIteratePending job.
func (c *PartitionCoordinator) RequestMoreIterations(wg *sync.WaitGroup) {
	// setup iterators to receive and send their iteration results
	for partitionIndex, iterator := range c.Iterators {
		wg.Add(1)
		go func(partitionIndex int, iterator *StateIterator) {
			defer wg.Done()
			iterator.ReceiveAndIteratePending(c.newWorkChannels[partitionIndex])
		}(partitionIndex, iterator)
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
	for partitionIndex, iterator := range c.Iterators {
		wg.Add(1)
		go func(partitionIndex int, iterator *StateIterator) {
			defer wg.Done()
			iterator.UpdateHistory(c.newWorkChannels[partitionIndex])
		}(partitionIndex, iterator)
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
	c.TimestepsHistory.NextIncrement = c.TimestepFunction.NextIncrement(
		c.TimestepsHistory,
	)

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
	iterators := make([]*StateIterator, 0)
	stateHistories := make([]*StateHistory, 0)
	newWorkChannels := make([](chan *IteratorInputMessage), 0)
	valueChannels := make([](chan []float64), 0)
	listenersByPartition := make(map[int]int)
	for _, partition := range implementations.Partitions {
		valueChannels = append(valueChannels, make(chan []float64))
		for _, upstreamPartition := range partition.ParamsFromUpstreamPartition {
			_, ok := listenersByPartition[upstreamPartition]
			if !ok {
				listenersByPartition[upstreamPartition] = 0
			}
			listenersByPartition[upstreamPartition] += 1
		}
	}
	for index, partition := range implementations.Partitions {
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
		upstreamByParams := make(map[string]*UpstreamStateValues)
		for params, upstream := range partition.ParamsFromUpstreamPartition {
			var slice []int
			if partition.ParamsFromSlice != nil {
				slice = partition.ParamsFromSlice[params]
			}
			upstreamByParams[params] = &UpstreamStateValues{
				Channel: valueChannels[upstream],
				Slice:   slice,
			}
		}
		iterators = append(
			iterators,
			&StateIterator{
				Iteration:      partition.Iteration,
				Params:         settings.OtherParams[index],
				PartitionIndex: index,
				ValueChannels: StateValueChannels{
					Upstreams: upstreamByParams,
					Downstream: &DownstreamStateValues{
						Channel: valueChannels[index],
						Copies:  listenersByPartition[index],
					},
				},
				OutputCondition: implementations.OutputCondition,
				OutputFunction:  implementations.OutputFunction,
			},
		)
		newWorkChannels = append(
			newWorkChannels,
			make(chan *IteratorInputMessage),
		)
		index += 1
	}
	return &PartitionCoordinator{
		Iterators:            iterators,
		StateHistories:       stateHistories,
		TimestepsHistory:     timestepsHistory,
		TimestepFunction:     implementations.TimestepFunction,
		TerminationCondition: implementations.TerminationCondition,
		newWorkChannels:      newWorkChannels,
	}
}
