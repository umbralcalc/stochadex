package simulator

import (
	"errors"
	"fmt"
	"runtime/debug"
	"sync/atomic"
	"unsafe"
)

// PartitionPanic is the panic a coordinator raises on its own goroutine when a
// partition's work panicked: in the partition's Iterate, its output, or the
// update of its history. The concurrent strategies run that work on worker
// goroutines, where a panic would otherwise end the process before any caller
// could recover it; the worker recovers it instead and the coordinator re-raises
// it once the step's other workers have finished, so a caller that recovers
// around Run or a Stepper's Step gets it, naming the partition. Under inline
// execution Run, and a stepping loop that defers NamePartitionPanic, raise the
// same value, so every strategy fails the same way; a bare inline Step, which
// defers nothing, raises the partition's own panic.
//
// When the panic is in a partition's iteration, the step is not committed:
// every state history and the time stay at the last completed step, and no
// partition that reads the failed one within the step runs. Either way, after a
// Step has panicked the stepper must only be closed, never stepped again.
type PartitionPanic struct {
	// Partition is the name of the partition whose work panicked.
	Partition string
	// Value is the value the partition panicked with.
	Value any
	// Stack is the panicking goroutine's stack, captured where it was
	// recovered: re-raising on the coordinator's goroutine loses it.
	Stack []byte
}

// Error names the partition and the value it panicked with.
func (p *PartitionPanic) Error() string {
	return fmt.Sprintf("partition %q panicked: %v", p.Partition, p.Value)
}

// Unwrap returns the panic value when it is an error, so errors.Is / errors.As
// see through to it.
func (p *PartitionPanic) Unwrap() error {
	err, _ := p.Value.(error)
	return err
}

// errUpstreamFailed is what a partition panics with when an upstream it reads
// within the step panicked. It is not a failure of its own and is never
// reported: the upstream's own panic, recorded first, is.
var errUpstreamFailed = errors.New("simulator: an upstream partition panicked")

// upstreamFailed is broadcast in place of a panicked partition's values, so its
// within-step consumers stop rather than wait forever for values it will never
// send. Its own backing array makes it distinguishable from every real
// broadcast, which is a fresh copy (nil when empty).
var upstreamFailed = make([]float64, 0, 1)

// isUpstreamFailed reports whether values received from an upstream channel is
// the upstreamFailed sentinel.
func isUpstreamFailed(values []float64) bool {
	return unsafe.SliceData(values) == unsafe.SliceData(upstreamFailed)
}

// partitionFailure holds the first panic recovered from a partition's work in
// the current step, for the coordinator to re-raise once the step's workers
// have finished.
type partitionFailure struct {
	first atomic.Pointer[PartitionPanic]
}

// record keeps recovered as the step's failure unless another partition failed
// first. A consumer stopped by a failed upstream is never first: the upstream
// records its own panic before it tells its consumers to stop.
func (f *partitionFailure) record(iterator *StateIterator, recovered any) {
	f.first.CompareAndSwap(nil, &PartitionPanic{
		Partition: iterator.Partition.Name,
		Value:     recovered,
		Stack:     debug.Stack(),
	})
}

// raise re-raises the recorded failure, if any, on the calling goroutine.
func (f *partitionFailure) raise() {
	if failure := f.first.Load(); failure != nil {
		panic(failure)
	}
}

// failIteration records a panic recovered from iterator's iteration phase and
// tells its within-step consumers, which are waiting on its values, to stop.
// Each consumer receives from all of its upstreams before it stops (see
// UpdateUpstreamParams), so every one of these sends is received.
func (f *partitionFailure) failIteration(iterator *StateIterator, recovered any) {
	f.record(iterator, recovered)
	if downstream := iterator.ValueChannels.Downstream; downstream != nil {
		for range downstream.Copies {
			downstream.Channel <- upstreamFailed
		}
	}
}
