package simulator

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// finalizeCounter records how often it is finalized.
type finalizeCounter struct {
	NilOutputFunction
	finalized int
}

func (c *finalizeCounter) Finalize() { c.finalized++ }

// delegatingStrategy is an execution strategy from outside the package whose
// stepper is a built-in one, so both would finalize if finalizing were not
// once per coordinator.
type delegatingStrategy struct{}

func (d *delegatingStrategy) NewStepper(c *PartitionCoordinator) Stepper {
	return (&InlineExecution{}).NewStepper(c)
}

// ownStepperStrategy is an execution strategy from outside the package with a
// stepper of its own, which knows nothing about finalizing.
type ownStepperStrategy struct{}

type ownStepper struct {
	coordinator *PartitionCoordinator
	waitGroup   sync.WaitGroup
}

func (o *ownStepper) Step()  { o.coordinator.Step(&o.waitGroup) }
func (o *ownStepper) Close() {}

func (o *ownStepperStrategy) NewStepper(c *PartitionCoordinator) Stepper {
	return &ownStepper{coordinator: c}
}

func outputStrategies() map[string]ExecutionStrategy {
	return map[string]ExecutionStrategy{
		"default":           nil,
		"spawn_per_step":    &SpawnPerStepExecution{},
		"persistent_worker": &PersistentWorkerExecution{},
		"inline":            &InlineExecution{},
		"custom_delegating": &delegatingStrategy{},
		"custom_own":        &ownStepperStrategy{},
	}
}

func TestStepperCloseFinalizesOutput(t *testing.T) {
	for name, strategy := range outputStrategies() {
		t.Run(name+": Run finalizes the output exactly once", func(t *testing.T) {
			sink := &finalizeCounter{}
			implementations := injectImplementations(3, strategy, nil)
			implementations.OutputFunction = sink
			NewPartitionCoordinator(injectSettings(), implementations).Run()
			if sink.finalized != 1 {
				t.Errorf("finalized %d times, want 1", sink.finalized)
			}
		})
		t.Run(name+": a stepped run is finalized by Close, exactly once", func(t *testing.T) {
			sink := &finalizeCounter{}
			implementations := injectImplementations(3, strategy, nil)
			implementations.OutputFunction = sink
			coordinator := NewPartitionCoordinator(injectSettings(), implementations)
			stepper := coordinator.NewStepper()
			for !coordinator.ReadyToTerminate() {
				stepper.Step()
			}
			if sink.finalized != 0 {
				t.Fatalf("finalized %d times before Close", sink.finalized)
			}
			stepper.Close()
			if sink.finalized != 1 {
				t.Errorf("finalized %d times after Close, want 1", sink.finalized)
			}
		})
	}
}

func TestSingleOutputViewIsUnwrapped(t *testing.T) {
	t.Run("one view: the iterators call its sink and condition directly", func(t *testing.T) {
		sink, condition := &finalizeCounter{}, &EveryNStepsOutputCondition{N: 2}
		implementations := injectImplementations(4, nil, nil)
		implementations.OutputFunction = &OutputViews{Views: []OutputView{
			{Name: "only", Condition: condition, Function: sink}}}
		coordinator := NewPartitionCoordinator(injectSettings(), implementations)
		for _, iterator := range coordinator.Iterators {
			if iterator.OutputFunction != OutputFunction(sink) ||
				iterator.OutputCondition != OutputCondition(condition) {
				t.Fatalf("iterator %s outputs through %T / %T, want the view's own sink and condition",
					iterator.Partition.Name, iterator.OutputFunction, iterator.OutputCondition)
			}
		}
		coordinator.Run()
		if sink.finalized != 1 {
			t.Errorf("the view's sink was finalized %d times, want 1", sink.finalized)
		}
	})

	t.Run("one view writes exactly what the bare sink and condition write", func(t *testing.T) {
		run := func(wrap bool) *StateTimeStorage {
			store := NewStateTimeStorage()
			implementations := injectImplementations(6, &InlineExecution{}, store)
			implementations.OutputCondition = &EveryNStepsOutputCondition{N: 2}
			if wrap {
				implementations.OutputFunction = &OutputViews{Views: []OutputView{{
					Name: "only", Condition: implementations.OutputCondition,
					Function: implementations.OutputFunction}}}
				implementations.OutputCondition = &EveryStepOutputCondition{}
			}
			NewPartitionCoordinator(injectSettings(), implementations).Run()
			return store
		}
		bare, wrapped := run(false), run(true)
		if len(bare.GetTimes()) != 4 {
			t.Fatalf("bare run recorded %d times, want 4 (every 2nd of 6 steps plus the start)",
				len(bare.GetTimes()))
		}
		assertStoresEqual(t, bare, wrapped, "single view vs bare sink")
	})

	t.Run("several views stay a tee", func(t *testing.T) {
		views := &OutputViews{Views: []OutputView{
			{Name: "a", Condition: &EveryStepOutputCondition{}, Function: &NilOutputFunction{}},
			{Name: "b", Condition: &EveryStepOutputCondition{}, Function: &NilOutputFunction{}}}}
		implementations := injectImplementations(2, nil, nil)
		implementations.OutputFunction = views
		coordinator := NewPartitionCoordinator(injectSettings(), implementations)
		if coordinator.Iterators[0].OutputFunction != OutputFunction(views) {
			t.Errorf("two views should reach the iterators as OutputViews, got %T",
				coordinator.Iterators[0].OutputFunction)
		}
	})
}

