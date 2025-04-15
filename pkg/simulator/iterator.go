package simulator

// Iteration is the interface that must be implemented for any partitioned
// state iteration in the stochadex. Its .Iterate method reads in the *Params,
// a int partitionIndex, the full current history of all partitions defined
// by a slice []*StateHistory and a *CumulativeTimestepsHistory reference and
// outputs an updated state history row in the form of a float64 slice.
type Iteration interface {
	Configure(partitionIndex int, settings *Settings)
	Iterate(
		params *Params,
		partitionIndex int,
		stateHistories []*StateHistory,
		timestepsHistory *CumulativeTimestepsHistory,
	) []float64
}

// UpstreamStateValues contains the information needed to receive state
// values from a computationally-upstream StateIterator.
type UpstreamStateValues struct {
	Channel chan []float64
	Indices []int
}

// DownstreamStateValues contains the information needed to send state
// values to a computationally-downstream StateIterator.
type DownstreamStateValues struct {
	Channel chan []float64
	Copies  int
}

// StateValueChannels defines the methods by which separate StateIterators
// can communicate with each other by sending the values of upstream
// iterators to downstream parameters via channels.
type StateValueChannels struct {
	Upstreams  map[string]*UpstreamStateValues
	Downstream *DownstreamStateValues
}

// UpdateUpstreamParams updates the provided params with the state values
// which have been provided computationally upstream via channels.
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

// BroadcastDownstream broadcasts the computationally-upstream state values
// to its configured number of downstreams on the relevant channel.
func (s *StateValueChannels) BroadcastDownstream(stateValues []float64) {
	for i := 0; i < s.Downstream.Copies; i++ {
		s.Downstream.Channel <- stateValues
	}
}

// NamedPartitionIndex pairs the name of a partition with the partition
// index assigned to it by the PartitionCoordinator.
type NamedPartitionIndex struct {
	Name  string
	Index int
}

// StateIterator handles iterations of a given state partition on a
// separate goroutine and reads/writes data from/to the state history.
type StateIterator struct {
	Iteration       Iteration
	Params          Params
	Partition       NamedPartitionIndex
	ValueChannels   StateValueChannels
	OutputCondition OutputCondition
	OutputFunction  OutputFunction
}

// Iterate takes the state and timesteps history and outputs an updated
// State struct using an implemented Iteration interface.
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

// ReceiveAndIteratePending listens for input messages sent to the input
// channel, runs Iterate when an IteratorInputMessage has been received on the
// provided inputChannel and then updates an internal pending state update object.
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

// UpdateHistory should always follow a call to ReceiveAndIteratePending as it
// enacts the internal pending state update on the StateHistory object passed over
// the provided inputChannel.
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

// NewStateIterator creates a new StateIterator, potentially also calling
// the output function if the condition is met by the initial state and time.
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
