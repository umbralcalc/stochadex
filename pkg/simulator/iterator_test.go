package simulator

import (
	"testing"

	"gonum.org/v1/gonum/mat"
)

// testIteration is a basic iteration for testing.
type testIteration struct {
}

func (t *testIteration) Configure(
	partitionIndex int,
	settings *Settings,
) {
}

func (t *testIteration) Iterate(
	params *Params,
	partitionIndex int,
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) []float64 {
	return []float64{0.0, 1.0, 2.0, 3.0}
}

func TestStateIterator(t *testing.T) {
	t.Run(
		"test the state value channels run",
		func(t *testing.T) {
			params := NewParams(make(map[string][]float64))
			values := []float64{0.0, 1.0, 2.0, 3.0}
			downstream := &DownstreamStateValues{
				Channel: make(chan []float64, 10),
				Copies:  1,
			}
			stateValueChannels := StateValueChannels{
				Upstreams: map[string]*UpstreamStateValues{
					"test_params": {Channel: downstream.Channel},
				},
				Downstream: downstream,
			}
			stateValueChannels.BroadcastDownstream(values)
			stateValueChannels.UpdateUpstreamParams(&params)
			for i, p := range params.Get("test_params") {
				if values[i] != p {
					t.Errorf("params didn't match: %f %f", values[i], p)
				}
			}
			iterator := &StateIterator{
				Iteration:       &testIteration{},
				Params:          params,
				Partition:       NamedPartitionIndex{Index: 0},
				ValueChannels:   stateValueChannels,
				OutputCondition: &NilOutputCondition{},
				OutputFunction:  &NilOutputFunction{},
			}
			inputChannel := make(chan *IteratorInputMessage, 10)
			message := &IteratorInputMessage{
				StateHistories: []*StateHistory{{
					Values: mat.NewDense(
						2,
						4,
						[]float64{0.0, 1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0},
					),
					NextValues:        []float64{1.0, 2.0, 3.0, 4.0},
					StateWidth:        4,
					StateHistoryDepth: 2,
				}},
				TimestepsHistory: &CumulativeTimestepsHistory{
					NextIncrement:     1.0,
					Values:            mat.NewVecDense(2, []float64{0.0, 1.0}),
					CurrentStepNumber: 0,
					StateHistoryDepth: 2,
				},
			}
			inputChannel <- message
			downstream.Channel <- values
			iterator.ReceiveAndIteratePending(inputChannel)
			inputChannel <- message
			iterator.UpdateHistory(inputChannel)
		},
	)
	t.Run(
		"test indexed upstream params from same source don't corrupt each other",
		func(t *testing.T) {
			params := NewParams(make(map[string][]float64))
			values := []float64{10.0, 20.0, 30.0, 40.0}
			downstream := &DownstreamStateValues{
				Channel: make(chan []float64, 10),
				Copies:  2,
			}
			// two upstream readers index different elements
			// from the same downstream channel
			stateValueChannels := StateValueChannels{
				Upstreams: map[string]*UpstreamStateValues{
					"reader_a": {
						Channel: downstream.Channel,
						Indices: []int{0, 2},
					},
					"reader_b": {
						Channel: downstream.Channel,
						Indices: []int{1, 3},
					},
				},
				Downstream: downstream,
			}
			stateValueChannels.BroadcastDownstream(values)
			stateValueChannels.UpdateUpstreamParams(&params)

			// reader_a should see [10.0, 30.0]
			readerA := params.Get("reader_a")
			if len(readerA) != 2 || readerA[0] != 10.0 || readerA[1] != 30.0 {
				t.Errorf("reader_a got %v, want [10 30]", readerA)
			}
			// reader_b should see [20.0, 40.0]
			readerB := params.Get("reader_b")
			if len(readerB) != 2 || readerB[0] != 20.0 || readerB[1] != 40.0 {
				t.Errorf("reader_b got %v, want [20 40]", readerB)
			}
		},
	)
	t.Run(
		"broadcast sends independent buffers per downstream listener",
		func(t *testing.T) {
			downstream := &DownstreamStateValues{
				Channel: make(chan []float64, 10),
				Copies:  2,
			}
			original := []float64{1.0, 2.0}
			ch := StateValueChannels{Downstream: downstream}
			ch.BroadcastDownstream(original)
			a := <-downstream.Channel
			b := <-downstream.Channel
			if len(a) != 2 || len(b) != 2 {
				t.Fatalf("unexpected lengths a=%d b=%d", len(a), len(b))
			}
			a[0] = 99.0
			if b[0] == 99.0 {
				t.Error("downstream copies share backing array")
			}
		},
	)
}
