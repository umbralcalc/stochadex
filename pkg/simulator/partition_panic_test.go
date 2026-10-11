package simulator

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// panicsAtStepIteration wraps an iteration, failing at step failAt: it panics
// with value in Iterate, or, when wrongWidth is set, returns one value too many
// so the partition's history update panics instead.
type panicsAtStepIteration struct {
	Iteration
	failAt     int
	value      any
	wrongWidth bool
	step       int
}

func (p *panicsAtStepIteration) Configure(partitionIndex int, settings *Settings) {
	p.Iteration.Configure(partitionIndex, settings)
	p.step = 0
}

func (p *panicsAtStepIteration) Iterate(
	params *Params,
	partitionIndex int,
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) []float64 {
	p.step++
	values := p.Iteration.Iterate(params, partitionIndex, stateHistories, timestepsHistory)
	if p.step != p.failAt {
		return values
	}
	if p.wrongWidth {
		return append(values, 0)
	}
	panic(p.value)
}

// panickingTimestepFunction panics on its third call, after steps in which
// partitions have run.
type panickingTimestepFunction struct{ calls int }

func (p *panickingTimestepFunction) NextIncrement(*CumulativeTimestepsHistory) float64 {
	p.calls++
	if p.calls == 3 {
		panic("timestep function failed")
	}
	return 1.0
}

// allStrategies is every execution strategy, the default (nil) included.
func allStrategies() map[string]ExecutionStrategy {
	strategies := namedStrategies()
	strategies["default"] = nil
	strategies["inline"] = &InlineExecution{}
	return strategies
}

// panicFrom runs drive and returns the value it panicked with, nil if none.
func panicFrom(drive func()) (recovered any) {
	defer func() { recovered = recover() }()
	drive()
	return nil
}

// waitForGoroutines fails unless the goroutine count returns to before within
// a second, i.e. every worker of a failed run finished or stopped.
func waitForGoroutines(t *testing.T, before int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			t.Fatalf("goroutines leaked: %d before, %d after", before, runtime.NumGoroutine())
		}
		time.Sleep(time.Millisecond)
	}
}

// failingChain is the chain topology with partition failing wrapped to fail at
// step 5. In the chain, partition_1 is read within the step by partition_2.
func failingChain(failing int, value any, wrongWidth bool) (*Settings, func() []Iteration) {
	settings, makeIterations := chainSettings()
	return settings, func() []Iteration {
		iterations := makeIterations()
		iterations[failing] = &panicsAtStepIteration{
			Iteration: iterations[failing], failAt: 5, value: value, wrongWidth: wrongWidth,
		}
		return iterations
	}
}

