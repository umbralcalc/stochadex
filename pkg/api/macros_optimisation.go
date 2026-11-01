package api

type evolutionStrategySpec struct {
	macroTypeField `yaml:",inline"`
	Sampler        struct {
		Name    string    `yaml:"name"`
		Default []float64 `yaml:"default"`
	} `yaml:"sampler"`
	Sorting struct {
		Name           string  `yaml:"name"`
		CollectionSize int     `yaml:"collection_size"`
		EmptyValue     float64 `yaml:"empty_value,omitempty"`
	} `yaml:"sorting"`
	Mean struct {
		Name         string    `yaml:"name"`
		Default      []float64 `yaml:"default"`
		Weights      []float64 `yaml:"weights,omitempty"`
		LearningRate float64   `yaml:"learning_rate"`
	} `yaml:"mean"`
	Covariance struct {
		Name         string    `yaml:"name"`
		Default      []float64 `yaml:"default"`
		LearningRate float64   `yaml:"learning_rate"`
	} `yaml:"covariance"`
	Reward struct {
		Partition      windowedPartitionSpec `yaml:"partition"`
		DiscountFactor float64               `yaml:"discount_factor"`
	} `yaml:"reward"`
	Window   windowedPartitionsSpec `yaml:"window"`
	Seed     uint64                 `yaml:"seed"`
	Steps    int                    `yaml:"steps"`
	Timestep float64                `yaml:"timestep,omitempty"`
}
