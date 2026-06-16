package simulator

import "sync"

// ExecutionStrategy drives a PartitionCoordinator's Run loop.
//
// A nil ExecutionStrategy on a coordinator (or on Implementations) selects the
// default spawn-per-step two-phase execution, which is exactly equivalent to
// repeatedly calling Step until termination. Strategies only ever change how
// Run advances the simulation; single Step semantics are never affected, so
// callers that drive a coordinator one Step at a time (including the test
// harnesses) always observe the default behaviour.
//
// Every strategy must produce byte-identical output to the default for the
// same Settings and Implementations: a strategy is purely an execution-policy
// choice, not a semantic one. This invariant is enforced by the cross-strategy
// equivalence tests.
type ExecutionStrategy interface {
	// Run advances the coordinator from its current state until its
	// TerminationCondition is met.
	Run(c *PartitionCoordinator)
}

// SpawnPerStepExecution is the default execution strategy: each step spawns one
// goroutine per partition for the iteration phase and again for the update
// phase, synchronised by a two-phase barrier. It is the named, explicitly
// selectable form of the behaviour Run uses when no strategy is configured.
//
// This strategy is stateless and safe to share across coordinators.
type SpawnPerStepExecution struct{}

// Run advances the coordinator until termination using the default
// spawn-per-step two-phase execution.
func (e *SpawnPerStepExecution) Run(c *PartitionCoordinator) {
	var wg sync.WaitGroup
	for !c.ReadyToTerminate() {
		c.Step(&wg)
	}
}

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
// the barrier ordering are unchanged; only the goroutine lifetime differs.
//
// This strategy is stateless and safe to share across coordinators; all
// per-run state is created inside Run.
type PersistentWorkerExecution struct{}

// Run spins up one worker goroutine per partition, advances the simulation
// through the two-phase barrier each step, then tears the workers down.
func (e *PersistentWorkerExecution) Run(c *PartitionCoordinator) {
	numPartitions := len(c.Iterators)

	// quit is closed once the run loop has finished so workers blocked waiting
	// for the next wake-up return instead of leaking.
	quit := make(chan struct{})
	var iterateWaitGroup, updateWaitGroup sync.WaitGroup

	for partitionIndex, iterator := range c.Iterators {
		go func(partitionIndex int, iterator *StateIterator) {
			workChannel := c.newWorkChannels[partitionIndex]
			for {
				// Iteration-phase wake-up. This is the only point at which a
				// worker is parked when the run loop finishes, so it is the
				// only receive that needs to be quit-aware.
				var message *IteratorInputMessage
				select {
				case <-quit:
					return
				case message = <-workChannel:
				}
				iterator.IteratePending(message)
				iterateWaitGroup.Done()

				// Update-phase wake-up. The run loop always sends the update
				// wake-up before it can terminate, so a plain receive is safe
				// here and avoids the extra select overhead.
				iterator.ApplyHistoryUpdate(<-workChannel)
				updateWaitGroup.Done()
			}
		}(partitionIndex, iterator)
	}

	for !c.ReadyToTerminate() {
		// update the overall step count and get the next time increment
		c.Shared.TimestepsHistory.CurrentStepNumber += 1
		c.Shared.TimestepsHistory.NextIncrement =
			c.TimestepFunction.NextIncrement(c.Shared.TimestepsHistory)

		// iteration phase: wake every worker, then wait for all to finish
		iterateWaitGroup.Add(numPartitions)
		for _, channel := range c.newWorkChannels {
			channel <- c.Shared
		}
		iterateWaitGroup.Wait()

		// update phase: wake every worker, then wait for all to finish
		updateWaitGroup.Add(numPartitions)
		for _, channel := range c.newWorkChannels {
			channel <- c.Shared
		}
		updateWaitGroup.Wait()

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

	// every worker is now blocked on an iteration-phase wake-up; release them
	close(quit)
}
