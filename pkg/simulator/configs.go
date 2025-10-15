package simulator

import (
	"strconv"

	"math/rand/v2"
)

// UpstreamConfig is the YAML-loadable representation of a slice of data
// from the output of a partition which is computationally upstream.
type UpstreamConfig struct {
	Upstream int   `yaml:"upstream"`
	Indices  []int `yaml:"indices,omitempty"`
}

// IterationSettings is the YAML-loadable per-partition configuration.
//
// Usage hints:
//   - Name is used to address partitions in other configs and params maps.
//   - ParamsFromUpstream forwards outputs from upstream partitions into Params.
//   - StateWidth and StateHistoryDepth control the size and depth of state.
type IterationSettings struct {
	Name               string                    `yaml:"name"`
	Params             Params                    `yaml:"params"`
	ParamsFromUpstream map[string]UpstreamConfig `yaml:"params_from_upstream,omitempty"`
	InitStateValues    []float64                 `yaml:"init_state_values"`
	Seed               uint64                    `yaml:"seed"`
	StateWidth         int                       `yaml:"state_width"`
	StateHistoryDepth  int                       `yaml:"state_history_depth"`
}

// Settings is the YAML-loadable top-level simulation configuration.
type Settings struct {
	Iterations            []IterationSettings `yaml:"iterations"`
	InitTimeValue         float64             `yaml:"init_time_value"`
	TimestepsHistoryDepth int                 `yaml:"timesteps_history_depth"`
}

// Init fills in defaults and ensures maps are initialised.
// Call immediately after unmarshalling from YAML.
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

// Implementations provides concrete implementations for a simulation run.
type Implementations struct {
	Iterations           []Iteration
	OutputCondition      OutputCondition
	OutputFunction       OutputFunction
	TerminationCondition TerminationCondition
	TimestepFunction     TimestepFunction
}

// NamedUpstreamConfig is like UpstreamConfig but refers to upstream by name.
type NamedUpstreamConfig struct {
	Upstream string `yaml:"upstream"`
	Indices  []int  `yaml:"indices,omitempty"`
}

// PartitionConfig defines a partition to add to a simulation.
//
// Usage hints:
//   - Iteration is not YAML-serialised; set it programmatically.
//   - ParamsAsPartitions allows passing partition indices via their names.
//   - ParamsFromUpstream forwards outputs from named upstream partitions.
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

// Init ensures params maps are initialised; call after unmarshalling YAML.
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

// SimulationConfig defines additional run-level configuration.
type SimulationConfig struct {
	OutputCondition      OutputCondition
	OutputFunction       OutputFunction
	TerminationCondition TerminationCondition
	TimestepFunction     TimestepFunction
	InitTimeValue        float64
}

// SimulationConfigStrings is the YAML-loadable version of SimulationConfig,
// referring to implementations by type names for templating.
type SimulationConfigStrings struct {
	OutputCondition      string  `yaml:"output_condition"`
	OutputFunction       string  `yaml:"output_function"`
	TerminationCondition string  `yaml:"termination_condition"`
	TimestepFunction     string  `yaml:"timestep_function"`
	InitTimeValue        float64 `yaml:"init_time_value"`
}

// PartitionConfigOrdering maintains the ordering and lookup for partitions.
// Can be updated dynamically via Append.
type PartitionConfigOrdering struct {
	Names        []string
	IndexByName  map[string]int
	ConfigByName map[string]*PartitionConfig
}

// Append inserts another partition into the ordering and updates lookups.
func (p *PartitionConfigOrdering) Append(config *PartitionConfig) {
	_, ok := p.ConfigByName[config.Name]
	if ok {
		panic("partition with name " + config.Name + " already exists")
	}
	p.Names = append(p.Names, config.Name)
	p.IndexByName[config.Name] = len(p.Names) - 1
	p.ConfigByName[config.Name] = config
}

// ConfigGenerator builds Settings and Implementations programmatically and
// can generate runnable configs on demand.
type ConfigGenerator struct {
	globalSeed              uint64
	simulationConfig        *SimulationConfig
	partitionConfigOrdering *PartitionConfigOrdering
}

// GetGlobalSeed returns the current global seed.
func (c *ConfigGenerator) GetGlobalSeed() uint64 {
	return c.globalSeed
}

// SetGlobalSeed assigns a random seed to each partition derived from the
// provided global seed.
func (c *ConfigGenerator) SetGlobalSeed(seed uint64) {
	c.globalSeed = seed
	r := rand.New(rand.NewPCG(seed, seed))
	for _, config := range c.partitionConfigOrdering.ConfigByName {
		config.Seed = uint64(r.IntN(1e8))
	}
}

// GetSimulation returns the current simulation config.
func (c *ConfigGenerator) GetSimulation() *SimulationConfig {
	return c.simulationConfig
}

// SetSimulation sets the current simulation config.
func (c *ConfigGenerator) SetSimulation(config *SimulationConfig) {
	c.simulationConfig = config
}

// GetPartition retrieves a partition config by name.
func (c *ConfigGenerator) GetPartition(name string) *PartitionConfig {
	return c.partitionConfigOrdering.ConfigByName[name]
}

// SetPartition adds a new partition config. Names must be unique.
func (c *ConfigGenerator) SetPartition(config *PartitionConfig) {
	config.Init()
	c.partitionConfigOrdering.Append(config)
}

// ResetPartition replaces the config for a partition by name.
func (c *ConfigGenerator) ResetPartition(name string, config *PartitionConfig) {
	config.Init()
	_, ok := c.partitionConfigOrdering.ConfigByName[name]
	if !ok {
		panic("partition: " + name + " doesn't exist to reset")
	}
	c.partitionConfigOrdering.ConfigByName[name] = config
}

// GenerateConfigs constructs Settings and Implementations ready to run.
// It computes state widths, converts named references, and configures
// iterations with their partition indices.
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

// NewConfigGenerator creates a new ConfigGenerator with empty ordering.
func NewConfigGenerator() *ConfigGenerator {
	return &ConfigGenerator{
		partitionConfigOrdering: &PartitionConfigOrdering{
			Names:        make([]string, 0),
			IndexByName:  make(map[string]int),
			ConfigByName: make(map[string]*PartitionConfig),
		},
	}
}
