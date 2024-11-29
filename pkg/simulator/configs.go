package simulator

import (
	"strconv"

	"golang.org/x/exp/rand"
)

// UpstreamConfig is the yaml-loadable representation of a slice of data
// from the output of a partitiion which is computationally upstream.
type UpstreamConfig struct {
	Upstream int   `yaml:"upstream"`
	Indices  []int `yaml:"indices,omitempty"`
}

// IterationSettings is the yaml-loadable config which defines the settings
// for an iteration which acts on a partition of the the global simulation
// state and its upstream partitions which may provide params for it.
type IterationSettings struct {
	Name               string                    `yaml:"name"`
	Params             Params                    `yaml:"params"`
	ParamsFromUpstream map[string]UpstreamConfig `yaml:"params_from_upstream,omitempty"`
	InitStateValues    []float64                 `yaml:"init_state_values"`
	Seed               uint64                    `yaml:"seed"`
	StateWidth         int                       `yaml:"state_width"`
	StateHistoryDepth  int                       `yaml:"state_history_depth"`
}

// Settings is the yaml-loadable config which defines all of the
// settings that can be set for a stochastic process defined by the
// stochadex.
type Settings struct {
	Iterations            []IterationSettings `yaml:"iterations"`
	InitTimeValue         float64             `yaml:"init_time_value"`
	TimestepsHistoryDepth int                 `yaml:"timesteps_history_depth"`
}

// Init checks to see if any of the param maps and iteration names have not
// not been populated and instantiates them if not. This is typically used
// immediately after unmarshalling from a yaml config.
func (s *Settings) Init() {
	for index, iteration := range s.Iterations {
		// ensures that a name is always given to the iteration
		if iteration.Name == "" {
			iteration.Name = strconv.Itoa(index)
		}
		iteration.Params.SetPartitionName(iteration.Name)
		// ensures the default maps are correctly instantiated from empty config
		if iteration.Params.Map == nil {
			iteration.Params.Map = make(map[string][]float64)
		}
		if iteration.ParamsFromUpstream == nil {
			iteration.ParamsFromUpstream = make(map[string]UpstreamConfig)
		}
		s.Iterations[index] = iteration
	}
}

// Implementations defines all of the interfaces that must be implemented in
// order to configure a stochastic process defined by the stochadex.
type Implementations struct {
	Iterations           []Iteration
	OutputCondition      OutputCondition
	OutputFunction       OutputFunction
	TerminationCondition TerminationCondition
	TimestepFunction     TimestepFunction
}

// NamedUpstreamConfig is the yaml-loadable representation of a slice of data
// from the output of a partitiion which is computationally upstream. This
// version uses a string name for this partition.
type NamedUpstreamConfig struct {
	Upstream string `yaml:"upstream"`
	Indices  []int  `yaml:"indices,omitempty"`
}

// PartitionConfig defines all of the configuration needed in order to
// add a partition to a stochadex simulation. This is mostly yaml-loadable,
// however the Iteration implementation needs to be inserted via templating.
type PartitionConfig struct {
	Name               string                         `yaml:"name"`
	Iteration          Iteration                      `yaml:"-"`
	Params             Params                         `yaml:"params"`
	ParamsAsPartitions map[string][]string            `yaml:"params_as_partitions,omitempty"`
	ParamsFromUpstream map[string]NamedUpstreamConfig `yaml:"params_from_upstream,omitempty"`
	InitStateValues    []float64                      `yaml:"init_state_values"`
	StateHistoryDepth  int                            `yaml:"state_history_depth"`
	Seed               uint64                         `yaml:"seed"`
}

// Init checks to see if any of the param maps have not been populated
// and instantiates them if not. This is typically used immediately after
// unmarshalling from a yaml config.
func (p *PartitionConfig) Init() {
	if p.Params.Map == nil {
		p.Params.Map = make(map[string][]float64)
	}
	if p.ParamsAsPartitions == nil {
		p.ParamsAsPartitions = make(map[string][]string)
	}
	if p.ParamsFromUpstream == nil {
		p.ParamsFromUpstream = make(map[string]NamedUpstreamConfig)
	}
}

// SimulationConfig defines all of the additional configuration needed
// in order to setup a stochadex simulation run.
type SimulationConfig struct {
	OutputCondition      OutputCondition
	OutputFunction       OutputFunction
	TerminationCondition TerminationCondition
	TimestepFunction     TimestepFunction
	InitTimeValue        float64
}

// SimulationConfigStrings defines all of the additional configuration
// needed in order to setup a stochadex simulation run. This is the
// yaml-loadable version of the config which includes string type names to
// insert into templating.
type SimulationConfigStrings struct {
	OutputCondition      string  `yaml:"output_condition"`
	OutputFunction       string  `yaml:"output_function"`
	TerminationCondition string  `yaml:"termination_condition"`
	TimestepFunction     string  `yaml:"timestep_function"`
	InitTimeValue        float64 `yaml:"init_time_value"`
}

// PartitionConfigOrdering is a structure which maintains various representations
// of the order in which partitions will be indexed in the simulation. This can
// be dynamically updated with new partitions using the .Append method.
type PartitionConfigOrdering struct {
	Names        []string
	IndexByName  map[string]int
	ConfigByName map[string]*PartitionConfig
}

// Append puts another partition into the specified ordering that it will appear
// in the simulation indexing.
func (p *PartitionConfigOrdering) Append(config *PartitionConfig) {
	_, ok := p.ConfigByName[config.Name]
	if ok {
		panic("partition with name " + config.Name + " already exists")
	}
	p.Names = append(p.Names, config.Name)
	p.IndexByName[config.Name] = len(p.Names) - 1
	p.ConfigByName[config.Name] = config
}

