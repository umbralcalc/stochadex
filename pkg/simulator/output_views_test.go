package simulator

import (
	"sync"
	"testing"

	"gonum.org/v1/gonum/floats"
)

// viewsGenerator builds a small bank of seeded random walks whose output goes
// through the given condition and function.
func viewsGenerator(
	condition OutputCondition,
	function OutputFunction,
	strategy ExecutionStrategy,
) *ConfigGenerator {
	generator := ensembleBuilder(3, 20)()
	simulation := *generator.GetSimulation()
	simulation.OutputCondition = condition
	simulation.OutputFunction = function
	simulation.ExecutionStrategy = strategy
	generator.SetSimulation(&simulation)
	return generator
}

func runGenerator(generator *ConfigGenerator) {
	NewPartitionCoordinator(generator.GenerateConfigs()).Run()
}

// assertSameRecords fails unless two storages recorded the same rows: the same
// times, and per partition the same values.
func assertSameRecords(t *testing.T, label string, got, want *StateTimeStorage) {
	t.Helper()
	if !floats.Equal(got.GetTimes(), want.GetTimes()) {
		t.Fatalf("%s: times %v, want %v", label, got.GetTimes(), want.GetTimes())
	}
	for _, name := range want.GetNames() {
		gotRows, wantRows := got.GetValues(name), want.GetValues(name)
		if len(gotRows) != len(wantRows) {
			t.Fatalf("%s: %s has %d rows, want %d", label, name, len(gotRows), len(wantRows))
		}
		for i := range wantRows {
			if !floats.Equal(gotRows[i], wantRows[i]) {
				t.Fatalf("%s: %s row %d = %v, want %v", label, name, i, gotRows[i], wantRows[i])
			}
		}
	}
}

func TestOutputViews(t *testing.T) {
	conditions := map[string]func() OutputCondition{
		"all": func() OutputCondition { return &EveryStepOutputCondition{} },
		"walk_1": func() OutputCondition {
			return &OnlyGivenPartitionsOutputCondition{Partitions: map[string]bool{"walk_1": true}}
		},
		"sparse": func() OutputCondition { return &EveryNStepsOutputCondition{N: 4} },
	}
	strategies := map[string]func() ExecutionStrategy{
		"default":           func() ExecutionStrategy { return nil },
		"inline":            func() ExecutionStrategy { return &InlineExecution{} },
		"persistent_worker": func() ExecutionStrategy { return &PersistentWorkerExecution{} },
	}

	for strategyName, strategy := range strategies {
		t.Run("each view records what a single-sink run with its condition records ("+strategyName+")", func(t *testing.T) {
			// One run through three views...
			stores := map[string]*StateTimeStorage{}
			views := make([]OutputView, 0, len(conditions))
			for name, condition := range conditions {
				stores[name] = NewStateTimeStorage()
				views = append(views, OutputView{
					Name: name, Condition: condition(),
					Function: &StateTimeStorageOutputFunction{Store: stores[name]},
				})
			}
			runGenerator(viewsGenerator(
				&EveryStepOutputCondition{}, &OutputViews{Views: views}, strategy()))

			// ...against one independent run per condition, through the classic
			// single condition + single sink path.
			for name, condition := range conditions {
				reference := NewStateTimeStorage()
				runGenerator(viewsGenerator(
					condition(), &StateTimeStorageOutputFunction{Store: reference}, strategy()))
				assertSameRecords(t, "view "+name, stores[name], reference)
			}

			// The views really are different filters of the same run.
			if n := len(stores["all"].GetTimes()); n != 21 {
				t.Errorf("all: %d rows, want 21 (initial state + 20 steps)", n)
			}
			for _, name := range []string{"walk_0", "walk_2"} {
				if rows := stores["walk_1"].GetValues(name); len(rows) != 0 {
					t.Errorf("walk_1 view recorded %d rows for %s", len(rows), name)
				}
			}
			if n := len(stores["sparse"].GetTimes()); n == 0 || n >= 21 {
				t.Errorf("sparse: %d rows, want a strict non-empty subset of 21", n)
			}
		})
	}

	t.Run("every view's sink is configured and finalized once per run", func(t *testing.T) {
		finalizing := &finalizingCountingSink{}
		plain := &countingSink{}
		runGenerator(viewsGenerator(&EveryStepOutputCondition{}, &OutputViews{Views: []OutputView{
			{Name: "finalizing", Condition: &EveryStepOutputCondition{}, Function: finalizing},
			{Name: "plain", Condition: &EveryStepOutputCondition{}, Function: plain},
		}}, nil))
		if finalizing.configures != 1 || plain.configures != 1 {
			t.Errorf("Configure calls: finalizing %d, plain %d, want 1 each",
				finalizing.configures, plain.configures)
		}
		if finalizing.finalizeCalls != 1 {
			t.Errorf("Finalize calls on the finalizing sink: %d, want 1", finalizing.finalizeCalls)
		}
		if finalizing.outputs != 3*21 || plain.outputs != 3*21 {
			t.Errorf("outputs: finalizing %d, plain %d, want 63 each", finalizing.outputs, plain.outputs)
		}
	})

	t.Run("Output called by hand reaches every view, whatever its condition", func(t *testing.T) {
		a, b := &countingSink{}, &countingSink{}
		views := &OutputViews{Views: []OutputView{
			{Name: "a", Condition: &NilOutputCondition{}, Function: a},
			{Name: "b", Condition: &EveryStepOutputCondition{}, Function: b},
		}}
		views.Output("p", []float64{1.0}, 0.5)
		if a.outputs != 1 || b.outputs != 1 {
			t.Errorf("a direct Output should reach every view: a %d, b %d", a.outputs, b.outputs)
		}
	})

	t.Run("views ignore the simulation's single condition", func(t *testing.T) {
		// A coordinator still carries one OutputCondition; with OutputViews it must
		// not gate the views, even when it selects nothing.
		store := NewStateTimeStorage()
		runGenerator(viewsGenerator(&NilOutputCondition{}, &OutputViews{Views: []OutputView{
			{Name: "all", Condition: &EveryStepOutputCondition{},
				Function: &StateTimeStorageOutputFunction{Store: store}},
		}}, nil))
		if n := len(store.GetTimes()); n != 21 {
			t.Errorf("view recorded %d rows, want 21", n)
		}
	})
}

// countingSink counts the calls a sink receives. It does not implement
// FinalizingOutputFunction; finalizingCountingSink adds that.
type countingSink struct {
	mutex      sync.Mutex
	configures int
	outputs    int
}

func (c *countingSink) Configure(*Settings) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.configures++
}

func (c *countingSink) Output(string, []float64, float64) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.outputs++
}

type finalizingCountingSink struct {
	countingSink
	finalizeCalls int
}

func (f *finalizingCountingSink) Finalize() {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.finalizeCalls++
}

var (
	_ FinalizingOutputFunction = (*finalizingCountingSink)(nil)
	_ OutputFunction           = (*countingSink)(nil)
)
