package api

import "testing"

func TestLoadDashboardConfig(t *testing.T) {
	t.Run(
		"test the dashboard config successfully loads from yaml",
		func(t *testing.T) {
			_ = LoadDashboardConfigFromYaml("./dashboard_config.yaml")
		},
	)
	t.Run(
		"test the dashboard config is inactive when yaml doesn't exist",
		func(t *testing.T) {
			config := LoadDashboardConfigFromYaml("")
			if config.Active() {
				t.Error("config should be inactive")
			}
		},
	)
}
