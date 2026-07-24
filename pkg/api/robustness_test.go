package api

import (
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
