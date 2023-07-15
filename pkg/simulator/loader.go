package simulator

import (
	"io/ioutil"

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

// LoadImplementationsConfig is the yaml-loadable config which defines all of
// the types that must be implemented in order to configure a stochastic process
// defined by the stochadex.
type LoadImplementationsConfig struct {
	Iterations                 []string           `yaml:"iterations"`
	AdditionalImplementations  []string           `yaml:"additional_implementations,omitempty"`
	OutputCondition            string             `yaml:"output_condition"`
	OutputConditionConfig      map[string]float64 `yaml:"output_condition_config,omitempty"`
	OutputFunction             string             `yaml:"output_function"`
	OutputFunctionConfig       map[string]float64 `yaml:"output_function_config,omitempty"`
	TerminationCondition       string             `yaml:"termination_condition"`
	TerminationConditionConfig map[string]float64 `yaml:"termination_condition_config,omitempty"`
	TimestepFunction           string             `yaml:"timestep_function"`
	TimestepFunctionConfig     map[string]float64 `yaml:"timestep_function_config,omitempty"`
}

// LoadImplementations defines all of the types that must be implemented in
// order to configure a stochastic process defined by the stochadex.
type LoadImplementations struct {
	Iterations           []Iteration
	OutputCondition      OutputCondition
	OutputFunction       OutputFunction
	TerminationCondition TerminationCondition
	TimestepFunction     TimestepFunction
}

// NewLoadImplementationsFromYaml creates a new LoadImplementations from
// a provided yaml path.
func NewLoadImplementationsFromYaml(path string) *LoadImplementations {
	yamlFile, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var implementationsConfig LoadImplementationsConfig
	err = yaml.Unmarshal(yamlFile, &implementationsConfig)
	if err != nil {
		panic(err)
	}
	implementations := &LoadImplementations{}
	return implementations
}

// NewStochadexConfig creates a new StochadexConfig from the provided LoadSettingsConfig
// and LoadImplementations.
func NewStochadexConfig(
	settings *LoadSettingsConfig,
	implementations *LoadImplementations,
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
