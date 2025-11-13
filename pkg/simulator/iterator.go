package simulator

// Iteration defines the interface for per-partition state update functions
// in stochadex simulations.
//
// The Iteration interface is the fundamental building block for defining
// how simulation state evolves over time. Each partition in a simulation
// uses an Iteration to compute its next state values based on the current
// state, parameters, and time information.
//
// Design Philosophy:
// The Iteration interface emphasizes modularity and composability. By
// providing a simple, well-defined interface, it enables the creation of
// complex simulations through the combination of simple, focused iterations.
// This design supports both built-in iteration types and custom user-defined
// iterations.
//
// Interface Methods:
//   - Configure: Initialize the iteration with simulation settings (called once)
//   - Iterate: Compute the next state values (called each simulation step)
//
// Configuration Phase:
// Configure is called once per partition during simulation setup. It receives:
//   - partitionIndex: The index of this partition in the simulation
//   - settings: Global simulation settings and configuration
//
// This phase is used for:
//   - Initializing random number generators
//   - Setting up internal data structures
//   - Configuring iteration-specific parameters
//   - Validating configuration parameters
//
// Iteration Phase:
// Iterate is called each simulation step to compute the next state values.
// It receives:
//   - params: Current simulation parameters for this partition
//   - partitionIndex: The index of this partition
//   - stateHistories: State histories for all partitions (for cross-partition access)
//   - timestepsHistory: Time and timestep information
//
// It must return:
//   - []float64: The next state values for this partition
//
// Implementation Requirements:
//   - Configure must be called before Iterate
//   - Iterate must return a slice of the correct length (matching state width)
//   - Iterate should not modify the input parameters or state histories
//   - Iterate should be deterministic given the same inputs and initial seed (for reproducible simulations)
//
// Example Usage:
//
//	type MyIteration struct {
//	    // Internal state
//	}
//
//	func (m *MyIteration) Configure(partitionIndex int, settings *Settings) {
//	    // Initialize iteration
//	}
//
//	func (m *MyIteration) Iterate(params *Params, partitionIndex int,
//	                              stateHistories []*StateHistory,
//	                              timestepsHistory *CumulativeTimestepsHistory) []float64 {
//	    // Compute next state values
//	    return []float64{newValue1, newValue2, ...}
//	}
//
// Common Iteration Types:
//   - Stochastic processes: WienerProcessIteration, PoissonProcessIteration
//   - Deterministic functions: ValuesFunctionIteration, ConstantValuesIteration
//   - Aggregation functions: VectorMeanIteration, GroupedAggregationIteration
//   - User-defined iterations: Custom implementations for specific needs
//
// Performance Considerations:
//   - Iterate is called frequently during simulation execution
//   - Implementations should be optimized for performance
//   - Avoid expensive computations or memory allocations in Iterate
//   - Consider caching expensive computations in Configure
//
// Thread Safety:
//   - Iterate may be called concurrently from multiple goroutines
//   - Implementations should be thread-safe or stateless
type Iteration interface {
	Configure(partitionIndex int, settings *Settings)
	Iterate(
		params *Params,
		partitionIndex int,
		stateHistories []*StateHistory,
		timestepsHistory *CumulativeTimestepsHistory,
	) []float64
}

// UpstreamStateValues contains information to receive state values from an
// upstream iterator via channel.
type UpstreamStateValues struct {
	Channel chan []float64
	Indices []int
}

// DownstreamStateValues contains information to broadcast state values to
// downstream iterators via channel.
type DownstreamStateValues struct {
	Channel chan []float64
	Copies  int
}

// StateValueChannels provides upstream/downstream channels for inter-iterator
// communication.
type StateValueChannels struct {
	Upstreams  map[string]*UpstreamStateValues
	Downstream *DownstreamStateValues
}