// panickedResourceError runs f and returns the *ResourceError it panicked
// with, or nil if it did not panic with one.
func panickedResourceError(f func()) (resourceErr *ResourceError) {
	defer func() {
		if recovered := recover(); recovered != nil {
			resourceErr, _ = recovered.(*ResourceError)
		}
	}()
	f()
	return nil
}

func TestJsonLogBuffersARun(t *testing.T) {
	const entries = 3000 // ~200 KB, several buffers' worth
	write := func(path string, configure bool) *JsonLogOutputFunction {
		sink := NewJsonLogOutputFunction(path)
		if configure {
			sink.Configure(nil)
		}
		for i := range entries {
			sink.Output(fmt.Sprintf("p%d", i%3), []float64{float64(i), -0.5, 1e-9}, float64(i)/7)
		}
		return sink
	}

	t.Run("a run's log is written in blocks and is exactly the unbuffered log once finalized", func(t *testing.T) {
		dir := t.TempDir()
		// Reference: the same entries written by hand, unbuffered.
		write(filepath.Join(dir, "reference.log"), false)
		reference, err := os.ReadFile(filepath.Join(dir, "reference.log"))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "run.log")
		sink := write(path, true)
		before, _ := os.ReadFile(path)
		if len(before) >= len(reference) || len(before)%jsonLogBufferSize != 0 {
			t.Errorf("before Finalize the log held %d of %d bytes; want whole %d-byte blocks, not all of it",
				len(before), len(reference), jsonLogBufferSize)
		}
		sink.Finalize()
		after, _ := os.ReadFile(path)
		if string(after) != string(reference) {
			t.Fatalf("finalized log differs from the unbuffered log (%d vs %d bytes)", len(after), len(reference))
		}
	})

	t.Run("a sink driven by hand writes each entry immediately", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "hand.log")
		sink := NewJsonLogOutputFunction(path)
		sink.Output("p", []float64{1}, 0)
		if entries := readJsonLog(t, path); len(entries) != 1 {
			t.Errorf("an unconfigured sink should write straight to the file, got %d entries", len(entries))
		}
	})

	t.Run("a failed write is a resource error, buffered or not", func(t *testing.T) {
		// Closing the file under the sink makes every later write fail.
		buffered := NewJsonLogOutputFunction(filepath.Join(t.TempDir(), "buffered.log"))
		buffered.Configure(nil)
		buffered.Output("p", []float64{1}, 0)
		buffered.file.Close()
		if err := panickedResourceError(buffered.Finalize); err == nil {
			t.Error("a failed flush in Finalize should panic with a ResourceError")
		}
		byHand := NewJsonLogOutputFunction(filepath.Join(t.TempDir(), "hand.log"))
		byHand.Output("p", []float64{1}, 0)
		byHand.file.Close()
		if err := panickedResourceError(func() { byHand.Output("p", []float64{2}, 1) }); err == nil {
			t.Error("a failed write should panic with a ResourceError")
		}
	})

	t.Run("a re-entrant run of fixed steps leaves a complete log", func(t *testing.T) {
		// ReentrantSimulation steps a run and closes its stepper rather than
		// calling Run, so this is the path Close's finalize exists for.
		path := filepath.Join(t.TempDir(), "nested.log")
		implementations := injectImplementations(100, &InlineExecution{}, nil)
		implementations.OutputFunction = NewJsonLogOutputFunction(path)
		simulation := NewReentrantSimulation(injectSettings(), implementations)
		simulation.Run(ReentrantRun{Steps: 5})
		if n := len(readJsonLog(t, path)); n != 12 {
			t.Fatalf("after one run the log has %d entries, want 12 (2 partitions x 6 rows)", n)
		}
		simulation.Run(ReentrantRun{Steps: 5})
		if n := len(readJsonLog(t, path)); n != 24 {
			t.Errorf("after a second run the log has %d entries, want 24 (runs append)", n)
		}
	})
}

