package api

import (
	"reflect"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// The host's side of a nested run: a Wiener drive whose history it feeds to
// the inner gradient, a forwarded variance for the inner walk, and the black-box
// row (the gradient's state, then the walk's).
const nestedHostSide = `    params: {burn_in_steps: [0], walk/variances: [2.0]}
    params_as_partitions: {gradient/update_from_partition_history: [drive]}
    init_state_values: [0.0, 0.0]
    state_history_depth: 1
    seed: 0
`

// nestedInnerRun is the inner run, at the given indent.
func nestedInnerRun(indent string) string {
	run := `partitions:
- name: gradient
  iteration: {type: data_comparison_gradient, likelihood: {type: normal}, gradient_function: mean_gradient}
  params: {mean: [0.5], covariance_matrix: [2.0], drive->data_values: []}
  init_state_values: [0.0]
  state_history_depth: 1
  seed: 0
- {name: walk, iteration: {type: wiener_process}, params: {variances: [0.5]}, init_state_values: [0.0], state_history_depth: 1, seed: 9}
simulation:
  termination_condition: {type: number_of_steps, max_steps: 3}
  timestep_function: {type: constant, stepsize: 1.0}
  init_time_value: 0.0
`
	return indent + strings.ReplaceAll(strings.TrimSuffix(run, "\n"), "\n", "\n"+indent) + "\n"
}

const nestedOuter = `main:
  partitions:
  - {name: drive, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [1.0], state_history_depth: 3, seed: 3}
`

const nestedClock = `  simulation:
    termination_condition: {type: number_of_steps, max_steps: 8}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`

// topLevelNested is the run as a top-level embedded: block.
var topLevelNested = nestedOuter + "  - name: nested\n" + nestedHostSide + nestedClock +
	"embedded:\n- name: nested\n" + nestedInnerRun("  ")

// specNested is the same run stated in place, as an embedded_simulation_run.
var specNested = nestedOuter + "  - name: nested\n    iteration:\n      type: embedded_simulation_run\n" +
	"      run:\n" + nestedInnerRun("        ") + nestedHostSide + nestedClock

// threeLevels nests a run hosting a run: level1's run steps a Wiener walk mid
// and hosts level2, whose run is an Ornstein-Uhlenbeck process deep reverting
// to mid, forwarded each step.
const threeLevels = `main:
  partitions:
  - name: level1
    iteration:
      type: embedded_simulation_run
      run:
        partitions:
        - {name: mid, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [0.0], state_history_depth: 1, seed: 5}
        - name: level2
          iteration:
            type: embedded_simulation_run
            run:
              partitions:
              - {name: deep, iteration: {type: ornstein_uhlenbeck}, params: {thetas: [2.0], mus: [0.0], sigmas: [0.3]}, init_state_values: [0.0], state_history_depth: 1, seed: 7}
              simulation:
                output_function: {type: nil}
                termination_condition: {type: number_of_steps, max_steps: 4}
                timestep_function: {type: constant, stepsize: 0.25}
                init_time_value: 0.0
          params: {burn_in_steps: [0], deep/mus: [0.0]}
          params_from_upstream: {deep/mus: {upstream: mid}}
          init_state_values: [0.0]
          state_history_depth: 1
          seed: 0
        simulation:
          termination_condition: {type: number_of_steps, max_steps: 3}
          timestep_function: {type: constant, stepsize: 1.0}
          init_time_value: 0.0
    params: {burn_in_steps: [0]}
    init_state_values: [0.0, 0.0]
    state_history_depth: 1
    seed: 0
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 5}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`

// levelTwoAsTopLevel is level1's run on its own, with level2 as a top-level
// embedded: block: the established path the reference is built from.
const levelTwoAsTopLevel = `main:
  partitions:
  - {name: mid, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [0.0], state_history_depth: 1, seed: 5}
  - name: level2
    params: {burn_in_steps: [0], deep/mus: [0.0]}
    params_from_upstream: {deep/mus: {upstream: mid}}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 0
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 3}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
embedded:
- name: level2
  partitions:
  - {name: deep, iteration: {type: ornstein_uhlenbeck}, params: {thetas: [2.0], mus: [0.0], sigmas: [0.3]}, init_state_values: [0.0], state_history_depth: 1, seed: 7}
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 4}
    timestep_function: {type: constant, stepsize: 0.25}
    init_time_value: 0.0
`

// threeLevelReference runs level1 by hand: Go wraps the top-level-embedded
// form of its run in an outer five-step run.
func threeLevelReference(t *testing.T) [][]float64 {
	t.Helper()
	inner, err := LoadConfig(writeConfigPath(t, levelTwoAsTopLevel))
	if err != nil {
		t.Fatal(err)
	}
	generator := simulator.NewConfigGenerator()
	generator.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 5},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1},
	})
	generator.SetPartition(&simulator.PartitionConfig{
		Name: "level1",
		Iteration: general.NewEmbeddedSimulationRunIteration(
			inner.GetConfigGenerator().GenerateConfigs()),
		Params:            simulator.NewParams(map[string][]float64{"burn_in_steps": {0}}),
		InitStateValues:   []float64{0, 0},
		StateHistoryDepth: 1,
	})
	settings, implementations := generator.GenerateConfigs()
	store := simulator.NewStateTimeStorage()
	implementations.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	simulator.NewPartitionCoordinator(settings, implementations).Run()
	return store.GetValues("level1")
}

