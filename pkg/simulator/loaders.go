package simulator

import (
	"os"
	"strconv"

	"gopkg.in/yaml.v2"
)

// InitParamsInSettings ensures the Params are correctly instantiated in
// a Settings config. This is typically used immediately after unmarshalling
// from a yaml config.
func InitParamsInSettings(settings *Settings) {
	for index, iteration := range settings.Iterations {
		iteration.Params.SetPartitionName(strconv.Itoa(index))
		// ensures the default map is correctly instantiated from empty config
		if iteration.Params.Map == nil {
			iteration.Params.Map = make(map[string][]float64)
		}
	}
}

// LoadSettingsFromYaml creates a new Settings struct from a provided yaml path.
func LoadSettingsFromYaml(path string) *Settings {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var settings Settings
	err = yaml.Unmarshal(yamlFile, &settings)
	InitParamsInSettings(&settings)
	if err != nil {
		panic(err)
	}
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
	config.Init()
	if err != nil {
		panic(err)
	}
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
