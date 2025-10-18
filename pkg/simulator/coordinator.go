// Package simulator provides the core simulation engine and infrastructure
// for stochadex simulations. It includes the main simulation loop, state management,
// partition coordination, and execution control mechanisms.
//
// Key Features:
//   - Partition-based simulation architecture
//   - Concurrent execution with goroutine coordination
//   - State history management and time tracking
//   - Configurable termination and output conditions
//   - Flexible timestep control
//   - Thread-safe state storage and communication
//
// Architecture Overview:
// The simulator uses a partition-based architecture where each partition
// represents a component of the simulation state. Partitions can communicate
// through upstream/downstream channels, enabling complex multi-component
// simulations with dependencies between components.
//
// Core Components:
//   - PartitionCoordinator: Orchestrates execution across all partitions
//   - StateIterator: Manages individual partition execution and communication
//   - StateTimeStorage: Thread-safe storage for simulation results
//   - ConfigGenerator: Creates simulation configurations from settings
//   - TerminationCondition: Controls when simulations stop
//   - OutputFunction: Handles result collection and storage
//
// Design Philosophy:
// The simulator emphasizes modularity, concurrency, and flexibility. It provides
// a robust foundation for building complex simulations while maintaining good
// performance characteristics and thread safety.
//
// Usage Patterns:
//   - Multi-component system simulation
//   - Agent-based modeling with interactions
//   - Monte Carlo simulations with multiple sources of randomness
//   - Time-series analysis and forecasting
//   - Parameter estimation and optimization
package simulator

import (
	"sync"

	"gonum.org/v1/gonum/mat"
)

// PartitionCoordinator orchestrates iteration work across partitions and
// applies state/time history updates in a coordinated manner.
//
// The PartitionCoordinator is the central component that manages the execution
// of all partitions in a simulation. It coordinates the timing, communication,
// and state updates across all partitions, ensuring proper synchronization
// and maintaining simulation consistency.
//
// Architecture:
// The coordinator uses a two-phase execution model:
//  1. Iteration Phase: All partitions compute their next state values
//  2. Update Phase: State and time histories are updated with new values
//
// This design ensures that all partitions see consistent state information
// during each iteration, preventing race conditions and maintaining
// simulation determinism.
//
// Concurrency Model:
//   - Each partition runs in its own goroutine for parallel execution
//   - Channels are used for inter-partition communication
//   - WaitGroups ensure proper synchronization between phases
//   - Shared state is protected by the coordinator's control flow
//
// Execution Flow:
//  1. Compute next timestep increment using TimestepFunction
//  2. Request iterations from all partitions (parallel execution)
//  3. Wait for all iterations to complete
//  4. Update state and time histories (parallel execution)
//  5. Check termination condition
//  6. Repeat until termination
//
// Fields:
//   - Iterators: List of StateIterators, one per partition
//   - Shared: Shared state and time information accessible to all partitions
//   - TimestepFunction: Function that determines the next timestep increment
//   - TerminationCondition: Condition that determines when to stop the simulation
//   - newWorkChannels: Communication channels for coordinating partition work
//
// Example Usage:
//
//	coordinator := NewPartitionCoordinator(settings, implementations)
//
//	// Run simulation until termination
//	coordinator.Run()
//
//	// Or step-by-step control
//	for !coordinator.ReadyToTerminate() {
//	    var wg sync.WaitGroup
//	    coordinator.Step(&wg)
//	}
//
// Performance:
//   - O(p) time complexity where p is the number of partitions
//   - Parallel execution of partition iterations
//   - Efficient channel-based communication
//   - Memory usage scales with partition count and state size
//
// Thread Safety:
//   - Safe for concurrent access to coordinator methods
//   - Internal synchronization ensures consistent state updates
//   - Partition communication is thread-safe through channels
type PartitionCoordinator struct {
	Iterators            []*StateIterator
	Shared               *IteratorInputMessage
	TimestepFunction     TimestepFunction
	TerminationCondition TerminationCondition
	newWorkChannels      [](chan *IteratorInputMessage)
}

// RequestMoreIterations spawns a goroutine per partition to run
// ReceiveAndIteratePending.
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

// UpdateHistory spawns a goroutine per partition to run UpdateHistory and
// shifts time history forward, adding NextIncrement to t[0].
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
	for i := c.Shared.TimestepsHistory.StateHistoryDepth - 1; i > 0; i-- {
		c.Shared.TimestepsHistory.Values.SetVec(i,
			c.Shared.TimestepsHistory.Values.AtVec(i-1))
	}
	// now update the history with the next time increment
	c.Shared.TimestepsHistory.Values.SetVec(0,
		c.Shared.TimestepsHistory.Values.AtVec(0)+
			c.Shared.TimestepsHistory.NextIncrement)
}

// Step performs one simulation tick: compute dt, request iterations, then
// apply state/time updates.
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

// ReadyToTerminate returns whether the TerminationCondition is met.
func (c *PartitionCoordinator) ReadyToTerminate() bool {
	return c.TerminationCondition.Terminate(
		c.Shared.StateHistories,
		c.Shared.TimestepsHistory,
	)
}

// Run advances by repeatedly calling Step until termination.
func (c *PartitionCoordinator) Run() {
	var wg sync.WaitGroup

	// terminate the for loop if the condition has been met
	for !c.ReadyToTerminate() {
		c.Step(&wg)
	}
}

// NewPartitionCoordinator wires Settings and Implementations into a runnable
// coordinator with initial state/time histories and channels.
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
	for _, iteration := range settings.Iterations {
		valueChannels = append(valueChannels, make(chan []float64))
		for _, values := range iteration.ParamsFromUpstream {
			_, ok := listenersByPartition[values.Upstream]
			if !ok {
				listenersByPartition[values.Upstream] = 0
			}
			listenersByPartition[values.Upstream] += 1
		}
	}
	for index, iteration := range settings.Iterations {
		stateHistoryValues := mat.NewDense(
			iteration.StateHistoryDepth,
			iteration.StateWidth,
			nil,
		)
		stateHistoryValues.SetRow(0, iteration.InitStateValues)
		stateHistories = append(
			stateHistories,
			&StateHistory{
				Values:            stateHistoryValues,
				StateWidth:        iteration.StateWidth,
				StateHistoryDepth: iteration.StateHistoryDepth,
			},
		)
		upstreamByParams := make(map[string]*UpstreamStateValues)
		for params, values := range iteration.ParamsFromUpstream {
			upstreamByParams[params] = &UpstreamStateValues{
				Channel: valueChannels[values.Upstream],
				Indices: values.Indices,
			}
		}
		iterators = append(
			iterators,
			NewStateIterator(
				implementations.Iterations[index],
				iteration.Params,
				iteration.Name,
				index,
				StateValueChannels{
					Upstreams: upstreamByParams,
					Downstream: &DownstreamStateValues{
						Channel: valueChannels[index],
						Copies:  listenersByPartition[index],
					},
				},
				implementations.OutputCondition,
				implementations.OutputFunction,
				iteration.InitStateValues,
				settings.InitTimeValue,
			),
		)
		newWorkChannels = append(
			newWorkChannels,
			make(chan *IteratorInputMessage, 1),
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
