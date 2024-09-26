package simulator

import (
	"gonum.org/v1/gonum/mat"
)

// Iteration is the interface that must be implemented for any stochastic
// phenomenon within the stochadex. Its .Iterate method reads in the Params
// map, a int partitionIndex, the full current history of the process defined
// by a slice []*StateHistory and a CumulativeTimestepsHistory reference and
// outputs an updated state history row in the form of a float64 slice.
type Iteration interface {
	Configure(partitionIndex int, settings *Settings)
	Iterate(
		params Params,
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
func (s *StateValueChannels) UpdateUpstreamParams(params Params) {
	for name, upstream := range s.Upstreams {
		switch indices := upstream.Indices; indices {
		case nil:
			params[name] = <-upstream.Channel
		default:
			values := <-upstream.Channel
			for i, index := range indices {
				values[i] = values[index]
			}
			params[name] = values[:len(indices)]
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

// StateIterator handles iterations of a given state partition on a
// separate goroutine and reads/writes data from/to the state history.
type StateIterator struct {
	Iteration       Iteration
	Params          Params
	PartitionIndex  int
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
		s.Params,
		s.PartitionIndex,
		stateHistories,
		timestepsHistory,
	)
	// get the new time for output
	time := timestepsHistory.Values.AtVec(0) + timestepsHistory.NextIncrement
	// also apply the output function if this step requires it
	if s.OutputCondition.IsOutputStep(s.PartitionIndex, newState, time) {
		s.OutputFunction.Output(s.PartitionIndex, newState, time)
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
	s.ValueChannels.UpdateUpstreamParams(s.Params)
	inputMessage.StateHistories[s.PartitionIndex].NextValues = s.Iterate(
		inputMessage.StateHistories,
		inputMessage.TimestepsHistory,
	)
	// broadcast a reference to the new state values for all downstream listeners
	s.ValueChannels.BroadcastDownstream(
		inputMessage.StateHistories[s.PartitionIndex].NextValues,
	)
}

// UpdateHistory should always follow a call to ReceiveAndIteratePending as it
// enacts the internal pending state update on the StateHistory object passed over
// the provided inputChannel.
func (s *StateIterator) UpdateHistory(inputChannel <-chan *IteratorInputMessage) {
	inputMessage := <-inputChannel
	// reference this partition
	partition := inputMessage.StateHistories[s.PartitionIndex]
	// make a temporary copy of this partition's previous values
	var partitionValuesCopy mat.Dense
	partitionValuesCopy.CloneFrom(inputMessage.StateHistories[s.PartitionIndex].Values)
	// iterate over the history (matrix columns) and shift them
	// back one timestep
	for i := 1; i < partition.StateHistoryDepth; i++ {
		partition.Values.SetRow(i, partitionValuesCopy.RawRowView(i-1))
	}
	// update the latest state in the history
	partition.Values.SetRow(0, partition.NextValues)
}
