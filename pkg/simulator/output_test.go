package simulator

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gonum.org/v1/gonum/floats"
)

// Item 4: OutputCondition decision logic.
func TestOutputConditions(t *testing.T) {
	t.Run(
		"EveryNStepsOutputCondition emits on multiples of N only",
		func(t *testing.T) {
			condition := &EveryNStepsOutputCondition{N: 3}
			history := &CumulativeTimestepsHistory{}
			want := map[int]bool{0: true, 1: false, 2: false, 3: true, 6: true, 7: false}
			for step, expected := range want {
				history.CurrentStepNumber = step
				if got := condition.IsOutputStep("p", nil, history); got != expected {
					t.Errorf("step %d: got %v, want %v", step, got, expected)
				}
			}
		},
	)
	t.Run(
		"OnlyGivenPartitionsOutputCondition filters by partition name",
		func(t *testing.T) {
			condition := &OnlyGivenPartitionsOutputCondition{
				Partitions: map[string]bool{"keep": true},
			}
			if !condition.IsOutputStep("keep", nil, nil) {
				t.Error("listed partition was not emitted")
			}
			if condition.IsOutputStep("drop", nil, nil) {
				t.Error("unlisted partition was emitted")
			}
		},
	)
	t.Run(
		"EveryStep always emits and Nil never does",
		func(t *testing.T) {
			if !(&EveryStepOutputCondition{}).IsOutputStep("p", nil, nil) {
				t.Error("EveryStepOutputCondition did not emit")
			}
			if (&NilOutputCondition{}).IsOutputStep("p", nil, nil) {
				t.Error("NilOutputCondition emitted")
			}
		},
	)
}

// Item 5: OutputFunction implementations move data into their sinks correctly.
func TestStateTimeStorageOutputFunction(t *testing.T) {
	t.Run(
		"Configure registers names and Output appends by cached index",
		func(t *testing.T) {
			store := NewStateTimeStorage()
			function := &StateTimeStorageOutputFunction{Store: store}
			settings := &Settings{
				Iterations: []IterationSettings{
					{Name: "alpha"},
					{Name: "beta"},
				},
			}
			function.Configure(settings)

			function.Output("alpha", []float64{1.0, 2.0}, 0.0)
			function.Output("beta", []float64{9.0}, 0.0)
			function.Output("alpha", []float64{3.0, 4.0}, 1.0)

			alpha := store.GetValues("alpha")
			if len(alpha) != 2 ||
				!floats.Equal(alpha[0], []float64{1.0, 2.0}) ||
				!floats.Equal(alpha[1], []float64{3.0, 4.0}) {
				t.Errorf("alpha series wrong: %v", alpha)
			}
			if beta := store.GetValues("beta"); len(beta) != 1 ||
				!floats.Equal(beta[0], []float64{9.0}) {
				t.Errorf("beta series wrong: %v", beta)
			}
			if times := store.GetTimes(); !floats.Equal(times, []float64{0.0, 1.0}) {
				t.Errorf("time axis wrong: %v", times)
			}
		},
	)
}

// readJsonLog decodes a newline-delimited JSON log file into entries.
func readJsonLog(t *testing.T, path string) []JsonLogEntry {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var entries []JsonLogEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry JsonLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("decoding log line %q: %v", scanner.Text(), err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestJsonLogOutputFunction(t *testing.T) {
	t.Run(
		"writes newline-delimited JSON entries in order",
		func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "log.jsonl")
			function := NewJsonLogOutputFunction(path)
			function.Output("alpha", []float64{1.0, 2.0}, 0.5)
			function.Output("beta", []float64{3.0}, 1.5)

			entries := readJsonLog(t, path)
			if len(entries) != 2 {
				t.Fatalf("got %d entries, want 2", len(entries))
			}
			if entries[0].PartitionName != "alpha" ||
				!floats.Equal(entries[0].State, []float64{1.0, 2.0}) ||
				entries[0].CumulativeTimesteps != 0.5 {
				t.Errorf("first entry wrong: %+v", entries[0])
			}
			if entries[1].PartitionName != "beta" ||
				entries[1].CumulativeTimesteps != 1.5 {
				t.Errorf("second entry wrong: %+v", entries[1])
			}
		},
	)
}

func TestJsonLogChannelOutputFunction(t *testing.T) {
	t.Run(
		"Close flushes buffered entries and Output does not alias its slice",
		func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "log.jsonl")
			function := NewJsonLogChannelOutputFunction(path)

			// A reusable buffer that the caller overwrites right after Output:
			// the logged value must reflect the state at call time, not the
			// later mutation (copy-on-retain guarantee).
			buffer := []float64{1.0, 2.0}
			function.Output("alpha", buffer, 0.0)
			buffer[0] = -99.0

			function.Close()

			entries := readJsonLog(t, path)
			if len(entries) != 1 {
				t.Fatalf("got %d entries, want 1", len(entries))
			}
			if !floats.Equal(entries[0].State, []float64{1.0, 2.0}) {
				t.Errorf(
					"Output aliased its input slice: logged %v, want [1 2]",
					entries[0].State,
				)
			}
		},
	)
}
