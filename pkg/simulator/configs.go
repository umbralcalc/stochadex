package simulator

import (
	"strconv"

	"golang.org/x/exp/rand"
)

// PartitionConfig defines all of the configuration needed in order to
// add a partition to a stochadex simulation.
type PartitionConfig struct {
	Name                        string
	Iteration                   Iteration
	Params                      Params
	ParamsAsPartitions          map[string][]string
	ParamsFromUpstreamPartition map[string]string
	ParamsFromUpstreamIndices   map[string][]int
	InitStateValues             []float64
	Seed                        uint64
	StateWidth                  int
	StateHistoryDepth           int
}

// SimulationConfig defines all of the additional configuration needed
// in order to setup a stochadex simulation run.
type SimulationConfig struct {
	OutputCondition       OutputCondition
	OutputFunction        OutputFunction
	TerminationCondition  TerminationCondition
	TimestepFunction      TimestepFunction
	InitTimeValue         float64
	TimestepsHistoryDepth int
}

// PartitionConfigOrdering is a structure which maintains various representations
// of the order in which partitions will be indexed in the simulation. This can
// be dynamically updated with new partitions using the .Insert method.
type PartitionConfigOrdering struct {
	Names        []string
	IndexByName  map[string]int
	ConfigByName map[string]*PartitionConfig
}

// Insert puts another partition into the specified ordering that it will appear
// in the simulation indexing. The rules for ordering are based on placing
// computationally downstream partitions after their upstream dependencies.
func (p *PartitionConfigOrdering) Insert(index int, config *PartitionConfig) {
	if index < 0 || index > len(p.Names) {
		panic("inserting out of bounds at index " + strconv.Itoa(index))
	}
	_, ok := p.ConfigByName[config.Name]
	if ok {
		panic("partition with name " + config.Name + " already exists")
	}
	p.Names = append(p.Names, "")
	copy(p.Names[index+1:], p.Names[index:])
	p.Names[index] = config.Name
	p.IndexByName[config.Name] = index
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
	maxIndex := 0
	for _, upstreamName := range config.ParamsFromUpstreamPartition {
		if index, ok := c.partitionConfigOrdering.IndexByName[upstreamName]; ok {
			if maxIndex < index {
				maxIndex = index
			}
		}
	}
	c.partitionConfigOrdering.Insert(maxIndex+1, config)
}

// GenerateConfigs generates the necessary Implementation and Settings configs
// required to create a new PartitionCoordinator based on the currently configured
// simulation that is represented by the generator.
func (c *ConfigGenerator) GenerateConfigs() (Implementations, Settings) {
	implementations := Implementations{
		Partitions:           make([]Partition, 0),
		OutputCondition:      c.simulationConfig.OutputCondition,
		OutputFunction:       c.simulationConfig.OutputFunction,
		TerminationCondition: c.simulationConfig.TerminationCondition,
		TimestepFunction:     c.simulationConfig.TimestepFunction,
	}
	settings := Settings{
		Params:                make([]Params, 0),
		InitStateValues:       make([][]float64, 0),
		InitTimeValue:         c.simulationConfig.InitTimeValue,
		Seeds:                 make([]uint64, 0),
		StateWidths:           make([]int, 0),
		StateHistoryDepths:    make([]int, 0),
		TimestepsHistoryDepth: c.simulationConfig.TimestepsHistoryDepth,
	}
	for _, name := range c.partitionConfigOrdering.Names {
		config := c.partitionConfigOrdering.ConfigByName[name]
		params := config.Params
		params.SetPartitionName(name)
		for paramName, partitionNames := range config.ParamsAsPartitions {
			partitionIndexValues := make([]float64, 0)
			for _, name := range partitionNames {
				partitionIndexValues = append(
					partitionIndexValues,
					float64(c.partitionConfigOrdering.IndexByName[name]),
				)
			}
			params.Set(paramName, partitionIndexValues)
		}
		partition := Partition{
			Iteration:                   config.Iteration,
			ParamsFromUpstreamPartition: make(map[string]int),
			ParamsFromIndices:           config.ParamsFromUpstreamIndices,
		}
		for paramsName, partitionName := range config.ParamsFromUpstreamPartition {
			partition.ParamsFromUpstreamPartition[paramsName] =
				c.partitionConfigOrdering.IndexByName[partitionName]
		}
		implementations.Partitions = append(implementations.Partitions, partition)
		settings.Params = append(settings.Params, params)
		settings.InitStateValues = append(
			settings.InitStateValues,
			config.InitStateValues,
		)
		settings.Seeds = append(settings.Seeds, config.Seed)
		settings.StateWidths = append(settings.StateWidths, config.StateWidth)
		settings.StateHistoryDepths = append(
			settings.StateHistoryDepths,
			config.StateHistoryDepth,
		)
	}
	return implementations, settings
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
