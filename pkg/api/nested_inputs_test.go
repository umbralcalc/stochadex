package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// nestedInput is an input of four rows at uneven times.
const nestedInput = `inputs:
  d: {source: {inline: {times: [0.0, 0.5, 2.0, 3.5], partitions: {x: [[1.0], [2.0], [4.0], [8.0]]}}}}
`

// inputRun is a run replaying input d's x, clocked by d, with a running total
// of what it replays and the time of each row it makes: over d it ends at
// [8, 14, 3.5], its last row, the sum of the rows after the first (2 + 4 + 8),
// and its last time.
const inputRun = `partitions:
- {name: obs, iteration: {type: from_input, input: d, partition: x}, init_state_values: [1.0], state_history_depth: 1, seed: 0}
- name: total
  iteration: {type: expression, fields: [{name: s}], outputs: ["s + v[0]"]}
  params: {v: [0.0]}
  params_from_upstream: {v: {upstream: obs}}
  init_state_values: [0.0]
  state_history_depth: 1
  seed: 0
- {name: clock, iteration: {type: expression, fields: [{name: c}], outputs: ["t + dt"]}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
simulation:
  termination_condition: {type: input_exhausted, input: d}
  timestep_function: {type: from_input, input: d}
  init_time_value: 0.0
`

// indent indents every line of text by n spaces.
func indent(text string, n int) string {
	pad := strings.Repeat(" ", n)
	return pad + strings.ReplaceAll(strings.TrimSuffix(text, "\n"), "\n", "\n"+pad) + "\n"
}

// outerClock is two constant outer steps.
const outerClock = `  simulation:
    termination_condition: {type: number_of_steps, max_steps: 2}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`

// nestedHost is a main run hosting inputRun in place.
func nestedHost(run string) string {
	return nestedInput + `main:
  partitions:
  - name: nested
    iteration:
      type: embedded_simulation_run
      run:
` + indent(run, 8) + `    params: {burn_in_steps: [0]}
    init_state_values: [1.0, 0.0, 0.0]
    state_history_depth: 1
    seed: 0
` + outerClock
}

// replicatedHost is a main run replicating inputRun three times.
func replicatedHost(run string) string {
	return nestedInput + `main:
  partitions:
  - name: replicas
    iteration:
      type: replicated_simulation_run
      replicas: 3
      output_partition: total
      run:
` + indent(run, 8) + `    init_state_values: [0.0, 0.0, 0.0]
    state_history_depth: 1
    seed: 5
` + outerClock
}

// lastRows runs a config and returns each step's row of a partition.
func lastRows(t *testing.T, yaml, partition string) [][]float64 {
	t.Helper()
	result, err := RunToStorage(writeConfig(t, yaml))
	if err != nil {
		t.Fatal(err)
	}
	return result.Storage.GetValues(partition)
}

