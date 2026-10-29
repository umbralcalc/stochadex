package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// replicaModel is one replica's run: an Ornstein-Uhlenbeck walk reverting to
// the mean its replica is given, and a two-wide partition echoing that mean and
// the walk, so a replica's output row is wider than one value.
const replicaModel = `      run:
        partitions:
        - name: walk
          iteration: {type: ornstein_uhlenbeck}
          params: {thetas: [0.5], mus: [0.0], sigmas: [1.0]}
          init_state_values: [0.0]
          state_history_depth: 1
          seed: 0
        - name: echo
          iteration: {type: param_values}
          params: {param_values: [0.0, 0.0]}
          params_from_upstream: {param_values: {upstream: walk, indices: [0, 0]}}
          init_state_values: [0.0, 0.0]
          state_history_depth: 1
          seed: 0
        simulation:
          termination_condition: {type: number_of_steps, max_steps: 4}
          timestep_function: {type: constant, stepsize: 0.5}
          init_time_value: 0.0
`

// replicatedYAML replicates replicaModel three times with the given extra spec
// lines, each replica's mean taken from a constant partition.
func replicatedYAML(spec string) string {
	return `main:
  partitions:
  - name: centres
    iteration: {type: constant_values}
    params: {}
    init_state_values: [1.0, -2.0, 5.0]
    state_history_depth: 1
    seed: 0
  - name: replicas
    iteration:
      type: replicated_simulation_run
      replicas: 3
` + spec + `    params: {replica_params: [0.0, 0.0, 0.0]}
    params_from_upstream: {replica_params: {upstream: centres}}
    init_state_values: [0, 0, 0, 0, 0, 0]
    state_history_depth: 1
    seed: 99
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 3}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`
}

const replicatedSpec = `      params_per_replica: 1
` + replicaModel + `      output_partition: echo
      param_forwarding: {"walk/mus": [0]}
`

// referenceReplica runs one replica's model alone, as an ordinary config,
// seeded as the replicated run promises: its partitions' seeds derived from
// the partition's seed, the step and the replica, in that order.
func referenceReplica(t *testing.T, mean float64, base uint64, step, replica int) []float64 {
	t.Helper()
	replicaSeed := simulator.DeriveSeed(simulator.DeriveSeed(base, step), replica)
	model := strings.NewReplacer(
		"      run:\n", "main:\n",
		"        ", "  ",
		"mus: [0.0]", fmt.Sprintf("mus: [%v]", mean),
	).Replace(replicaModel)
	seeds := []uint64{simulator.DeriveSeed(replicaSeed, 0), simulator.DeriveSeed(replicaSeed, 1)}
	model = strings.Replace(model, "seed: 0", fmt.Sprintf("seed: %d", seeds[0]), 1)
	model = strings.Replace(model, "seed: 0", fmt.Sprintf("seed: %d", seeds[1]), 1)
	result, err := RunToStorage(writeConfig(t, model))
	if err != nil {
		t.Fatal(err)
	}
	rows := result.Storage.GetValues("echo")
	return rows[len(rows)-1]
}

