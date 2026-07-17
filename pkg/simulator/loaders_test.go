package simulator

import (
	"testing"
)

func TestLoadSettings(t *testing.T) {
	t.Run(
		"test settings config loaded properly",
		func(t *testing.T) {
			_ = LoadSettingsFromYaml("./test_settings.yaml")
		},
	)
}

func TestLoadPartitionConfig(t *testing.T) {
	t.Run(
		"test partition config is loaded properly",
		func(t *testing.T) {
			config := LoadPartitionConfigFromYaml("./test_partition_config.yaml")

			// Discarding the result would let a key that loads into nothing pass, which is
			// how this fixture kept two keys from a superseded schema: yaml ignores what it
			// does not recognise, so the wiring below silently did not load at all.
			upstream, ok := config.ParamsFromUpstream["other_test_params"]
			if !ok {
				t.Fatalf("upstream wiring did not load: got %v", config.ParamsFromUpstream)
			}
			if upstream.Upstream != "from_this_partition" {
				t.Errorf("upstream: got %q, want from_this_partition", upstream.Upstream)
			}
			if len(upstream.Indices) != 6 {
				t.Errorf("upstream indices: got %v, want six of them", upstream.Indices)
			}
			if got := config.ParamsAsPartitions["some_partition_params"]; len(got) != 2 {
				t.Errorf("params as partitions: got %v, want two entries", got)
			}
			if got := config.Params.Map["test_params"]; len(got) != 3 {
				t.Errorf("params: got %v, want three entries", got)
			}
			// Width is the init state's, never declared: a state_width key sat here for
			// years reading as though it set something.
			if len(config.InitStateValues) != 7 {
				t.Errorf("state width: got %d, want 7", len(config.InitStateValues))
			}
			if config.Seed != 12345 {
				t.Errorf("seed: got %d, want 12345", config.Seed)
			}
		},
	)
}

func TestLoadSimulationConfigStrings(t *testing.T) {
	t.Run(
		"test simulation config strings are loaded properly",
		func(t *testing.T) {
			_ = LoadSimulationConfigStringsFromYaml("./test_simulation_config_strings.yaml")
		},
	)
}
