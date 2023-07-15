package simulator

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/akamensky/argparse"
	"gopkg.in/yaml.v2"
)

// LoadSettingsConfig is the yaml-loadable config which defines all of
// the settings that can be set for a stochastic process defined by the
// stochadex.
type LoadSettingsConfig struct {
	OtherParams           []*OtherParams `yaml:"other_params"`
	InitStateValues       [][]float64    `yaml:"init_state_values"`
	Seeds                 []uint64       `yaml:"seeds"`
	StateWidths           []int          `yaml:"state_widths"`
	StateHistoryDepths    []int          `yaml:"state_history_depths"`
	TimestepsHistoryDepth int            `yaml:"timesteps_history_depth"`
}

// NewLoadSettingsConfigFromYaml creates a new LoadSettingsConfig from
// a provided yaml path.
func NewLoadSettingsConfigFromYaml(path string) *LoadSettingsConfig {
	yamlFile, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var settings LoadSettingsConfig
	err = yaml.Unmarshal(yamlFile, &settings)
	if err != nil {
		panic(err)
	}
	return &settings
}

// NewLoadSettingsConfigFromYaml calls NewLoadSettingsConfigFromYaml
// with a yaml path string provided by argparse.
func NewLoadSettingsConfigFromArgParsedYaml() *LoadSettingsConfig {
	parser := argparse.NewParser(
		"stochadex simulator",
		"simulates your chosen stochastic process",
	)
	s := parser.String(
		"s",
		"string",
		&argparse.Options{Required: true, Help: "yaml config path for settings"},
	)
	err := parser.Parse(os.Args)
	if err != nil {
		fmt.Print(parser.Usage(err))
	}
	return NewLoadSettingsConfigFromYaml(*s)
}

// LoadImplementationsConfig is the yaml-loadable config which defines all of
// the methods that must be implemented in order to configure a stochastic process
// defined by the stochadex.
type LoadImplementationsConfig struct {
	Iterations           []Iteration
	OutputCondition      OutputCondition
	OutputFunction       OutputFunction
	TerminationCondition TerminationCondition
	TimestepFunction     TimestepFunction
	Additional           map[string][]string
}

// NewStochadexConfig creates a new StochadexConfig from the provided LoadSettingsConfig
// and LoadImplementationsConfig.
func NewStochadexConfig(
	settings *LoadSettingsConfig,
	implementations *LoadImplementationsConfig,
) *StochadexConfig {
	partitions := make([]*StateConfig, 0)
	for index, iteration := range implementations.Iterations {
		partitions = append(
			partitions,
			&StateConfig{
				Iteration: iteration,
				Params: &ParamsConfig{
					Other:           settings.OtherParams[index],
					InitStateValues: settings.InitStateValues[index],
					Seed:            settings.Seeds[index],
				},
				Width:        settings.StateWidths[index],
				HistoryDepth: settings.StateHistoryDepths[index],
			},
		)
	}
	return &StochadexConfig{
		Partitions: partitions,
		Output: &OutputConfig{
			Condition: implementations.OutputCondition,
			Function:  implementations.OutputFunction,
		},
		Steps: &StepsConfig{
			TerminationCondition:  implementations.TerminationCondition,
			TimestepFunction:      implementations.TimestepFunction,
			TimestepsHistoryDepth: settings.TimestepsHistoryDepth,
		},
	}
}
