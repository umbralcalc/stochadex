package simulator

import (
	"gonum.org/v1/gonum/mat"
)

// Params is a type alias for the parameters needed to configure
// the stochastic process.
type Params map[string][]float64

// StateHistory represents the information contained within a windowed
// history of []float64 state values.
type StateHistory struct {
	// each row is a different state in the history, by convention,
	// starting with the most recent at index = 0
	Values *mat.Dense
	// should be of length = StateWidth
	NextValues        []float64
	StateWidth        int
	StateHistoryDepth int
}

// CumulativeTimestepsHistory is a windowed history of cumulative timestep values
// which includes the next value to increment time by and number of steps taken.
type CumulativeTimestepsHistory struct {
	NextIncrement     float64
	Values            *mat.VecDense
	CurrentStepNumber int
	StateHistoryDepth int
}

// IteratorInputMessage defines the message which is passed from the
// PartitionCoordinator to a StateIterator of a given partition when
// the former is requesting the latter to perform a job.
type IteratorInputMessage struct {
	StateHistories   []*StateHistory
	TimestepsHistory *CumulativeTimestepsHistory
}

// Partition is the config which defines an iteration which acts on a
// partition of the the global simulation state and its upstream partitions
// which may provide params for it.
type Partition struct {
	Iteration                   Iteration
	ParamsFromUpstreamPartition map[string]int
	ParamsFromIndices           map[string][]int
}

// Implementations defines all of the types that must be implemented in
// order to configure a stochastic process defined by the stochadex.
type Implementations struct {
	Partitions           []Partition
	OutputCondition      OutputCondition
	OutputFunction       OutputFunction
	TerminationCondition TerminationCondition
	TimestepFunction     TimestepFunction
}

// PartitionStrings is the yaml-loadable config for a Partition.
type PartitionStrings struct {
	Iteration                   string           `yaml:"iteration"`
	ParamsFromUpstreamPartition map[string]int   `yaml:"params_from_upstream_partition,omitempty"`
	ParamsFromIndices           map[string][]int `yaml:"params_from_indices,omitempty"`
}

// ImplementationStrings is the yaml-loadable config which consists of string type
// names to insert into templating.
type ImplementationStrings struct {
	Partitions           []PartitionStrings `yaml:"partitions"`
	OutputCondition      string             `yaml:"output_condition"`
	OutputFunction       string             `yaml:"output_function"`
	TerminationCondition string             `yaml:"termination_condition"`
	TimestepFunction     string             `yaml:"timestep_function"`
}

// Settings is the yaml-loadable config which defines all of the
// settings that can be set for a stochastic process defined by the
// stochadex.
type Settings struct {
	Params                []Params    `yaml:"params"`
	InitStateValues       [][]float64 `yaml:"init_state_values"`
	InitTimeValue         float64     `yaml:"init_time_value"`
	Seeds                 []uint64    `yaml:"seeds"`
	StateWidths           []int       `yaml:"state_widths"`
	StateHistoryDepths    []int       `yaml:"state_history_depths"`
	TimestepsHistoryDepth int         `yaml:"timesteps_history_depth"`
}
