package simulator

import (
	"os"

	"gopkg.in/yaml.v2"
)

// LoadSettingsFromYaml loads Settings from a YAML file path.
//
// Usage hints:
//   - Calls Init to populate missing defaults after unmarshalling.
func LoadSettingsFromYaml(path string) *Settings {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var settings Settings
	err = yaml.Unmarshal(yamlFile, &settings)
	if err != nil {
		panic(err)
	}
	settings.Init()
	return &settings
}

// LoadPartitionConfigFromYaml loads PartitionConfig from a YAML file path.
//
// Usage hints:
//   - Calls Init to populate missing defaults after unmarshalling.
func LoadPartitionConfigFromYaml(path string) *PartitionConfig {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var config PartitionConfig
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		panic(err)
	}
	config.Init()
	return &config
}

// LoadSimulationConfigStringsFromYaml loads SimulationConfigStrings from YAML.
func LoadSimulationConfigStringsFromYaml(path string) *SimulationConfigStrings {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var config SimulationConfigStrings
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		panic(err)
	}
	return &config
}