func TestReplicatedSimulationRun(t *testing.T) {
	t.Run("each replica's row is its own model run alone, with its mean and derived seed", func(t *testing.T) {
		result, err := RunToStorage(writeConfig(t, replicatedYAML(replicatedSpec)))
		if err != nil {
			t.Fatal(err)
		}
		rows := result.Storage.GetValues("replicas")
		if len(rows) != 4 {
			t.Fatalf("got %d rows, want 4", len(rows))
		}
		for step := 1; step <= 3; step++ {
			for replica, mean := range []float64{1, -2, 5} {
				want := referenceReplica(t, mean, 99, step, replica)
				got := rows[step][2*replica : 2*replica+2]
				if !floats.Equal(got, want) {
					t.Errorf("step %d replica %d: got %v, want %v", step, replica, got, want)
				}
			}
		}
	})

	t.Run("matches the smc_inference macro it generalises, exactly", func(t *testing.T) {
		macro, err := RunToStorage(LoadApiRunConfigFromYaml("testdata/replicated/smc_macro.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		twin, err := RunToStorage(LoadApiRunConfigFromYaml("testdata/replicated/smc_twin.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		assertStoragesEqual(t, "twin vs macro", twin.Storage, macro.Storage)
		final := macro.Storage.GetValues("smc_sim")[3]
		if final[0] == final[1] {
			t.Error("two particles scored alike; the comparison would not see a shared seed")
		}
	})

	t.Run("a model with no parameters needs no replica_params, and runs expressions", func(t *testing.T) {
		yaml := strings.NewReplacer(
			"    params: {replica_params: [0.0, 0.0, 0.0]}\n    params_from_upstream: {replica_params: {upstream: centres}}\n",
			"    params: {}\n",
			"init_state_values: [0, 0, 0, 0, 0, 0]", "init_state_values: [0, 0, 0]",
		).Replace(replicatedYAML(`      run:
        partitions:
        - {name: tick, params: {}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
        expressions:
        - partition: tick
          fields: [{name: x}]
          outputs: ["x + dt"]
        simulation:
          termination_condition: {type: number_of_steps, max_steps: 4}
          timestep_function: {type: constant, stepsize: 0.25}
          init_time_value: 0.0
      output_partition: tick
`))
		result, err := RunToStorage(writeConfig(t, yaml))
		if err != nil {
			t.Fatal(err)
		}
		if row := result.Storage.GetValues("replicas")[3]; !floats.Equal(row, []float64{1, 1, 1}) {
			t.Errorf("each replica should count 4 steps of 0.25, got %v", row)
		}
	})

	cases := []struct{ name, yaml, want string }{
		{"no output partition", replicatedYAML(strings.Replace(replicatedSpec, "      output_partition: echo\n", "", 1)),
			"needs output_partition:"},
		{"an output partition the model lacks", replicatedYAML(strings.Replace(replicatedSpec, "output_partition: echo", "output_partition: nope", 1)),
			`output partition "nope" is not in the replicated model`},
		{"forwarding into a partition the model lacks", replicatedYAML(strings.Replace(replicatedSpec, "walk/mus", "stroll/mus", 1)),
			`param forwarding names partition "stroll"`},
		{"forwarding into an undeclared param", replicatedYAML(strings.Replace(replicatedSpec, "walk/mus", "walk/muse", 1)),
			`param_forwarding "walk/muse" sets "muse", which partition "walk" does not declare`},
		{"a forwarding key without a param", replicatedYAML(strings.Replace(replicatedSpec, "walk/mus", "walk", 1)),
			`param forwarding key "walk" must be "partition_name/param_name"`},
		{"forwarding a parameter past the replica's own", replicatedYAML(strings.Replace(replicatedSpec, "[0]}", "[1]}", 1)),
			`sends parameter 1, but each replica has 1 (params_per_replica)`},
		{"no replicas", strings.Replace(replicatedYAML(replicatedSpec), "replicas: 3", "replicas: 0", 1),
			"replicas must be > 0, got 0"},
		{"a negative parameter count", replicatedYAML(strings.Replace(replicatedSpec, "params_per_replica: 1", "params_per_replica: -1", 1)),
			"params_per_replica must be >= 0, got -1"},
		{"an unknown field", replicatedYAML(replicatedSpec + "      copies: 2\n"), "field copies not found"},
		{"a replica writing its own output", replicatedYAML(strings.Replace(replicatedSpec, "        simulation:\n",
			"        simulation:\n          output_function: {type: stdout}\n", 1)),
			`a replica's run writes no output of its own (every replica would write it at once), so remove its output_function "stdout"`},
		{"a replica set from an input", "inputs: {data: {source: {inline: {times: [0], partitions: {param_values: [[1.0, 1.0]]}}}}}\n" + replicatedYAML(strings.Replace(replicatedSpec, "          params_from_upstream:",
			"          params_from_input: {param_values: {input: data, partition: param_values}}\n          params_from_upstream:", 1)),
			`params_from_input is not yet supported inside a replicated run (partition "echo")`},
		{"a replica's wiring naming an unknown upstream", replicatedYAML(strings.Replace(replicatedSpec, "upstream: walk", "upstream: wlak", 1)),
			"invalid run wiring:"},
		{"a replica whose wiring deadlocks", replicatedYAML(strings.Replace(replicatedSpec, "          seed: 0\n        - name: echo",
			"          params_from_upstream: {mus: {upstream: echo, indices: [0]}}\n          seed: 0\n        - name: echo", 1)),
			"deadlock"},
		{"a replica's unknown iteration", replicatedYAML(strings.Replace(replicatedSpec, "type: ornstein_uhlenbeck", "type: ornstein", 1)),
			`unknown data-spec type "ornstein"`},
	}
	for _, c := range cases {
		t.Run(c.name+" is a config error at load", func(t *testing.T) {
			_, err := LoadConfig(writeConfigPath(t, c.yaml))
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), "replicated_simulation_run: ") ||
				!strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}
}
