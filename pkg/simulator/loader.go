package simulator

func LoadNewPartitionManagerFromConfig() *PartitionManager {
	var partitions []*StateConfig
	var outputConfig OutputConfig
	var stepsConfig StepsConfig
	config := &StochadexConfig{
		Partitions: partitions,
		Output:     outputConfig,
		Steps:      stepsConfig,
	}
	return NewPartitionManager(config)
}
