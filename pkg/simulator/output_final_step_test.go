package simulator

import (
	"strings"
	"testing"

	"gonum.org/v1/gonum/floats"
)

// lastRows is each partition's last recorded row, and the last recorded time.
func lastRows(store *StateTimeStorage) (map[string][]float64, float64) {
	rows := map[string][]float64{}
	for _, name := range store.GetNames() {
		values := store.GetValues(name)
		rows[name] = values[len(values)-1]
	}
	times := store.GetTimes()
	return rows, times[len(times)-1]
}

// assertFinalStep fails unless final recorded exactly one row per partition
// named in want: the last row a full run recorded, at the full run's last time.
func assertFinalStep(t *testing.T, final, full *StateTimeStorage, want []string) {
	t.Helper()
	wantRows, wantTime := lastRows(full)
	if times := final.GetTimes(); len(times) != 1 || times[0] != wantTime {
		t.Fatalf("final_step recorded times %v, want [%v]", times, wantTime)
	}
	wanted := map[string]bool{}
	for _, name := range want {
		wanted[name] = true
	}
	for _, name := range final.GetNames() {
		if rows := final.GetValues(name); !wanted[name] && len(rows) != 0 {
			t.Fatalf("final_step recorded %d rows of %s, which it should not write", len(rows), name)
		}
	}
	for _, name := range want {
		rows := final.GetValues(name)
		if len(rows) != 1 || !floats.Equal(rows[0], wantRows[name]) {
			t.Fatalf("%s: final_step recorded %v, want one row %v", name, rows, wantRows[name])
		}
	}
}

func TestFinalStepOutputCondition(t *testing.T) {
	strategies := map[string]func() ExecutionStrategy{
		"default":           func() ExecutionStrategy { return nil },
		"inline":            func() ExecutionStrategy { return &InlineExecution{} },
		"persistent_worker": func() ExecutionStrategy { return &PersistentWorkerExecution{} },
	}
	all := []string{"walk_0", "walk_1", "walk_2"}
	for strategyName, strategy := range strategies {
		t.Run("as the run's condition it writes only the last step ("+strategyName+")", func(t *testing.T) {
			full, final := NewStateTimeStorage(), NewStateTimeStorage()
			runGenerator(viewsGenerator(&EveryStepOutputCondition{},
				&StateTimeStorageOutputFunction{Store: full}, strategy()))
			runGenerator(viewsGenerator(&FinalStepOutputCondition{},
				&StateTimeStorageOutputFunction{Store: final}, strategy()))
			assertFinalStep(t, final, full, all)
		})
		t.Run("as a view beside an every-step view ("+strategyName+")", func(t *testing.T) {
			full, final := NewStateTimeStorage(), NewStateTimeStorage()
			runGenerator(viewsGenerator(&EveryStepOutputCondition{}, &OutputViews{Views: []OutputView{
				{Name: "full", Condition: &EveryStepOutputCondition{},
					Function: &StateTimeStorageOutputFunction{Store: full}},
				{Name: "final", Condition: &FinalStepOutputCondition{
					Partitions: map[string]bool{"walk_1": true}},
					Function: &StateTimeStorageOutputFunction{Store: final}},
			}}, strategy()))
			if n := len(full.GetTimes()); n != 21 {
				t.Fatalf("the every-step view recorded %d rows, want 21", n)
			}
			assertFinalStep(t, final, full, []string{"walk_1"})
		})
	}

	t.Run("a single final_step view writes only the last step", func(t *testing.T) {
		full, final := NewStateTimeStorage(), NewStateTimeStorage()
		runGenerator(viewsGenerator(&EveryStepOutputCondition{},
			&StateTimeStorageOutputFunction{Store: full}, nil))
		runGenerator(viewsGenerator(&EveryStepOutputCondition{}, &OutputViews{Views: []OutputView{
			{Name: "final", Condition: &FinalStepOutputCondition{},
				Function: &StateTimeStorageOutputFunction{Store: final}},
		}}, nil))
		assertFinalStep(t, final, full, all)
	})

	t.Run("it follows a termination condition that is not a step count", func(t *testing.T) {
		build := func(condition OutputCondition, store *StateTimeStorage) *ConfigGenerator {
			generator := viewsGenerator(condition, &StateTimeStorageOutputFunction{Store: store}, nil)
			simulation := *generator.GetSimulation()
			simulation.TerminationCondition = &TimeElapsedTerminationCondition{MaxTimeElapsed: 1.25}
			generator.SetSimulation(&simulation)
			return generator
		}
		full, final := NewStateTimeStorage(), NewStateTimeStorage()
		runGenerator(build(&EveryStepOutputCondition{}, full))
		runGenerator(build(&FinalStepOutputCondition{}, final))
		assertFinalStep(t, final, full, all)
	})

	t.Run("a condition reused for a second run holds nothing from the first", func(t *testing.T) {
		condition := &FinalStepOutputCondition{}
		first, second := NewStateTimeStorage(), NewStateTimeStorage()
		runGenerator(viewsGenerator(condition, &StateTimeStorageOutputFunction{Store: first}, nil))
		runGenerator(viewsGenerator(condition, &StateTimeStorageOutputFunction{Store: second}, nil))
		assertSameRecords(t, "second run", second, first)
	})

	t.Run("its sink is finalized once, after the held states are written", func(t *testing.T) {
		sink := &finalizingCountingSink{}
		runGenerator(viewsGenerator(&FinalStepOutputCondition{}, sink, nil))
		if sink.outputs != 3 || sink.finalizeCalls != 1 {
			t.Errorf("outputs %d, finalize calls %d; want 3 and 1", sink.outputs, sink.finalizeCalls)
		}
	})
}

func TestFinalStepOutputConditionFromData(t *testing.T) {
	t.Run("it writes every partition when partitions is not given", func(t *testing.T) {
		condition, err := ResolveOutputCondition(ComponentSpec{Type: "final_step"})
		if err != nil {
			t.Fatal(err)
		}
		final, ok := condition.(*FinalStepOutputCondition)
		if !ok || final.Partitions != nil {
			t.Fatalf("got %#v, want a final_step condition over every partition", condition)
		}
	})
	t.Run("partitions names the only ones it writes", func(t *testing.T) {
		condition, err := ResolveOutputCondition(ComponentSpec{Type: "final_step",
			Fields: map[string]interface{}{"partitions": []interface{}{"walk_1", "walk_2"}}})
		if err != nil {
			t.Fatal(err)
		}
		full, final := NewStateTimeStorage(), NewStateTimeStorage()
		runGenerator(viewsGenerator(&EveryStepOutputCondition{},
			&StateTimeStorageOutputFunction{Store: full}, nil))
		runGenerator(viewsGenerator(condition, &StateTimeStorageOutputFunction{Store: final}, nil))
		assertFinalStep(t, final, full, []string{"walk_1", "walk_2"})
	})
	t.Run("it refuses partitions that are not a list of names", func(t *testing.T) {
		_, err := ResolveOutputCondition(ComponentSpec{Type: "final_step",
			Fields: map[string]interface{}{"partitions": "walk_1"}})
		if err == nil || !strings.Contains(err.Error(), `field "partitions" must be a list`) {
			t.Fatalf("got %v, want an error that partitions must be a list", err)
		}
	})
	t.Run("as a per-step filter it selects no step", func(t *testing.T) {
		// Its output is written when the run's output is finalized, never by a
		// step: a sink gated by IsOutputStep alone would write nothing.
		if (&FinalStepOutputCondition{}).IsOutputStep("walk_0", []float64{1}, nil) {
			t.Fatal("IsOutputStep selected a step")
		}
	})
}
