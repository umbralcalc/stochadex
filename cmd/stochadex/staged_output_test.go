package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// stagedSink resolves a registered output function by its config spec, as a
// config run does, and stages it.
func stagedSink(t *testing.T, kind string, fields map[string]interface{}) simulator.StagedOutputFunction {
	t.Helper()
	resolved, err := simulator.ResolveOutputFunction(simulator.ComponentSpec{Type: kind, Fields: fields})
	if err != nil {
		t.Fatal(err)
	}
	sink, ok := resolved.(simulator.StagedOutputFunction)
	if !ok {
		t.Fatalf("the %s output function cannot be staged", kind)
	}
	sink.Stage()
	return sink
}

// writeWalk runs a two-row, one-partition run through sink.
func writeWalk(sink simulator.OutputFunction, partitions ...string) {
	if len(partitions) == 0 {
		partitions = []string{"walk"}
	}
	settings := &simulator.Settings{}
	for _, name := range partitions {
		settings.Iterations = append(settings.Iterations, simulator.IterationSettings{Name: name})
	}
	sink.Configure(settings)
	sink.Output("walk", []float64{1.5}, 0)
	sink.Output("walk", []float64{2.5}, 1)
	sink.(simulator.FinalizingOutputFunction).Finalize()
}

func notExists(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, os.ErrNotExist)
}

func TestStagedArrowOutput(t *testing.T) {
	t.Run("the file is written under a partial name and renamed into place at Commit", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "run.arrow")
		os.WriteFile(path, []byte("an earlier run"), 0o644)
		sink := stagedSink(t, "arrow", map[string]interface{}{"path": path})
		writeWalk(sink)
		if data, _ := os.ReadFile(path); string(data) != "an earlier run" {
			t.Fatal("the file at the path changed before Commit")
		}
		if err := sink.Commit(); err != nil {
			t.Fatal(err)
		}
		storage, err := loadArrowStorage(path)
		if err != nil {
			t.Fatal(err)
		}
		rows := storage.GetValues("walk")
		if len(rows) != 2 || !floats.Equal(rows[0], []float64{1.5}) || !floats.Equal(rows[1], []float64{2.5}) {
			t.Errorf("the committed file holds %v, want the run's rows", rows)
		}
		if !notExists(path + ".partial") {
			t.Error("Commit left the partial file")
		}
	})

	t.Run("Abort leaves the earlier file and no partial", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "run.arrow")
		os.WriteFile(path, []byte("an earlier run"), 0o644)
		sink := stagedSink(t, "arrow", map[string]interface{}{"path": path})
		writeWalk(sink)
		sink.Abort()
		if data, _ := os.ReadFile(path); string(data) != "an earlier run" || !notExists(path+".partial") {
			t.Error("Abort should leave the earlier file and remove the partial one")
		}
	})

	t.Run("a staged file that cannot be written fails Commit", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing", "run.arrow")
		sink := stagedSink(t, "arrow", map[string]interface{}{"path": path})
		writeWalk(sink)
		if err := sink.Commit(); err == nil {
			t.Errorf("expected Commit to fail writing into a missing directory, got %v", err)
		}
	})

	t.Run("a run with no single table writes nothing, as before, and is not a failure", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "run.arrow")
		sink := stagedSink(t, "arrow", map[string]interface{}{"path": path})
		writeWalk(sink, "walk", "silent") // "silent" never outputs, so the rows differ
		if err := sink.Commit(); err != nil || !notExists(path) {
			t.Errorf("want no file and no error, got %v", err)
		}
	})
}
