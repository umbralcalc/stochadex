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
	return &settings
}
