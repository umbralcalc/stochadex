package simulator

import (
	"testing"
)

func TestLoadSettings(t *testing.T) {
	t.Run(
		"test config is loaded properly",
		func(t *testing.T) {
			_ = LoadSettingsFromYaml("test_settings.yaml")
		},
	)
}
