package simulator

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeRun writes entries through sink as one configured, finalized run.
func writeRun(sink *JsonLogOutputFunction, entries int) {
	sink.Configure(nil)
	for i := range entries {
		sink.Output("p", []float64{float64(i), 0.5}, float64(i))
	}
	sink.Finalize()
}

func fileContents(t *testing.T, path string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data), true
}

func TestStagedJsonLog(t *testing.T) {
	// The reference is the same run through an unstaged sink.
	reference := func(t *testing.T, runs int) string {
		path := filepath.Join(t.TempDir(), "reference.log")
		sink := NewJsonLogOutputFunction(path)
		for range runs {
			writeRun(sink, 2000)
		}
		contents, _ := fileContents(t, path)
		return contents
	}

	t.Run("nothing reaches the path until Commit, which publishes the unstaged log", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "run.log")
		writeFile(t, path, "an earlier run's log\n")
		sink := NewJsonLogOutputFunction(path)
		sink.Stage()
		writeRun(sink, 2000)
		if contents, _ := fileContents(t, path); contents != "an earlier run's log\n" {
			t.Fatalf("before Commit the path held %d bytes, want the earlier log untouched", len(contents))
		}
		if err := sink.Commit(); err != nil {
			t.Fatal(err)
		}
		if contents, _ := fileContents(t, path); contents != reference(t, 1) {
			t.Error("the committed log differs from the unstaged log")
		}
		if _, exists := fileContents(t, path+partialSuffix); exists {
			t.Error("Commit should leave no partial file")
		}
	})

	t.Run("runs finalized several times are committed once, appended, as a nested run's are", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nested.log")
		sink := NewJsonLogOutputFunction(path)
		sink.Stage()
		for range 3 {
			writeRun(sink, 2000)
		}
		if _, exists := fileContents(t, path); exists {
			t.Fatal("a nested run's log reached its path before the outer run committed")
		}
		if err := sink.Commit(); err != nil {
			t.Fatal(err)
		}
		if contents, _ := fileContents(t, path); contents != reference(t, 3) {
			t.Error("the committed log differs from the three runs written unstaged")
		}
	})

	t.Run("Abort leaves nothing, and the earlier log intact", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "run.log")
		writeFile(t, path, "an earlier run's log\n")
		sink := NewJsonLogOutputFunction(path)
		sink.Stage()
		sink.Configure(nil)
		sink.Output("p", []float64{1}, 0) // still buffered, the file open
		sink.Abort()
		if contents, _ := fileContents(t, path); contents != "an earlier run's log\n" {
			t.Error("Abort changed the log at the path")
		}
		if _, exists := fileContents(t, path+partialSuffix); exists {
			t.Error("Abort should remove the partial file")
		}
	})

	t.Run("a sink that never ran commits and aborts nothing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "run.log")
		writeFile(t, path, "kept\n")
		sink := NewJsonLogOutputFunction(path)
		sink.Stage()
		sink.Abort()
		if err := sink.Commit(); err != nil {
			t.Fatal(err)
		}
		if contents, _ := fileContents(t, path); contents != "kept\n" {
			t.Error("a sink that never ran changed the path")
		}
	})

	t.Run("a sink committed and run again truncates its partial file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "run.log")
		sink := NewJsonLogOutputFunction(path)
		sink.Stage()
		writeRun(sink, 2000)
		if err := sink.Commit(); err != nil {
			t.Fatal(err)
		}
		if err := sink.Commit(); err != nil {
			t.Errorf("a repeated Commit should do nothing, got %v", err)
		}
		writeRun(sink, 2000)
		if err := sink.Commit(); err != nil {
			t.Fatal(err)
		}
		if contents, _ := fileContents(t, path); contents != reference(t, 1) {
			t.Error("a second committed run should replace the first, not append to it")
		}
	})

	t.Run("Commit without Finalize flushes first", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "run.log")
		sink := NewJsonLogOutputFunction(path)
		sink.Stage()
		sink.Configure(nil)
		for i := range 2000 {
			sink.Output("p", []float64{float64(i), 0.5}, float64(i))
		}
		if err := sink.Commit(); err != nil {
			t.Fatal(err)
		}
		if contents, _ := fileContents(t, path); contents != reference(t, 1) {
			t.Error("Commit lost buffered entries")
		}
	})

	t.Run("a Commit that cannot publish is a resource error", func(t *testing.T) {
		// A non-empty directory at the path cannot be replaced by a file.
		path := filepath.Join(t.TempDir(), "taken")
		writeFile(t, filepath.Join(path, "inside"), "x")
		sink := NewJsonLogOutputFunction(path)
		sink.Stage()
		writeRun(sink, 1)
		var resource *ResourceError
		if err := sink.Commit(); !errors.As(err, &resource) {
			t.Errorf("expected a ResourceError, got %v", err)
		}
	})

	t.Run("an unstaged sink writes straight to its path, as before", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "run.log")
		writeRun(NewJsonLogOutputFunction(path), 2000)
		if contents, _ := fileContents(t, path); contents != reference(t, 1) {
			t.Error("an unstaged log should be complete at its path once finalized")
		}
	})
}

func TestOutputViewsStageEveryStagedSink(t *testing.T) {
	dir := t.TempDir()
	logs := []string{filepath.Join(dir, "a.log"), filepath.Join(dir, "b.log")}
	views := &OutputViews{Views: []OutputView{
		{Name: "a", Condition: &EveryStepOutputCondition{}, Function: NewJsonLogOutputFunction(logs[0])},
		{Name: "memory", Condition: &EveryStepOutputCondition{},
			Function: &StateTimeStorageOutputFunction{Store: NewStateTimeStorage()}},
		{Name: "b", Condition: &EveryStepOutputCondition{}, Function: NewJsonLogOutputFunction(logs[1])},
	}}
	run := func() {
		views.Stage()
		for _, view := range views.Views {
			if sink, ok := view.Function.(*JsonLogOutputFunction); ok {
				writeRun(sink, 3)
			}
		}
	}
	run()
	for _, log := range logs {
		if _, exists := fileContents(t, log); exists {
			t.Fatalf("%s reached its path before Commit", log)
		}
	}
	if err := views.Commit(); err != nil {
		t.Fatal(err)
	}
	for _, log := range logs {
		if _, exists := fileContents(t, log); !exists {
			t.Errorf("%s was not committed", log)
		}
		os.Remove(log)
	}
	run()
	views.Abort()
	for _, log := range logs {
		if _, exists := fileContents(t, log+partialSuffix); exists {
			t.Errorf("%s's partial file survived Abort", log)
		}
	}
	// Every failure is returned, not just the first.
	for _, log := range logs {
		writeFile(t, filepath.Join(log, "inside"), "x")
	}
	run()
	err := views.Commit()
	var resource *ResourceError
	if !errors.As(err, &resource) || len(err.(interface{ Unwrap() []error }).Unwrap()) != 2 {
		t.Errorf("expected both views' failures, got %v", err)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
