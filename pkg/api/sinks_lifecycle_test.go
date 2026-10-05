package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

const sentinel = "previous run's output\n"

// writeSentinel puts recognisable content at path, standing in for the output of
// an earlier run that loading or inspecting a config must not destroy.
func writeSentinel(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertSentinelIntact(t *testing.T, path, when string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", when, err)
	}
	if string(got) != sentinel {
		t.Fatalf("%s changed %s to %q", when, path, got)
	}
}

func readLogEntries(t *testing.T, path string) []simulator.JsonLogEntry {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	entries := make([]simulator.JsonLogEntry, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry simulator.JsonLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("decoding %q: %v", scanner.Text(), err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func TestSinksOpenOnlyWhenARunStarts(t *testing.T) {
	t.Run("loading, RunToStorage and ensembles leave a json_log untouched; Run replaces it", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "run.log")
		writeSentinel(t, logPath)
		yaml := fmt.Sprintf(batchConfigYAML, "{type: every_step}", logPath)

		config := writeConfig(t, yaml)
		assertSentinelIntact(t, logPath, "loading the config")

		result, err := RunToStorage(config)
		if err != nil {
			t.Fatal(err)
		}
		assertSentinelIntact(t, logPath, "RunToStorage")

		// Ensemble members are rebuilt by re-loading the file once per member.
		ensemble := writeConfig(t, yaml+"run: {mode: ensemble, seeds: [1, 2, 3]}\n")
		if _, err := RunToStorage(ensemble); err != nil {
			t.Fatal(err)
		}
		assertSentinelIntact(t, logPath, "an ensemble run's member reloads")

		// Running for real replaces the old output with exactly this run's rows.
		Run(config, &SocketConfig{})
		entries := readLogEntries(t, logPath)
		if len(entries) != 2*31 {
			t.Fatalf("expected 62 entries (2 partitions x 31 rows), got %d", len(entries))
		}
		times := result.Storage.GetTimes()
		indexOf := map[float64]int{}
		for i, time := range times {
			indexOf[time] = i
		}
		for _, entry := range entries {
			want := result.Storage.GetValues(entry.PartitionName)[indexOf[entry.CumulativeTimesteps]]
			if !floats.Equal(entry.State, want) {
				t.Fatalf("%s at %v = %v, want %v",
					entry.PartitionName, entry.CumulativeTimesteps, entry.State, want)
			}
		}
	})

	t.Run("a nested run's json_log accumulates every inner run", func(t *testing.T) {
		// The nested sink is configured and finalized once per outer step; its
		// records must accumulate, not be truncated by each inner run.
		logPath := filepath.Join(t.TempDir(), "inner.log")
		config := writeConfig(t, fmt.Sprintf(`main:
  partitions:
  - name: nested
    params: {burn_in_steps: [0]}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 0
  simulation:
    output_condition: {type: every_step}
    output_function: {type: nil}
    termination_condition: {type: number_of_steps, max_steps: 3}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
embedded:
- name: nested
  partitions:
  - name: inner
    iteration: {type: wiener_process}
    params: {variances: [1.0]}
    init_state_values: [5.0]
    state_history_depth: 1
    seed: 11
  simulation:
    output_condition: {type: every_step}
    output_function: {type: json_log, path: "%s"}
    termination_condition: {type: number_of_steps, max_steps: 4}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`, logPath))
		Run(config, &SocketConfig{})
		entries := readLogEntries(t, logPath)
		// 3 outer steps, each running the inner simulation: its initial row + 4 steps.
		if len(entries) != 3*5 {
			t.Fatalf("expected 15 entries (3 inner runs x 5 rows), got %d", len(entries))
		}
		for run := range 3 {
			first := entries[run*5]
			if first.State[0] != 5.0 {
				t.Errorf("inner run %d should start from its initial state 5.0, got %v",
					run, first.State)
			}
		}
	})

	t.Run("a postgres sink connects when the run starts, not at load", func(t *testing.T) {
		// Port 1 refuses connections immediately, with or without a local database.
		logLess := strings.Replace(fmt.Sprintf(batchConfigYAML, "{type: every_step}", "unused"),
			`output_function: {type: json_log, path: "unused"}`,
			`output_function: {type: postgres, driver: postgres, `+
				`dsn: "postgres://u:p@127.0.0.1:1/db?sslmode=disable&connect_timeout=1", `+
				`table: results}`, 1)
		if !strings.Contains(logLess, "type: postgres") {
			t.Fatal("test config edit not applied")
		}
		config := writeConfig(t, logLess) // previously panicked: dial tcp 127.0.0.1:1
		if _, err := RunToStorage(config); err != nil {
			t.Fatalf("RunToStorage needs no database, got %v", err)
		}
		panicked := func() (message string) {
			defer func() {
				if r := recover(); r != nil {
					message = fmt.Sprint(r)
				}
			}()
			Run(config, &SocketConfig{})
			return ""
		}()
		if !strings.Contains(panicked, "127.0.0.1:1") {
			t.Errorf("running should fail connecting to the database, got %q", panicked)
		}
	})
}
