package api

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// SocketConfig is a yaml-loadable config for the real-time websocket
// connection to the simulation.
type SocketConfig struct {
	Address          string `yaml:"address"`
	Handle           string `yaml:"handle"`
	MillisecondDelay uint64 `yaml:"millisecond_delay"`
}

// Active determines whether or not the websocket should be active.
func (s *SocketConfig) Active() bool {
	return s.Address != ""
}

// LoadSocketConfigFromYaml creates a new SocketConfig from a
// provided yaml path. If the path is an empty string this outputs a
// SocketConfig with empty fields.
func LoadSocketConfigFromYaml(path string) *SocketConfig {
	config := SocketConfig{}
	if path == "" {
		fmt.Printf("\nParsed no socket config file: running without websocket ...\n")
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