func TestPartitionPanic(t *testing.T) {
	cases := map[string]struct {
		failing    int
		wrongWidth bool
		check      func(t *testing.T, value any)
	}{
		"an upstream panicking in Iterate": {failing: 1, check: func(t *testing.T, value any) {
			if value != "boom" {
				t.Errorf("want the value boom, got %v", value)
			}
		}},
		"a consumer panicking in Iterate": {failing: 2, check: func(t *testing.T, value any) {
			if value != "boom" {
				t.Errorf("want the value boom, got %v", value)
			}
		}},
		"a partition whose history update panics": {failing: 1, wrongWidth: true,
			check: func(t *testing.T, value any) {
				if !errors.Is(value.(error), mat.ErrRowLength) {
					t.Errorf("want mat.ErrRowLength, got %v", value)
				}
			}},
	}
	drivers := map[string]func(*Settings, func() []Iteration, ExecutionStrategy){
		"Run": func(settings *Settings, makeIterations func() []Iteration, strategy ExecutionStrategy) {
			runStrategyConfig(settings, makeIterations, 20, strategy)
		},
		"a Stepper loop deferring NamePartitionPanic": func(
			settings *Settings, makeIterations func() []Iteration, strategy ExecutionStrategy,
		) {
			coordinator := newStrategyCoordinator(settings, makeIterations, strategy)
			stepper := coordinator.NewStepper()
			defer stepper.Close()
			defer coordinator.NamePartitionPanic()
			for !coordinator.ReadyToTerminate() {
				stepper.Step()
			}
		},
	}
	for caseName, c := range cases {
		for strategyName, strategy := range allStrategies() {
			for driverName, drive := range drivers {
				t.Run(caseName+" under "+strategyName+" via "+driverName, func(t *testing.T) {
					before := runtime.NumGoroutine()
					settings, makeIterations := failingChain(c.failing, "boom", c.wrongWidth)
					recovered := panicFrom(func() { drive(settings, makeIterations, strategy) })
					failure, ok := recovered.(*PartitionPanic)
					if !ok {
						t.Fatalf("want a *PartitionPanic, got %T: %v", recovered, recovered)
					}
					want := settings.Iterations[c.failing].Name
					if failure.Partition != want {
						t.Errorf("want partition %s, got %s", want, failure.Partition)
					}
					if !strings.HasPrefix(failure.Error(), `partition "`+want+`" panicked: `) {
						t.Errorf("unexpected message %q", failure.Error())
					}
					if len(failure.Stack) == 0 {
						t.Error("want the panicking goroutine's stack")
					}
					c.check(t, failure.Value)
					waitForGoroutines(t, before)
				})
			}
		}
	}

	t.Run("a failed iteration commits nothing and stops its consumers", func(t *testing.T) {
		// partition_1 fails at step 5; partition_2 reads it within the step,
		// but would iterate happily on any params, so only the failure
		// stopping it keeps it from running on values partition_1 never sent.
		settings, makeChain := chainSettings()
		makeIterations := func() []Iteration {
			iterations := makeChain()
			iterations[2] = &doublingProcessIteration{}
			return iterations
		}
		reference := stepStrategyConfig(settings, makeIterations, 4, nil)
		for strategyName, strategy := range allStrategies() {
			iterations := makeIterations()
			iterations[1] = &panicsAtStepIteration{
				Iteration: iterations[1], failAt: 5, value: "boom"}
			for index, iteration := range iterations {
				iteration.Configure(index, settings)
			}
			store := NewStateTimeStorage()
			coordinator := NewPartitionCoordinator(settings, &Implementations{
				Iterations:           iterations,
				OutputCondition:      &EveryStepOutputCondition{},
				OutputFunction:       &StateTimeStorageOutputFunction{Store: store},
				TerminationCondition: &NumberOfStepsTerminationCondition{MaxNumberOfSteps: 20},
				TimestepFunction:     &ConstantTimestepFunction{Stepsize: 1.0},
				ExecutionStrategy:    strategy,
			})
			stepper := coordinator.NewStepper()
			if panicFrom(func() {
				defer coordinator.NamePartitionPanic()
				for !coordinator.ReadyToTerminate() {
					stepper.Step()
				}
			}) == nil {
				t.Fatalf("%s: want the run to panic", strategyName)
			}
			stepper.Close()
			if got := coordinator.Shared.TimestepsHistory.Values.AtVec(0); got != 4.0 {
				t.Errorf("%s: want the time left at 4, got %v", strategyName, got)
			}
			for index, name := range []string{"partition_0", "partition_1", "partition_2"} {
				want := reference.GetValues(name)
				got := coordinator.Shared.StateHistories[index].Values.RawRowView(0)
				if !floats.Equal(want[len(want)-1], got) {
					t.Errorf("%s: %s's history holds %v, want step 4's %v",
						strategyName, name, got, want[len(want)-1])
				}
			}
			// The consumer never ran on the failed step, so it output no row
			// for it: its initial state and steps 1 to 4 only.
			if got := len(store.GetValues("partition_2")); got != 5 {
				t.Errorf("%s: want 5 rows of partition_2, got %d", strategyName, got)
			}
		}
	})

	t.Run("a bare inline Step raises the partition's own panic", func(t *testing.T) {
		// Inline Step defers nothing, to stay at serial speed, so without
		// NamePartitionPanic deferred its panic is the partition's own; the
		// concurrent strategies' Step names the partition either way.
		for strategyName, strategy := range allStrategies() {
			settings, makeIterations := failingChain(1, "boom", false)
			coordinator := newStrategyCoordinator(settings, makeIterations, strategy)
			stepper := coordinator.NewStepper()
			recovered := panicFrom(func() {
				for !coordinator.ReadyToTerminate() {
					stepper.Step()
				}
			})
			stepper.Close()
			_, named := recovered.(*PartitionPanic)
			if wantNamed := strategyName != "inline"; named != wantNamed || recovered == nil {
				t.Errorf("%s: got %T %v", strategyName, recovered, recovered)
			}
		}
	})

	t.Run("a partition exiting its goroutine is not taken for a panic", func(t *testing.T) {
		// runtime.Goexit (t.FailNow in an iteration, say) runs the deferred
		// NamePartitionPanic with a partition running but nothing to recover.
		settings, makeIterations := chainSettings()
		coordinator := newStrategyCoordinator(settings, func() []Iteration {
			iterations := makeIterations()
			iterations[1] = &goexitIteration{Iteration: iterations[1]}
			return iterations
		}, &InlineExecution{})
		done := make(chan any)
		go func() {
			defer func() { done <- recover() }()
			coordinator.Run()
		}()
		if recovered := <-done; recovered != nil {
			t.Errorf("want the goroutine to exit cleanly, got %v", recovered)
		}
	})

	t.Run("runs that do not panic are unchanged", func(t *testing.T) {
		settings, makeIterations := chainSettings()
		reference := runStrategyConfig(settings, makeIterations, 20, nil)
		for strategyName, strategy := range allStrategies() {
			got := runStrategyConfig(settings, func() []Iteration {
				iterations := makeIterations()
				iterations[1] = &panicsAtStepIteration{Iteration: iterations[1], failAt: 0}
				return iterations
			}, 20, strategy)
			assertStoresEqual(t, reference, got, strategyName)
		}
	})

	t.Run("a panic outside a partition's work propagates as it was", func(t *testing.T) {
		for strategyName, strategy := range allStrategies() {
			settings, makeIterations := chainSettings()
			iterations := makeIterations()
			for index, iteration := range iterations {
				iteration.Configure(index, settings)
			}
			coordinator := NewPartitionCoordinator(settings, &Implementations{
				Iterations:           iterations,
				TerminationCondition: &NumberOfStepsTerminationCondition{MaxNumberOfSteps: 5},
				TimestepFunction:     &panickingTimestepFunction{},
				ExecutionStrategy:    strategy,
			})
			if recovered := panicFrom(coordinator.Run); recovered != "timestep function failed" {
				t.Errorf("%s: want the timestep function's own panic, got %v", strategyName, recovered)
			}
		}
	})

	t.Run("errors.As and errors.Is see through to an error value", func(t *testing.T) {
		sentinel := errors.New("sentinel")
		settings, makeIterations := failingChain(0, sentinel, false)
		recovered := panicFrom(func() { runStrategyConfig(settings, makeIterations, 20, nil) })
		err, ok := recovered.(error)
		if !ok || !errors.Is(err, sentinel) {
			t.Fatalf("want an error wrapping the sentinel, got %v", recovered)
		}
		var failure *PartitionPanic
		if !errors.As(err, &failure) || failure.Partition != "partition_0" {
			t.Errorf("want partition_0's PartitionPanic, got %v", err)
		}
		if (&PartitionPanic{Value: "not an error"}).Unwrap() != nil {
			t.Error("want no unwrapped error for a non-error value")
		}
	})

	t.Run("a nested failure names both partitions", func(t *testing.T) {
		// A failure re-raised by an inner coordinator inside an outer
		// partition's Iterate is wrapped again with the outer partition.
		settings, makeIterations := failingChain(0, &PartitionPanic{
			Partition: "inner", Value: "boom"}, false)
		recovered := panicFrom(func() {
			runStrategyConfig(settings, makeIterations, 20, &PersistentWorkerExecution{})
		})
		const want = `partition "partition_0" panicked: partition "inner" panicked: boom`
		if err, ok := recovered.(error); !ok || err.Error() != want {
			t.Errorf("want %q, got %v", want, recovered)
		}
	})
}

// newStrategyCoordinator configures fresh iterations and builds a coordinator
// for them under strategy, running for at most 20 steps.
func newStrategyCoordinator(
	settings *Settings,
	makeIterations func() []Iteration,
	strategy ExecutionStrategy,
) *PartitionCoordinator {
	iterations := makeIterations()
	for index, iteration := range iterations {
		iteration.Configure(index, settings)
	}
	return NewPartitionCoordinator(settings, &Implementations{
		Iterations:           iterations,
		TerminationCondition: &NumberOfStepsTerminationCondition{MaxNumberOfSteps: 20},
		TimestepFunction:     &ConstantTimestepFunction{Stepsize: 1.0},
		ExecutionStrategy:    strategy,
	})
}

// goexitIteration ends the goroutine running it on its first step.
type goexitIteration struct{ Iteration }

func (g *goexitIteration) Iterate(
	*Params, int, []*StateHistory, *CumulativeTimestepsHistory,
) []float64 {
	runtime.Goexit()
	return nil
}
