package api

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// SocketConfig configures an optional real-time websocket used to stream
// simulation updates.
type SocketConfig struct {
	Address          string `yaml:"address"`
	Handle           string `yaml:"handle"`
	MillisecondDelay uint64 `yaml:"millisecond_delay"`
}

// Active reports whether the websocket server should be started.
func (s *SocketConfig) Active() bool {
	return s.Address != ""
}

// LoadSocketConfigFromYaml loads a SocketConfig from YAML.
// If the path is empty, it returns a zero-valued config and logs that sockets
// are disabled.
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