func TestNestedInputs(t *testing.T) {
	want := []float64{8, 14, 3.5}
	everyStep := func(t *testing.T, rows [][]float64, want []float64) {
		t.Helper()
		if len(rows) != 3 {
			t.Fatalf("got %d rows, want 3", len(rows))
		}
		for step := 1; step < len(rows); step++ {
			if !floats.Equal(rows[step], want) {
				t.Errorf("step %d: %v, want %v", step, rows[step], want)
			}
		}
	}

	t.Run("a nested run replays an input and runs on its clock", func(t *testing.T) {
		everyStep(t, lastRows(t, nestedHost(inputRun), "nested"), want)
	})

	t.Run("a nested replay names its input partition by its own name, unless told", func(t *testing.T) {
		everyStep(t, lastRows(t, strings.Replace(nestedHost(strings.NewReplacer(
			"{name: obs, iteration: {type: from_input, input: d, partition: x}", "{name: w, iteration: {type: from_input, input: d}",
			"{upstream: obs}", "{upstream: w}").Replace(inputRun)), "partitions: {x:", "partitions: {w:", 1), "nested"), want)
	})

	t.Run("two levels down", func(t *testing.T) {
		inner := "partitions:\n- name: inner\n  iteration:\n    type: embedded_simulation_run\n    run:\n" +
			indent(inputRun, 6) + "  params: {burn_in_steps: [0]}\n  init_state_values: [1.0, 0.0, 0.0]\n" +
			"  state_history_depth: 1\n  seed: 0\nsimulation:\n" +
			"  termination_condition: {type: number_of_steps, max_steps: 1}\n" +
			"  timestep_function: {type: constant, stepsize: 1.0}\n  init_time_value: 0.0\n"
		everyStep(t, lastRows(t, nestedHost(inner), "nested"), want)
	})

	t.Run("every replica of a replicated run replays it", func(t *testing.T) {
		everyStep(t, lastRows(t, replicatedHost(inputRun), "replicas"), []float64{14, 14, 14})
	})

	t.Run("a top-level embedded run replays it", func(t *testing.T) {
		everyStep(t, lastRows(t, nestedInput+`main:
  partitions:
  - {name: inner_run, params: {burn_in_steps: [0]}, init_state_values: [1.0, 0.0, 0.0], state_history_depth: 1, seed: 0}
`+outerClock+`embedded:
- name: inner_run
`+indent(inputRun, 2), "inner_run"), want)
	})

	t.Run("each ensemble member binds its own", func(t *testing.T) {
		result, err := RunToStorage(writeConfig(t, nestedHost(inputRun)+"run: {mode: ensemble, seeds: [1, 2]}\n"))
		if err != nil {
			t.Fatal(err)
		}
		for index, member := range result.Members {
			everyStep(t, member.Storage.GetValues("nested"), want)
			if t.Failed() {
				t.Fatalf("member %d", index)
			}
		}
	})

	t.Run("the bound run passes the harness, running twice alike", func(t *testing.T) {
		config, err := LoadConfig(writeConfigPath(t, nestedHost(inputRun)))
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := preparedMainGenerator(config)
		if err != nil {
			t.Fatal(err)
		}
		if err := simulator.RunWithHarnesses(prepared.settings, prepared.implementations); err != nil {
			t.Error(err)
		}
	})

	t.Run("a nested reader run unbound fails loudly, pointing at the API", func(t *testing.T) {
		// The run checks its end before each step, so each case leaves only
		// its own reader on the input.
		bounded := strings.NewReplacer("{type: input_exhausted, input: d}", "{type: number_of_steps, max_steps: 2}")
		for reader, run := range map[string]string{
			"a from_input partition (input \"d\") was run unbound": strings.NewReplacer(
				"{type: input_exhausted, input: d}", "{type: number_of_steps, max_steps: 2}",
				"{type: from_input, input: d}", "{type: constant, stepsize: 1.0}").Replace(inputRun),
			"timestep_function from_input (input \"d\") was run unbound": bounded.Replace(strings.Replace(inputRun,
				"{type: from_input, input: d, partition: x}", "{type: constant_values}", 1)),
			"termination_condition input_exhausted (input \"d\") was run unbound": strings.NewReplacer(
				"{type: from_input, input: d, partition: x}", "{type: constant_values}",
				"{type: from_input, input: d}", "{type: constant, stepsize: 1.0}").Replace(inputRun),
		} {
			config, err := LoadConfig(writeConfigPath(t, nestedHost(run)))
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered == nil || !strings.Contains(
						fmt.Sprint(recovered), reader) {
						t.Errorf("expected a panic saying %q, got %v", reader, recovered)
					}
				}()
				settings, implementations := config.GetConfigGenerator().GenerateConfigs()
				simulator.NewPartitionCoordinator(settings, implementations).Run()
			}()
		}
	})

	for _, c := range []struct {
		name, yaml string
		kind       ErrorKind
		want       string
	}{
		{"a nested replay as wide as no row of its input", nestedHost(strings.Replace(inputRun,
			"partition: x}, init_state_values: [1.0]", "partition: x}, init_state_values: [1.0, 1.0]", 1)),
			ErrData, `nested partition "obs" has 2 init_state_values but input "d"'s "x" rows have 1`},
		{"a nested replay of a partition its input lacks", nestedHost(strings.Replace(inputRun,
			"partition: x}", "partition: z}", 1)),
			ErrData, `input "d" has no partition "z" (it has: x)`},
		{"a nested replay of an input that cannot be read", strings.Replace(nestedHost(inputRun),
			"{inline: {times: [0.0, 0.5, 2.0, 3.5], partitions: {x: [[1.0], [2.0], [4.0], [8.0]]}}}",
			"{csv: {path: gone.csv, time_column: 0, state_columns: {x: [1]}}}", 1),
			ErrUnavailable, `loading input "d"`},
		{"a nested clock on an input that cannot be read", strings.Replace(nestedHost(strings.Replace(inputRun,
			"{type: from_input, input: d, partition: x}", "{type: constant_values}", 1)),
			"{inline: {times: [0.0, 0.5, 2.0, 3.5], partitions: {x: [[1.0], [2.0], [4.0], [8.0]]}}}",
			"{csv: {path: gone.csv, time_column: 0, state_columns: {x: [1]}}}", 1),
			ErrUnavailable, `loading input "d"`},
		{"a nested end on an input that cannot be read", strings.Replace(nestedHost(strings.NewReplacer(
			"{type: from_input, input: d, partition: x}", "{type: constant_values}",
			"{type: from_input, input: d}", "{type: constant, stepsize: 1.0}").Replace(inputRun)),
			"{inline: {times: [0.0, 0.5, 2.0, 3.5], partitions: {x: [[1.0], [2.0], [4.0], [8.0]]}}}",
			"{csv: {path: gone.csv, time_column: 0, state_columns: {x: [1]}}}", 1),
			ErrUnavailable, `loading input "d"`},
	} {
		t.Run(c.name+" fails the run, saying why", func(t *testing.T) {
			_, err := RunToStorage(writeConfig(t, c.yaml))
			if KindOf(err) != c.kind || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected %v containing %q, got %v", c.kind, c.want, err)
			}
		})
	}

	for _, c := range []struct{ name, yaml, want string }{
		{"a replica reading an undeclared input", strings.Replace(replicatedHost(inputRun), "input: d, partition: x", "input: e, partition: x", 1),
			`nested partition "obs" names input "e", which inputs: does not declare`},
		{"a nested end on an undeclared input", strings.Replace(nestedHost(inputRun), "{type: input_exhausted, input: d}", "{type: input_exhausted, input: e}", 1),
			`a nested run's termination_condition input_exhausted names input "e", which inputs: does not declare`},
	} {
		t.Run(c.name+" is a config error at load", func(t *testing.T) {
			_, err := LoadConfig(writeConfigPath(t, c.yaml))
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}
}
