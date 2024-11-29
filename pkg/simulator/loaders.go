package simulator

import (
	"os"

	"gopkg.in/yaml.v2"
)

// LoadSettingsFromYaml creates a new Settings struct from a provided yaml path.
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

// LoadPartitionConfigFromYaml creates a new PartitionConfig struct from a
// provided yaml path.
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

// LoadSimulationConfigStringsFromYaml creates a new SimulationConfigStrings
// struct from a provided yaml path.
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
