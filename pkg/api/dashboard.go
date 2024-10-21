package api

import (
	"fmt"
	"os"
	"os/exec"

	"gopkg.in/yaml.v2"
)

// DashboardConfig is a yaml-loadable config for the real-time dashboard.
type DashboardConfig struct {
	Address          string `yaml:"address"`
	Handle           string `yaml:"handle"`
	MillisecondDelay uint64 `yaml:"millisecond_delay"`
	ReactAppLocation string `yaml:"react_app_location"`
	LaunchDashboard  bool   `yaml:"launch_dashboard"`
}

// LoadDashboardConfigFromYaml creates a new DashboardConfig from a
// provided yaml path. If the path is an empty string this outputs a
// DashboardConfig with empty and dummy fields.
func LoadDashboardConfigFromYaml(path string) *DashboardConfig {
	config := DashboardConfig{}
	if path == "" {
		fmt.Printf("Parsed no dashboard config file: running without dashboard")
		config.Address = "dummy"
		config.Handle = "dummy"
	} else {
		yamlFile, err := os.ReadFile(path)
		if err != nil {
			panic(err)
		}
		err = yaml.Unmarshal(yamlFile, &config)
		if err != nil {
			panic(err)
		}
	}
	return &config
}

// StartReactApp starts a react app defined in the appLocation directory.
func StartReactApp(appLocation string) (*os.Process, error) {
	cmd := exec.Command("serve", "-s", "build")
	cmd.Dir = appLocation
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start react app: %w", err)
	}

	return cmd.Process, nil
}
