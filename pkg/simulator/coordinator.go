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
	Shared               *IteratorInputMessage
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
		channel <- c.Shared
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
		channel <- c.Shared
	}

	// iterate over the history of timesteps and shift them back one
	var timestepsHistoryValuesCopy mat.VecDense
	timestepsHistoryValuesCopy.CloneFromVec(c.Shared.TimestepsHistory.Values)
	for i := 1; i < c.Shared.TimestepsHistory.StateHistoryDepth; i++ {
		c.Shared.TimestepsHistory.Values.SetVec(i, timestepsHistoryValuesCopy.AtVec(i-1))
	}
	// now update the history with the next time increment
	c.Shared.TimestepsHistory.Values.SetVec(
		0,
		timestepsHistoryValuesCopy.AtVec(0)+c.Shared.TimestepsHistory.NextIncrement,
	)
}

// Step is the main method call of PartitionCoordinator - call this proceeding
// a new configuration of the latter to run the desired process for a single step.
func (c *PartitionCoordinator) Step(wg *sync.WaitGroup) {
	// update the overall step count and get the next time increment
	c.Shared.TimestepsHistory.CurrentStepNumber += 1
	c.Shared.TimestepsHistory.NextIncrement = c.TimestepFunction.NextIncrement(
		c.Shared.TimestepsHistory,
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
		c.Shared.StateHistories,
		c.Shared.TimestepsHistory,
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
		for _, values := range partition.ParamsFromUpstream {
			_, ok := listenersByPartition[values.Upstream]
			if !ok {
				listenersByPartition[values.Upstream] = 0
			}
			listenersByPartition[values.Upstream] += 1
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
		for params, values := range partition.ParamsFromUpstream {
			upstreamByParams[params] = &UpstreamStateValues{
				Channel: valueChannels[values.Upstream],
				Indices: values.Indices,
			}
		}
		iterators = append(
			iterators,
			&StateIterator{
				Iteration:      partition.Iteration,
				Params:         settings.Params[index],
				PartitionName:  partition.Name,
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
		Iterators: iterators,
		Shared: &IteratorInputMessage{
			StateHistories:   stateHistories,
			TimestepsHistory: timestepsHistory,
		},
		TimestepFunction:     implementations.TimestepFunction,
		TerminationCondition: implementations.TerminationCondition,
		newWorkChannels:      newWorkChannels,
	}
}
