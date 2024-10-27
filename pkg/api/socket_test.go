package api

import "testing"

func TestLoadSocketConfig(t *testing.T) {
	t.Run(
		"test the socket config successfully loads from yaml",
		func(t *testing.T) {
			_ = LoadSocketConfigFromYaml("./socket_config.yaml")
		},
	)
	t.Run(
		"test the socket config is inactive when yaml doesn't exist",
		func(t *testing.T) {
			config := LoadSocketConfigFromYaml("")
			if config.Active() {
				t.Error("config should be inactive")
			}
		},
	)
}
