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
//	// Or step-by-step control under the configured execution strategy
//	stepper := coordinator.NewStepper()
//	defer stepper.Close()
//	for !coordinator.ReadyToTerminate() {
//	    stepper.Step()
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
	RunStrategy          ExecutionStrategy
	// OutputFunction is retained solely so Run can Finalize a sink that implements
	// FinalizingOutputFunction; per-step output goes through the iterators.
	OutputFunction  OutputFunction
	newWorkChannels [](chan *IteratorInputMessage)
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

	c.advanceTimestepsHistory()
}

// beginStep opens a simulation tick: it advances the step counter and computes
// the next timestep increment. It is the shared prologue of every strategy's
// Stepper (and of Step), so the "how do we start a step" logic lives in one
// place.
func (c *PartitionCoordinator) beginStep() {
	c.Shared.TimestepsHistory.CurrentStepNumber += 1
	c.Shared.TimestepsHistory.NextIncrement = c.TimestepFunction.NextIncrement(
		c.Shared.TimestepsHistory,
	)
}

// advanceTimestepsHistory closes a simulation tick: it shifts the timesteps
// history back one and appends the next time value. It is the shared epilogue
// of every strategy's Stepper (and of UpdateHistory), so the "how do we commit
// the new time" logic lives in one place.
func (c *PartitionCoordinator) advanceTimestepsHistory() {
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

// Step performs one simulation tick under the default spawn-per-step execution:
// compute dt, request iterations, then apply state/time updates. It is the
// single-step primitive the SpawnPerStepExecution stepper delegates to; other
// strategies advance a step through their own Stepper. Callers that want to
// drive a step under the coordinator's configured strategy should use
// NewStepper instead.
func (c *PartitionCoordinator) Step(wg *sync.WaitGroup) {
	c.beginStep()

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

// NewStepper returns a Stepper that advances the coordinator one step at a
// time under its configured RunStrategy (a nil RunStrategy selects the default
// spawn-per-step execution). This is the strategy-aware counterpart to Step:
// it lets callers drive any execution strategy stepwise — inspecting or
// mutating state between steps — exactly as the default algorithm can be driven
// with Step, while keeping that strategy's execution policy (persistent
// workers, inline execution, ...).
//
// The caller drives Step until ReadyToTerminate reports true and must call the
// stepper's Close when done to release any resources it holds:
//
//	stepper := coordinator.NewStepper()
//	defer stepper.Close()
//	for !coordinator.ReadyToTerminate() {
//	    stepper.Step()
//	}
func (c *PartitionCoordinator) NewStepper() Stepper {
	if c.RunStrategy != nil {
		return c.RunStrategy.NewStepper(c)
	}
	return (&SpawnPerStepExecution{}).NewStepper(c)
}

// Run advances the coordinator to termination under its configured RunStrategy
// (a nil RunStrategy selects the default spawn-per-step two-phase execution).
// It is the canonical run loop shared by every strategy: build a Stepper, step
// until termination, then release the stepper.
func (c *PartitionCoordinator) Run() {
	stepper := c.NewStepper()
	defer stepper.Close()

	// terminate the for loop if the condition has been met
	for !c.ReadyToTerminate() {
		stepper.Step()
	}

	// Give a resource-holding sink its one chance to flush/seal once no further
	// output can arrive (see FinalizingOutputFunction). Sinks that do not implement
	// it are untouched.
	if f, ok := c.OutputFunction.(FinalizingOutputFunction); ok {
		f.Finalize()
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
	implementations.OutputFunction.Configure(settings)
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
				NextValues:        make([]float64, iteration.StateWidth),
				StateWidth:        iteration.StateWidth,
				StateHistoryDepth: iteration.StateHistoryDepth,
			},
		)
		upstreamByParams := make(map[string]*UpstreamStateValues)
		for params, values := range iteration.ParamsFromUpstream {
			upstreamByParams[params] = &UpstreamStateValues{
				Channel:  valueChannels[values.Upstream],
				Indices:  values.Indices,
				Upstream: values.Upstream,
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
				timestepsHistory,
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
		RunStrategy:          implementations.ExecutionStrategy,
		OutputFunction:       implementations.OutputFunction,
		newWorkChannels:      newWorkChannels,
	}
}
