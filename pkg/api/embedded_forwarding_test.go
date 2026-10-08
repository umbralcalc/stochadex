package api

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// hostedYAML is a run whose host partition "nested" embeds a short inner
// Wiener run and sets its variances through the host key "inner/variances",
// wired by binding (and any extra partitions).
func hostedYAML(csv, binding, extra string) string {
	return fmt.Sprintf(`inputs:
  vol: {source: {csv: {path: %q, time_column: 0, state_columns: {variances: [1]}}}}
main:
  partitions:
%s  - name: nested
    params: {burn_in_steps: [0], inner/variances: [1.0]}
    %s
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 0
  simulation:
    output_condition: {type: every_step}
    output_function: {type: nil}
    termination_condition: {type: number_of_steps, max_steps: 5}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
embedded:
- name: nested
  partitions:
  - {name: inner, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [5.0], state_history_depth: 1, seed: 11}
  simulation:
    output_condition: {type: nil}
    output_function: {type: nil}
    termination_condition: {type: number_of_steps, max_steps: 4}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`, csv, extra, binding)
}

func TestParamsReachEmbeddedRunsThroughTheirHost(t *testing.T) {
	dir := t.TempDir()
	csv := filepath.Join(dir, "vol.csv")
	writeFile(t, csv, "0,0.5\n1,4\n2,0.01\n3,9\n4,1\n5,16\n")

	t.Run("params_from_input on a host key sets the inner run's param", func(t *testing.T) {
		got, err := RunToStorage(writeConfig(t, hostedYAML(csv,
			"params_from_input: {inner/variances: {input: vol, partition: variances}}", "")))
		if err != nil {
			t.Fatal(err)
		}
		// Reference: the same values through params_from_upstream from a replay.
		reference, err := RunToStorage(writeConfig(t, hostedYAML(csv,
			"params_from_upstream: {inner/variances: {upstream: variances}}",
			"  - {name: variances, iteration: {type: from_input, input: vol}, state_history_depth: 1, seed: 0}\n")))
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(got.Storage.GetValues("nested")) != fmt.Sprint(reference.Storage.GetValues("nested")) {
			t.Fatalf("through params_from_input %v, through params_from_upstream %v",
				got.Storage.GetValues("nested"), reference.Storage.GetValues("nested"))
		}
		// Positive control: the input changes the inner run.
		withoutInput := hostedYAML(csv, "", "")
		constant, err := RunToStorage(writeConfig(t, withoutInput[strings.Index(withoutInput, "main:"):]))
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(constant.Storage.GetValues("nested")) == fmt.Sprint(got.Storage.GetValues("nested")) {
			t.Error("the run with variances from the input matches one with constant variances")
		}
	})

	withoutInputs := func(yaml string) string { return yaml[strings.Index(yaml, "main:"):] }
	misspelled := func(yaml string) string {
		return strings.Replace(yaml, "inner/variances: [1.0]", "innr/variances: [1.0]", 1)
	}
	for name, yaml := range map[string]string{
		"params": misspelled(withoutInputs(hostedYAML(csv, "", ""))),
		"params_from_upstream": withoutInputs(hostedYAML(csv,
			"params_from_upstream: {innr/variances: {upstream: driver}}",
			"  - {name: driver, iteration: {type: param_values}, params: {param_values: [2.0]}, "+
				"init_state_values: [2.0], state_history_depth: 1, seed: 0}\n")),
		"params_from_input": misspelled(hostedYAML(csv,
			"params_from_input: {innr/variances: {input: vol, partition: variances}}", "")),
	} {
		t.Run("a misspelled inner partition in "+name+" is a config error", func(t *testing.T) {
			_, err := LoadConfig(writeConfigPath(t, yaml))
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(),
				`partition "nested" forwards params key "innr/variances" into embedded run "nested", which has no partition "innr" (it has: inner)`) {
				t.Errorf("expected ErrConfig naming the key and the inner partitions, got %v", err)
			}
		})
	}
}
