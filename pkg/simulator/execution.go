package simulator

import "sync"

// ExecutionStrategy chooses how a PartitionCoordinator advances — the policy
// for turning per-partition work into completed steps (spawn a goroutine per
// partition per step, keep one persistent worker per partition, run everything
// inline on the calling goroutine, ...).
//
// A nil ExecutionStrategy on a coordinator (or on Implementations) selects the
// default spawn-per-step two-phase execution. A strategy is purely an
// execution-policy choice, not a semantic one: every strategy must produce
// byte-identical output to the default for the same Settings and
// Implementations. This invariant is enforced by the cross-strategy
// equivalence tests.
//
// A strategy's single primitive is NewStepper, which builds a Stepper holding
// whatever per-run state the policy needs. Both batch execution
// (PartitionCoordinator.Run) and manual stepwise driving
// (PartitionCoordinator.NewStepper) are expressed in terms of it, so every
// strategy is steppable in exactly the same way the default two-phase
// algorithm is — there is no strategy that can only run to termination.
type ExecutionStrategy interface {
	// NewStepper returns a Stepper that advances c one step at a time under
	// this strategy's execution policy.
	NewStepper(c *PartitionCoordinator) Stepper
}

// Stepper advances a PartitionCoordinator one step at a time under a specific
// ExecutionStrategy's policy. It owns the per-run state that policy needs —
// e.g. the persistent worker goroutines of PersistentWorkerExecution — which
// is why stepping is a distinct object rather than a bare coordinator method:
// that state lives across steps and must be released with Close when the run
// ends.
//
// Typical stepwise use, which mirrors the default algorithm's step-by-step
// control but keeps the chosen strategy's execution policy:
//
//	stepper := coordinator.NewStepper()
//	defer stepper.Close()
//	for !coordinator.ReadyToTerminate() {
//	    stepper.Step()
//	    // inspect or mutate state between steps here (keyboard input,
//	    // external coupling, output throttling, per-step assertions, ...)
//	}
//
// Step advances by exactly one simulation tick — compute the next timestep
// increment, run the iteration phase for every partition, then the update
// phase — leaving the coordinator in the same committed state the default
// algorithm reaches after one Step. Close releases resources and must be
// called exactly once; Step must not be called after Close.
type Stepper interface {
	Step()
	Close()
}

// SpawnPerStepExecution is the default execution strategy: each step spawns one
// goroutine per partition for the iteration phase and again for the update
// phase, synchronised by a two-phase barrier. It is the named, explicitly
// selectable form of the behaviour used when no strategy is configured.
//
// This strategy is stateless and safe to share across coordinators.
type SpawnPerStepExecution struct{}

// NewStepper returns a Stepper that advances the coordinator using the default
// spawn-per-step two-phase execution (one goroutine per partition per phase).
func (e *SpawnPerStepExecution) NewStepper(c *PartitionCoordinator) Stepper {
	return &spawnPerStepStepper{coordinator: c}
}

// spawnPerStepStepper delegates each step to PartitionCoordinator.Step, which
// spawns the per-phase goroutines. The reused WaitGroup carries no state
// between steps (it always returns to zero), so Close is a no-op.
type spawnPerStepStepper struct {
	coordinator *PartitionCoordinator
	waitGroup   sync.WaitGroup
}

// Step advances the coordinator by one spawn-per-step tick.
func (s *spawnPerStepStepper) Step() { s.coordinator.Step(&s.waitGroup) }

// Close releases the stepper. Spawn-per-step holds no long-lived resources, so
// this does nothing.
func (s *spawnPerStepStepper) Close() {}

// PersistentWorkerExecution runs the simulation with one long-lived goroutine
// per partition rather than spawning a fresh goroutine per partition per phase
// per step. Each worker loops "wait-for-iterate -> iterate -> signal-done ->
// wait-for-update -> update -> signal-done", which removes the per-step
// goroutine spawn/teardown cost.
//
// The two-phase barrier is retained: workers are still woken and acknowledged
// once per phase so the update phase observes a consistent snapshot. This
// strategy therefore moves the per-step constant down (no spawn allocations)
// but keeps the per-step cross-goroutine synchronisation; it does not cross the
// serial floor for trivially small per-step work.
//
// Output is byte-identical to the default strategy: the per-partition work and
// the barrier ordering are unchanged; only the goroutine lifetime differs. The
// workers are spawned by NewStepper and torn down by the returned Stepper's
// Close, so a stepwise caller keeps the same persistent workers across every
// Step rather than paying setup per step.
//
// This strategy is stateless and safe to share across coordinators; all
// per-run state lives on the Stepper.
type PersistentWorkerExecution struct{}

// NewStepper spins up one long-lived worker goroutine per partition and returns
// a Stepper that drives them through the two-phase barrier each Step. The
// workers run until the Stepper's Close is called.
func (e *PersistentWorkerExecution) NewStepper(c *PartitionCoordinator) Stepper {
	s := &persistentWorkerStepper{
		coordinator: c,
		// quit is closed by Close so workers blocked waiting for the next
		// iteration-phase wake-up return instead of leaking.
		quit: make(chan struct{}),
	}
	for partitionIndex, iterator := range c.Iterators {
		go func(partitionIndex int, iterator *StateIterator) {
			workChannel := c.newWorkChannels[partitionIndex]
			for {
				// Iteration-phase wake-up. This is the only point at which a
				// worker is parked between steps, so it is the only receive
				// that needs to be quit-aware.
				var message *IteratorInputMessage
				select {
				case <-s.quit:
					return
				case message = <-workChannel:
				}
				iterator.IteratePending(message)
				s.iterateWaitGroup.Done()

				// Update-phase wake-up. A Step always sends the update wake-up
				// before it returns, so a plain receive is safe here and avoids
				// the extra select overhead.
				iterator.ApplyHistoryUpdate(<-workChannel)
				s.updateWaitGroup.Done()
			}
		}(partitionIndex, iterator)
	}
	return s
}

