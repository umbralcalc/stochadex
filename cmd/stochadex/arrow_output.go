package main

import (
	"fmt"
	"os"

	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/umbralcalc/stochadex/pkg/arrowstore"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Registers `output_function: {type: arrow, path: run.arrow}`, which writes the whole run
// as a single Arrow IPC file — the columnar interchange format Polars, pandas and DuckDB
// read directly. Arrow-go is pure Go, so this sink ships in the ordinary cross-compiled
// binary; only DuckDB and the BLAS acceleration need cgo.
func init() {
	simulator.RegisterComponent(
		"output_function",
		"arrow",
		func(spec simulator.ComponentSpec) (interface{}, error) {
			path, err := stringField(spec, "path")
			if err != nil {
				return nil, err
			}
			store := arrowstore.NewArrowStateTimeStorage()
			return &arrowFileOutput{
				store: store,
				inner: &arrowstore.ArrowStateTimeStorageOutputFunction{Store: store},
				path:  path,
			}, nil
		},
	)
}

// arrowFileOutput accumulates the run into an Arrow storage and writes it out once, at
// Finalize. It implements simulator.FinalizingOutputFunction because a columnar buffer is
// only a readable record after the final row — there is nothing meaningful to write
// per-step.
type arrowFileOutput struct {
	store *arrowstore.ArrowStateTimeStorage
	inner *arrowstore.ArrowStateTimeStorageOutputFunction
	path  string
	// staged, written and failed track a staged run (see Stage).
	staged, written bool
	failed          error
}

func (a *arrowFileOutput) Configure(settings *simulator.Settings) {
	a.inner.Configure(settings)
}

func (a *arrowFileOutput) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
	a.inner.Output(partitionName, state, cumulativeTimesteps)
}

func (a *arrowFileOutput) Finalize() {
	if err := a.write(); err != nil {
		if !a.staged {
			fmt.Fprintf(os.Stderr, "stochadex: %v\n", err)
			return
		}
		a.failed = err
	}
}

// Stage writes the file under a ".partial" name until Commit, so a run that
// fails or is killed leaves any file at the path as it was.
func (a *arrowFileOutput) Stage() { a.staged = true }

// Commit renames the staged file into place, or returns why it was not written.
func (a *arrowFileOutput) Commit() error {
	if a.failed != nil {
		return a.failed
	}
	if !a.written {
		return nil
	}
	return os.Rename(a.writePath(), a.path)
}

// Abort removes the staged file.
func (a *arrowFileOutput) Abort() { os.Remove(a.writePath()) }

func (a *arrowFileOutput) writePath() string {
	if a.staged {
		return a.path + ".partial"
	}
	return a.path
}

// write writes the run's record to the file. A run whose partitions produced
// differing row counts has no single table to write, which is reported and is
// not a failure to write.
func (a *arrowFileOutput) write() error {
	defer a.store.Release()

	// Record finalises the builders and returns nil when partitions produced differing
	// row counts — an Arrow file needs one rectangular table, so say so plainly.
	record := a.store.Record()
	if record == nil {
		fmt.Fprintf(os.Stderr,
			"stochadex: cannot write %s — partitions produced differing row counts, "+
				"so the run is not a single rectangular table. Use an output_condition "+
				"that emits every partition every step (e.g. {type: every_step}).\n",
			a.path)
		return nil
	}
	defer record.Release()

	file, err := os.Create(a.writePath())
	if err != nil {
		return fmt.Errorf("creating %s: %w", a.writePath(), err)
	}
	defer file.Close()

	writer, err := ipc.NewFileWriter(file, ipc.WithSchema(record.Schema()))
	if err != nil {
		return fmt.Errorf("opening Arrow writer for %s: %w", a.path, err)
	}
	if err := writer.Write(record); err != nil {
		return fmt.Errorf("writing %s: %w", a.path, err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", a.path, err)
	}
	a.written = true
	return nil
}

// stringField reads a required string key from a data spec, with an error naming the
// output type and the key so a mistyped config is actionable rather than a nil path.
func stringField(spec simulator.ComponentSpec, key string) (string, error) {
	raw, ok := spec.Fields[key]
	if !ok {
		return "", fmt.Errorf("output_function %q: missing required field %q", spec.Type, key)
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf(
			"output_function %q: field %q must be a string, got %T", spec.Type, key, raw)
	}
	return value, nil
}