func TestEmbeddedSimulationRunSpec(t *testing.T) {
	t.Run("stated in place, a run gives what the same top-level embedded: run gives", func(t *testing.T) {
		top, err := RunToStorage(writeConfig(t, topLevelNested))
		if err != nil {
			t.Fatal(err)
		}
		spec, err := RunToStorage(writeConfig(t, specNested))
		if err != nil {
			t.Fatal(err)
		}
		assertStoragesEqual(t, "spec vs top-level", spec.Storage, top.Storage)
		rows := spec.Storage.GetValues("nested")
		if rows[5][0] == rows[6][0] || rows[5][1] == rows[6][1] {
			t.Errorf("the fed gradient and the walk should both move, got %v then %v", rows[5], rows[6])
		}
	})

	t.Run("three levels in one file match level two built by hand around level one", func(t *testing.T) {
		result, err := RunToStorage(writeConfig(t, threeLevels))
		if err != nil {
			t.Fatal(err)
		}
		got, want := result.Storage.GetValues("level1"), threeLevelReference(t)
		if len(got) != len(want) {
			t.Fatalf("%d rows, want %d", len(got), len(want))
		}
		for step := range got {
			if !floats.Equal(got[step], want[step]) {
				t.Errorf("step %d: %v, want %v", step, got[step], want[step])
			}
		}
		// Both levels are live: mid and deep (in level2's column) move.
		if got[4][0] == got[5][0] || got[4][1] == got[5][1] {
			t.Errorf("levels two and three should both move, got %v then %v", got[4], got[5])
		}
	})

	t.Run("two partitions stating the same nested run each run their own", func(t *testing.T) {
		nested := `    iteration:
      type: embedded_simulation_run
      run:
        partitions:
        - {name: walk, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [0.0], state_history_depth: 1, seed: 9}
        simulation:
          termination_condition: {type: number_of_steps, max_steps: 3}
          timestep_function: {type: constant, stepsize: 1.0}
          init_time_value: 0.0
    params: {burn_in_steps: [0]}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 0
`
		result, err := RunToStorage(writeConfig(t, "main:\n  partitions:\n  - name: first\n"+nested+
			"  - name: second\n"+nested+nestedClock))
		if err != nil {
			t.Fatal(err)
		}
		first, second := result.Storage.GetValues("first"), result.Storage.GetValues("second")
		if !equalRows(first, second) || first[5][0] == first[6][0] {
			t.Errorf("two identical nested runs should step alike and move, got %v and %v", first, second)
		}
	})

	t.Run("a loaded config runs alike twice", func(t *testing.T) {
		config := writeConfig(t, threeLevels)
		first, err := RunToStorage(config)
		if err != nil {
			t.Fatal(err)
		}
		second, err := RunToStorage(config)
		if err != nil {
			t.Fatal(err)
		}
		assertStoragesEqual(t, "second run vs first", second.Storage, first.Storage)
	})

	t.Run("inspect --io gives the host's column layout", func(t *testing.T) {
		config, err := LoadConfig(writeConfigPath(t, specNested))
		if err != nil {
			t.Fatal(err)
		}
		want := []ManifestEmbedded{{Partition: "nested", Columns: []ManifestColumns{
			{Partition: "gradient", Offset: 0, Width: 1}, {Partition: "walk", Offset: 1, Width: 1}}}}
		if got := Manifest(config).Embedded; !reflect.DeepEqual(got, want) {
			t.Errorf("layout %+v, want %+v", got, want)
		}
		// A config assembled in Go has no load scope; its nested run is read
		// all the same.
		assembled := &ApiRunConfig{Main: config.Main}
		if got := Manifest(assembled).Embedded; !reflect.DeepEqual(got, want) {
			t.Errorf("assembled config's layout %+v, want %+v", got, want)
		}
	})

	deep := "              - {name: deep,"
	cases := []struct{ name, yaml, want string }{
		{"a nested run writing its own output", strings.Replace(threeLevels,
			"        simulation:\n          termination_condition: {type: number_of_steps, max_steps: 3}",
			"        simulation:\n          output_function: {type: stdout}\n          termination_condition: {type: number_of_steps, max_steps: 3}", 1),
			`its run writes output_function "stdout", but a nested run is a black box`},
		{"a nested run's embedded run writing its own output", strings.Replace(specNested,
			"      run:\n", "      run:\n        embedded:\n        - name: spare\n          partitions: [{name: idle, iteration: {type: constant_values}, "+
				"params: {}, init_state_values: [0.0], state_history_depth: 1, seed: 0}]\n          simulation: {output_function: {type: stdout}, "+
				"termination_condition: {type: number_of_steps, max_steps: 1}, timestep_function: {type: constant, stepsize: 1.0}}\n", 1),
			`its embedded run "spare" writes output_function "stdout"`},
		{"a nested run reading an undeclared input", strings.Replace(threeLevels, deep,
			"              - {name: obs, iteration: {type: from_input, input: data}, state_history_depth: 1, seed: 0}\n"+deep, 1),
			`nested partition "obs" names input "data", which inputs: does not declare`},
		{"a nested run set from an input", strings.Replace(threeLevels, "params: {thetas: [2.0], mus: [0.0], sigmas: [0.3]},",
			"params: {thetas: [2.0], mus: [0.0], sigmas: [0.3]}, params_from_input: {thetas: {input: data, partition: t}},", 1),
			`params_from_input is not yet supported inside a nested run (partition "deep")`},
		{"a nested run clocked by an undeclared input", strings.Replace(threeLevels,
			"                timestep_function: {type: constant, stepsize: 0.25}", "                timestep_function: {type: from_input, input: data}", 1),
			`a nested run's timestep_function from_input names input "data", which inputs: does not declare`},
		{"a host forwarding into a partition its nested run lacks", strings.Replace(threeLevels,
			"deep/mus: [0.0]}", "deep/mus: [0.0], deeper/mus: [1.0]}", 1),
			`partition "level2" forwards params key "deeper/mus" into embedded run "level2", which has no partition "deeper" (it has: deep)`},
		{"a nested partition with no iteration", strings.Replace(threeLevels,
			"{name: mid, iteration: {type: wiener_process}, ", "{name: mid, ", 1),
			"config omits iteration for partition name: mid"},
		{"a nested partition keeping no history", strings.Replace(threeLevels,
			"state_history_depth: 1, seed: 7}", "state_history_depth: 0, seed: 7}", 1),
			`main partition "deep" needs state_history_depth of at least 1, got 0`},
		{"a nested run whose wiring deadlocks", strings.Replace(threeLevels,
			"{name: mid, iteration: {type: wiener_process}, params: {variances: [1.0]},",
			"{name: mid, iteration: {type: wiener_process}, params: {variances: [1.0]}, params_from_upstream: {variances: {upstream: level2}},", 1),
			"deadlock"},
		{"an unknown iteration in a nested run's embedded run", strings.Replace(specNested, "      run:\n",
			"      run:\n        embedded:\n        - name: spare\n          partitions: [{name: idle, iteration: {type: constant}, "+
				"params: {}, init_state_values: [0.0], state_history_depth: 1, seed: 0}]\n          simulation: {"+
				"termination_condition: {type: number_of_steps, max_steps: 1}, timestep_function: {type: constant, stepsize: 1.0}}\n", 1),
			`unknown data-spec type "constant"`},
		{"an unknown field", strings.Replace(threeLevels, "      run:\n", "      runs: 2\n      run:\n", 1),
			"field runs not found"},
		{"a nested memory iteration its host does not feed", strings.Replace(specNested,
			"    params_as_partitions: {gradient/update_from_partition_history: [drive]}\n", "", 1),
			`embedded run "nested" partition "gradient" (data_comparison_gradient) reads a history its host feeds it, but host "nested" does not`},
		{"a memory iteration unfed two levels down", strings.Replace(threeLevels, deep,
			"              - {name: copy, iteration: {type: from_history}, params: {}, init_state_values: [0.0], state_history_depth: 1, seed: 0}\n"+deep, 1),
			`embedded run "level2" partition "copy" (from_history) reads a history its host feeds it, but host "level2" does not`},
		{"a memory iteration unfed in a nested run's embedded run", strings.NewReplacer(
			"      run:\n        partitions:\n", "      run:\n        partitions:\n        - {name: holder, params: {burn_in_steps: [0]}, "+
				"init_state_values: [0.0], state_history_depth: 1, seed: 0}\n",
			nestedHostSide, "        embedded:\n        - name: holder\n          partitions: [{name: copy, iteration: {type: from_history}, "+
				"params: {}, init_state_values: [0.0], state_history_depth: 1, seed: 0}]\n          simulation: {"+
				"termination_condition: {type: number_of_steps, max_steps: 1}, timestep_function: {type: constant, stepsize: 1.0}}\n"+
				nestedHostSide).Replace(specNested),
			`embedded run "holder" partition "copy" (from_history) reads a history its host feeds it, but host "holder" does not`},
	}
	for _, c := range cases {
		t.Run(c.name+" is a config error at load", func(t *testing.T) {
			_, err := LoadConfig(writeConfigPath(t, c.yaml))
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}
}
