package api

import (
	"fmt"
	"os"
	"os/exec"
)

// DashboardConfig is a yaml-loadable config for the real-time dashboard.
type DashboardConfig struct {
	Address          string `yaml:"address"`
	Handle           string `yaml:"handle"`
	MillisecondDelay uint64 `yaml:"millisecond_delay"`
	ReactAppLocation string `yaml:"react_app_location"`
	LaunchDashboard  bool   `yaml:"launch_dashboard"`
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
