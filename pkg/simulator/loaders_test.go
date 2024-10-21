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
			_ = LoadPartitionConfigFromYaml("./test_partition_config.yaml")
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
