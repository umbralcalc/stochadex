package api

import (
	"fmt"
	"testing"

	"gopkg.in/yaml.v2"
)

// TestYamlBooleanCoercionSurvives is a regression guard: YAML 1.1 treats bare
// y/n/yes/no/on/off/true/false as booleans, so a partition named any of those
// would be silently corrupted into a boolean if a macro field were decoded through
// interface{}. Decoding straight into the typed dataRefSpec must preserve them.
func TestYamlBooleanCoercionSurvives(t *testing.T) {
	for _, name := range []string{"y", "n", "yes", "no", "on", "off", "true", "false"} {
		var spec dataRefSpec
		if err := yaml.Unmarshal([]byte("partition_name: "+name), &spec); err != nil {
			t.Fatalf("%q: %v", name, err)
		}
		if spec.PartitionName != name {
			t.Errorf("partition_name %q was coerced to %q", name, spec.PartitionName)
		}
	}
}

// TestMacroConfigPreservesBooleanishNames checks the same end-to-end: a macro
// referencing a partition named "no" decodes with the name intact.
func TestMacroConfigPreservesBooleanishNames(t *testing.T) {
	var config ApiRunConfig
	err := yaml.Unmarshal([]byte(
		"macros:\n- {type: vector_mean, name: no, data: {partition_name: yes}}\n"), &config)
	if err != nil {
		t.Fatal(err)
	}
	spec := config.Macros[0].Spec.(*vectorMeanSpec)
	if spec.Name != "no" || spec.Data.PartitionName != "yes" {
		t.Errorf("boolean-like names corrupted: name=%q data=%q", spec.Name, spec.Data.PartitionName)
	}
}

// TestIsFullyDataBoundary checks the in-process/codegen boundary: a config is
// fully data only when it names no Go anywhere. Any Go iteration, extra_vars, or
// Go-expression simulation component must flip it to the codegen path.
func TestIsFullyDataBoundary(t *testing.T) {
	base := `main:
  partitions:
  - name: p
    %s
    params: {rate: [0.05]}
    init_state_values: [1.0]
    state_history_depth: 1
    seed: 1
  %s
  simulation:
    output_condition: {type: every_step}
    output_function: {type: stdout}
    termination_condition: {type: number_of_steps, max_steps: 5}
    timestep_function: %s
`
	cases := []struct {
		name        string
		iteration   string
		expressions string
		timestep    string
		wantData    bool
	}{
		{"all data", "iteration: {type: wiener_process}", "", "{type: constant, stepsize: 1.0}", true},
		{"expression partition", "", "expressions:\n  - {partition: p, fields: [{name: x}], outputs: [\"x\"]}", "{type: constant, stepsize: 1.0}", true},
		{"go iteration", `iteration: "&continuous.WienerProcessIteration{}"`, "", "{type: constant, stepsize: 1.0}", false},
		{"go simulation component", "iteration: {type: wiener_process}", "", `"&simulator.ConstantTimestepFunction{Stepsize: 1.0}"`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			yamlText := fmt.Sprintf(base, tc.iteration, tc.expressions, tc.timestep)
			var strings ApiRunConfigStrings
			if err := yaml.Unmarshal([]byte(yamlText), &strings); err != nil {
				t.Fatal(err)
			}
			if got := strings.IsFullyData(); got != tc.wantData {
				t.Errorf("IsFullyData() = %v, want %v", got, tc.wantData)
			}
		})
	}
}