// persistentWorkerStepper drives the long-lived workers created by
// NewStepper. After every Step each worker is parked on its iteration-phase
// wake-up, so Close can release them all by closing quit.
type persistentWorkerStepper struct {
	coordinator      *PartitionCoordinator
	quit             chan struct{}
	iterateWaitGroup sync.WaitGroup
	updateWaitGroup  sync.WaitGroup
}

// Step advances the coordinator by one tick, waking every persistent worker
// once for the iteration phase and once for the update phase.
func (s *persistentWorkerStepper) Step() {
	c := s.coordinator
	numPartitions := len(c.Iterators)

	c.beginStep()

	// iteration phase: wake every worker, then wait for all to finish
	s.iterateWaitGroup.Add(numPartitions)
	for _, channel := range c.newWorkChannels {
		channel <- c.Shared
	}
	s.iterateWaitGroup.Wait()

	// update phase: wake every worker, then wait for all to finish
	s.updateWaitGroup.Add(numPartitions)
	for _, channel := range c.newWorkChannels {
		channel <- c.Shared
	}
	s.updateWaitGroup.Wait()

	c.advanceTimestepsHistory()
}

// Close releases the persistent workers. Every worker is parked on an
// iteration-phase wake-up between steps, so closing quit returns them all.
func (s *persistentWorkerStepper) Close() { close(s.quit) }

// InlineExecution runs the simulation entirely on the calling goroutine: no
// worker goroutines, no channel handshakes and no WaitGroup barrier. Each step
// runs the iteration phase for every partition and then the update phase for
// every partition, in index order, by calling the iterators directly.
//
// This is the only strategy that synchronises nothing per step, so it is the
// one that reaches serial speed when concurrency buys nothing — most obviously
// a single-partition run, where the default per-step goroutine spawn and
// channel round-trip are pure overhead.
//
// Within-step params_from_upstream edges are supported, but because inline
// execution has no blocking channel handshake to wait on, every upstream
// producer must be ordered before its downstream consumers: the producer's
// staged output is read directly, so it must already have run this step.
// NewStepper validates this up front and panics with a clear message if any
// consumer is ordered before (or at the same index as) one of its upstreams —
// which also catches cycles — rather than silently reading stale values.
// Reorder the partitions so upstreams precede consumers, or use a concurrent
// strategy. Partitions coupled only through state-history reads (which are
// lag-based and need no within-step handshake) are unaffected by the ordering
// rule.
//
// Output is byte-identical to the default strategy: the two phases are still
// applied in order, so the iteration phase observes the previous step's
// committed history exactly as the barrier guarantees, and upstream params
// carry the same current-step producer output the channel broadcast would.
//
// This strategy is stateless and safe to share across coordinators.
type InlineExecution struct{}

// NewStepper validates the partition ordering and returns a Stepper that
// advances the coordinator inline on the calling goroutine. It panics if any
// consumer is not ordered strictly after all of its upstreams (which also
// rejects cycles and self-edges).
func (e *InlineExecution) NewStepper(c *PartitionCoordinator) Stepper {
	// Fail loudly instead of reading stale producer output if any consumer is
	// not ordered strictly after all of its upstreams.
	for consumerIndex, iterator := range c.Iterators {
		for _, upstream := range iterator.ValueChannels.Upstreams {
			if upstream.Upstream >= consumerIndex {
				panic("InlineExecution requires upstreams to be ordered before " +
					"their consumers: partition " + iterator.Partition.Name +
					" reads an upstream that is not earlier in partition order; " +
					"reorder the partitions or use a concurrent execution strategy")
			}
		}
	}
	return &inlineStepper{coordinator: c}
}

// inlineStepper advances the coordinator on the calling goroutine with no
// concurrency, so it holds no resources and Close is a no-op.
type inlineStepper struct {
	coordinator *PartitionCoordinator
}

// Step advances the coordinator by one tick, running the iteration phase for
// every partition and then the update phase for every partition, in index
// order, on the calling goroutine.
func (s *inlineStepper) Step() {
	c := s.coordinator

	c.beginStep()

	// iteration phase for every partition, then update phase for every
	// partition: the same two-phase ordering as the default strategy, so a
	// partition that reads another's history still sees the previous step's
	// committed values. Upstream params are read directly from producers'
	// staged output, which the ordering check guarantees is already set this
	// step.
	for _, iterator := range c.Iterators {
		iterator.IteratePendingInline(c.Shared)
	}
	for _, iterator := range c.Iterators {
		iterator.ApplyHistoryUpdate(c.Shared)
	}

	c.advanceTimestepsHistory()
}

// Close releases the stepper. Inline execution holds no long-lived resources,
// so this does nothing.
func (s *inlineStepper) Close() {}
