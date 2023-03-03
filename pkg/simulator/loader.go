package simulator

import (
	"io/ioutil"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v2"
)

type PartitionedInitStateValues struct {
	Values []float64 `mapstructure:"values"`
}

type LoadSettingsConfig struct {
	InitStateValues       []*PartitionedInitStateValues `mapstructure:"init_state_values"`
	Seeds                 []uint64                      `mapstructure:"seeds"`
	StateWidths           []int                         `mapstructure:"state_widths"`
	StateHistoryDepths    []int                         `mapstructure:"state_history_depths"`
	TimestepsHistoryDepth int                           `mapstructure:"timesteps_history_depth"`
}

func NewLoadSettingsConfigFromYaml(path string) *LoadSettingsConfig {
	yamlFile, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var input map[string]interface{}
	err = yaml.Unmarshal(yamlFile, input)
	if err != nil {
		panic(err)
	}
	var settings LoadSettingsConfig
	err = mapstructure.Decode(input, &settings)
	if err != nil {
		panic(err)
	}
	return &settings
}

type LoadImplementationsConfig struct {
	Iterations           []Iteration
	OutputCondition      OutputCondition
	OutputFunction       OutputFunction
	TerminationCondition TerminationCondition
	TimestepFunction     TimestepFunction
}

func NewStochadexConfig(
	otherParams []OtherParams,
	settings *LoadSettingsConfig,
	implementations *LoadImplementationsConfig,
) *StochadexConfig {
	partitions := make([]*StateConfig, 0)
	for index, iteration := range implementations.Iterations {
		partitions = append(
			partitions,
			&StateConfig{
				Iteration: &iteration,
				Params: &ParamsConfig{
					Other:           &otherParams[index],
					InitStateValues: settings.InitStateValues[index].Values,
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
			Condition: &implementations.OutputCondition,
			Function:  &implementations.OutputFunction,
		},
		Steps: &StepsConfig{
			TerminationCondition:  &implementations.TerminationCondition,
			TimestepFunction:      &implementations.TimestepFunction,
			TimestepsHistoryDepth: settings.TimestepsHistoryDepth,
		},
	}
}