// UpdateUpstreamParams updates Params with values received from upstream
// channels.
func (s *StateValueChannels) UpdateUpstreamParams(params *Params) {
	for name, upstream := range s.Upstreams {
		switch indices := upstream.Indices; indices {
		case nil:
			params.Set(name, <-upstream.Channel)
		default:
			values := <-upstream.Channel
			for i, index := range indices {
				values[i] = values[index]
			}
			params.Set(name, values[:len(indices)])
		}
	}
}

// BroadcastDownstream sends state values to all configured downstream copies.
func (s *StateValueChannels) BroadcastDownstream(stateValues []float64) {
	for range s.Downstream.Copies {
		s.Downstream.Channel <- stateValues
	}
}

// NamedPartitionIndex pairs the name of a partition with the partition
// index assigned to it by the PartitionCoordinator.
type NamedPartitionIndex struct {
	Name  string
	Index int
}

// StateIterator runs an Iteration for a partition on a goroutine and
// manages reads/writes to history and output.
type StateIterator struct {
	Iteration       Iteration
	Params          Params
	Partition       NamedPartitionIndex
	ValueChannels   StateValueChannels
	OutputCondition OutputCondition
	OutputFunction  OutputFunction
}

// Iterate runs the Iteration and optionally triggers output if the condition
// is met for the new state/time.
func (s *StateIterator) Iterate(
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) []float64 {
	newState := s.Iteration.Iterate(
		&s.Params,
		s.Partition.Index,
		stateHistories,
		timestepsHistory,
	)
	// get the new time for output
	time := timestepsHistory.Values.AtVec(0) + timestepsHistory.NextIncrement
	// also apply the output function if this step requires it
	if s.OutputCondition.IsOutputStep(s.Partition.Name, newState, time) {
		s.OutputFunction.Output(s.Partition.Name, newState, time)
	}
	return newState
}

// ReceiveAndIteratePending listens for an IteratorInputMessage, updates
// upstream-driven params, runs Iterate, and stores a pending state update.
func (s *StateIterator) ReceiveAndIteratePending(
	inputChannel <-chan *IteratorInputMessage,
) {
	inputMessage := <-inputChannel
	// listen to the upstream channels which may set new params
	s.ValueChannels.UpdateUpstreamParams(&s.Params)
	inputMessage.StateHistories[s.Partition.Index].NextValues = s.Iterate(
		inputMessage.StateHistories,
		inputMessage.TimestepsHistory,
	)
	// broadcast a reference to the new state values for all downstream listeners
	s.ValueChannels.BroadcastDownstream(
		inputMessage.StateHistories[s.Partition.Index].NextValues,
	)
}

// UpdateHistory applies the pending state update to the partition history.
func (s *StateIterator) UpdateHistory(inputChannel <-chan *IteratorInputMessage) {
	inputMessage := <-inputChannel
	// reference this partition
	partition := inputMessage.StateHistories[s.Partition.Index]
	// iterate over the history (matrix columns) and shift them
	// back one timestep
	for i := partition.StateHistoryDepth - 1; i > 0; i-- {
		partition.Values.SetRow(i, partition.Values.RawRowView(i-1))
	}
	// update the latest state in the history
	partition.Values.SetRow(0, partition.NextValues)
}

// NewStateIterator creates a StateIterator and may emit initial output if
// the condition is met by the initial state/time.
func NewStateIterator(
	iteration Iteration,
	params Params,
	partitionName string,
	partitionIndex int,
	valueChannels StateValueChannels,
	outputCondition OutputCondition,
	outputFunction OutputFunction,
	initState []float64,
	initTime float64,
) *StateIterator {
	// allows for the initial state values to potentially be output as well
	if outputCondition.IsOutputStep(partitionName, initState, initTime) {
		outputFunction.Output(partitionName, initState, initTime)
	}
	return &StateIterator{
		Iteration: iteration,
		Params:    params,
		Partition: NamedPartitionIndex{
			Name:  partitionName,
			Index: partitionIndex,
		},
		ValueChannels:   valueChannels,
		OutputCondition: outputCondition,
		OutputFunction:  outputFunction,
	}
}