// ConfigGenerator enables users to dynamically configure a stochadex simulation
// programmatically while providing tools for just-in-time generation of
// the necessary Implementation and Settings configs required to create a new
// PartitionCoordinator through the .GenerateConfigs method.
type ConfigGenerator struct {
	globalSeed              uint64
	simulationConfig        *SimulationConfig
	partitionConfigOrdering *PartitionConfigOrdering
}

// GetGlobalSeed retrieves what global seed is currently set.
func (c *ConfigGenerator) GetGlobalSeed() uint64 {
	return c.globalSeed
}

// SetGlobalSeed sets a random seed for each partition in the simulation
// based on a process which itself is dependent on the input random seed.
func (c *ConfigGenerator) SetGlobalSeed(seed uint64) {
	c.globalSeed = seed
	rand.Seed(seed)
	for _, config := range c.partitionConfigOrdering.ConfigByName {
		config.Seed = uint64(rand.Intn(1e8))
	}
}

// GetSimulation retrieves the current configured simulation config
// that is in the generator.
func (c *ConfigGenerator) GetSimulation() *SimulationConfig {
	return c.simulationConfig
}

// SetSimulation sets a new simulation config in the generator.
func (c *ConfigGenerator) SetSimulation(config *SimulationConfig) {
	c.simulationConfig = config
}

// GetPartition retrieves a partition config in the generator using
// on its name.
func (c *ConfigGenerator) GetPartition(name string) *PartitionConfig {
	return c.partitionConfigOrdering.ConfigByName[name]
}

// SetPartition sets another new partition config in the generator
// which must have a unique name field to every other one which
// currently exists otherwise there is an error.
func (c *ConfigGenerator) SetPartition(config *PartitionConfig) {
	config.Init()
	c.partitionConfigOrdering.Append(config)
}

// ResetPartition allows the user to reset the config for a partition by name.
func (c *ConfigGenerator) ResetPartition(name string, config *PartitionConfig) {
	config.Init()
	_, ok := c.partitionConfigOrdering.ConfigByName[name]
	if !ok {
		panic("partition: " + name + " doesn't exist to reset")
	}
	c.partitionConfigOrdering.ConfigByName[name] = config
}

// GenerateConfigs generates the necessary Implementation and Settings configs
// required to create a new PartitionCoordinator based on the currently configured
// simulation that is represented by the generator.
func (c *ConfigGenerator) GenerateConfigs() (*Settings, *Implementations) {
	implementations := Implementations{
		Iterations:           make([]Iteration, 0),
		OutputCondition:      c.simulationConfig.OutputCondition,
		OutputFunction:       c.simulationConfig.OutputFunction,
		TerminationCondition: c.simulationConfig.TerminationCondition,
		TimestepFunction:     c.simulationConfig.TimestepFunction,
	}
	settings := Settings{
		Iterations:    make([]IterationSettings, 0),
		InitTimeValue: c.simulationConfig.InitTimeValue,
	}
	maxHistoryDepth := 0
	for _, name := range c.partitionConfigOrdering.Names {
		config := c.partitionConfigOrdering.ConfigByName[name]
		params := config.Params
		params.SetPartitionName(name)
		for paramName, partitionNames := range config.ParamsAsPartitions {
			partitionIndexValues := make([]float64, 0)
			for _, name := range partitionNames {
				if index, ok := c.partitionConfigOrdering.IndexByName[name]; ok {
					partitionIndexValues = append(
						partitionIndexValues,
						float64(index),
					)
				} else {
					panic("error converting params name: " + name +
						" into partition index - no partition by that name")
				}
			}
			params.Set(paramName, partitionIndexValues)
		}
		paramsFromUpstream := make(map[string]UpstreamConfig)
		for paramsName, partitionValues := range config.ParamsFromUpstream {
			if index, ok := c.partitionConfigOrdering.
				IndexByName[partitionValues.Upstream]; ok {
				paramsFromUpstream[paramsName] = UpstreamConfig{
					Upstream: index,
					Indices:  partitionValues.Indices,
				}
			} else {
				panic("error converting upstream name: " + partitionValues.Upstream +
					" into partition index - no partition by that name")
			}
		}
		implementations.Iterations = append(implementations.Iterations, config.Iteration)
		iterationSettings := IterationSettings{
			Name:               name,
			Params:             params,
			ParamsFromUpstream: paramsFromUpstream,
			InitStateValues:    config.InitStateValues,
			Seed:               config.Seed,
			StateWidth:         len(config.InitStateValues),
			StateHistoryDepth:  config.StateHistoryDepth,
		}
		settings.Iterations = append(settings.Iterations, iterationSettings)
		if config.StateHistoryDepth > maxHistoryDepth {
			maxHistoryDepth = config.StateHistoryDepth
		}
	}
	settings.TimestepsHistoryDepth = maxHistoryDepth
	// configure each iteration with settings now that we know its
	// assigned partition index
	for index, iteration := range implementations.Iterations {
		iteration.Configure(index, &settings)
	}
	return &settings, &implementations
}

// NewConfigGenerator creates a new ConfigGenerator.
func NewConfigGenerator() *ConfigGenerator {
	return &ConfigGenerator{
		partitionConfigOrdering: &PartitionConfigOrdering{
			Names:        make([]string, 0),
			IndexByName:  make(map[string]int),
			ConfigByName: make(map[string]*PartitionConfig),
		},
	}
}
