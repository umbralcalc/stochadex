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
	for index, params := range settings.Params {
		settings.Params[index].SetPartitionName(strconv.Itoa(index))
		// ensures the default map is correctly instantiated from empty config
		if params.Map == nil {
			settings.Params[index].Map = make(map[string][]float64)
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
