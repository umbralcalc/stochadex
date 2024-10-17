package simulator

import (
	"os"

	"gopkg.in/yaml.v2"
)

// InitEmptyParamsInSettings ensures the default maps of Params are correctly
// instantiated in a Settings config.
func InitEmptyParamsInSettings(settings Settings) Settings {
	for index, params := range settings.Params {
		// ensures the default map is correctly instantiated from empty config
		if params.Map == nil {
			settings.Params[index].Map = make(map[string][]float64)
		}
	}
	return settings
}

// LoadSettingsFromYaml creates a new Settings struct from a provided yaml path.
func LoadSettingsFromYaml(path string) *Settings {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var settings Settings
	err = yaml.Unmarshal(yamlFile, &settings)
	settings = InitEmptyParamsInSettings(settings)
	if err != nil {
		panic(err)
	}
	return &settings
}