func TestOutputPathAllocations(t *testing.T) {
	t.Run("routing a step through output views allocates nothing", func(t *testing.T) {
		views := &OutputViews{Views: []OutputView{
			{Name: "a", Condition: &EveryStepOutputCondition{}, Function: &NilOutputFunction{}},
			{Name: "b", Condition: &EveryNStepsOutputCondition{N: 2}, Function: &NilOutputFunction{}}}}
		history := &CumulativeTimestepsHistory{CurrentStepNumber: 4}
		state := []float64{1, 2}
		if allocs := testing.AllocsPerRun(1000, func() {
			views.OutputStep("p", state, history, 1.0)
		}); allocs != 0 {
			t.Errorf("OutputViews.OutputStep allocated %v times per output, want 0", allocs)
		}
	})

	t.Run("a single view adds no allocations per step over the bare sink", func(t *testing.T) {
		perStep := func(wrap bool) float64 {
			implementations := injectImplementations(1<<30, &InlineExecution{}, nil)
			implementations.OutputFunction = &NilOutputFunction{}
			if wrap {
				implementations.OutputFunction = &OutputViews{Views: []OutputView{{
					Name: "only", Condition: &EveryStepOutputCondition{}, Function: &NilOutputFunction{}}}}
			}
			stepper := NewPartitionCoordinator(injectSettings(), implementations).NewStepper()
			defer stepper.Close()
			return testing.AllocsPerRun(1000, stepper.Step)
		}
		if bare, wrapped := perStep(false), perStep(true); wrapped != bare {
			t.Errorf("a step allocates %v times through a single view, %v with the bare sink", wrapped, bare)
		}
	})

	t.Run("a buffered json_log entry allocates only for its encoding", func(t *testing.T) {
		sink := NewJsonLogOutputFunction(filepath.Join(t.TempDir(), "allocs.log"))
		sink.Configure(nil)
		defer sink.Finalize()
		state := []float64{1.5, -2, 3e-7}
		if allocs := testing.AllocsPerRun(1000, func() {
			sink.Output("partition", state, 12.5)
		}); allocs > 2 {
			t.Errorf("a json_log entry allocated %v times, want at most 2 (json.Marshal)", allocs)
		}
	})
}

// BenchmarkJsonLogRun is one run of the json_log sink alone: configure, 2000
// entries, finalize.
func BenchmarkJsonLogRun(b *testing.B) {
	sink := NewJsonLogOutputFunction(filepath.Join(b.TempDir(), "run.log"))
	state := []float64{0.1234, -1.5, 2.25, 3.5}
	b.ReportAllocs()
	for b.Loop() {
		sink.Configure(nil)
		for i := range 2000 {
			sink.Output("w", state, float64(i))
		}
		sink.Finalize()
	}
}

// BenchmarkSinglePartitionInlineView is BenchmarkSinglePartitionInline's model
// with its output through a single OutputViews view, the shape every config
// run has: it should cost the same as the bare sink.
func BenchmarkSinglePartitionInlineView(b *testing.B) {
	implementations := injectImplementations(1<<30, &InlineExecution{}, nil)
	implementations.OutputFunction = &OutputViews{Views: []OutputView{{
		Name: "only", Condition: &EveryStepOutputCondition{}, Function: &NilOutputFunction{}}}}
	stepper := NewPartitionCoordinator(injectSettings(), implementations).NewStepper()
	defer stepper.Close()
	b.ReportAllocs()
	for b.Loop() {
		stepper.Step()
	}
}
