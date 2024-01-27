package simulator

import (
	"gonum.org/v1/gonum/mat"
)

// OtherParams is a yaml-loadable struct to put any additional
// parameters needed to configure the stochastic process.
type OtherParams struct {
	FloatParams     map[string][]float64 `yaml:"float_params"`
	IntParams       map[string][]int64   `yaml:"int_params"`
	FloatParamsMask map[string][]bool    `yaml:"float_params_mask,omitempty"`
	IntParamsMask   map[string][]bool    `yaml:"int_params_mask,omitempty"`
}

// ParamsConfig contains all the hyperparameters of the stochastic process.
type ParamsConfig struct {
	Other           *OtherParams
	InitStateValues []float64
	InitTimeValue   float64
	Seed            uint64
}

// StateConfig completely configures a given state partition of the
// full stochastic process.
type StateConfig struct {
	Iteration    Iteration
	Params       *ParamsConfig
	Width        int
	HistoryDepth int
}

// StepsConfig completely configures all of the necessary information
// required to specify how the stochastic process evolves (steps) in time.
type StepsConfig struct {
	TerminationCondition  TerminationCondition
	TimestepFunction      TimestepFunction
	TimestepsHistoryDepth int
}

// OutputConfig completely specifies how each state partition outputs
// information to the user.
type OutputConfig struct {
	Condition OutputCondition
	Function  OutputFunction
}

// StochadexConfig fully configures a stochastic process implemented
// in the stochadex.
type StochadexConfig struct {
	Partitions []*StateConfig
	Output     *OutputConfig
	Steps      *StepsConfig
}

// StateHistory represents the information contained within a windowed
// history of State structs.
type StateHistory struct {
	// each row is a different state in the history, by convention,
	// starting with the most recent at index = 0
	Values            *mat.Dense
	StateWidth        int
	StateHistoryDepth int
}

// CumulativeTimestepsHistory is a windowed history of cumulative timestep values
// which includes the next value to increment time by.
type CumulativeTimestepsHistory struct {
	NextIncrement     float64
	Values            *mat.VecDense
	StateHistoryDepth int
}

// IteratorInputMessage defines the message which is passed from the
// PartitionCoordinator to a StateIterator of a given partition when
// the former is requesting the latter to perform a job.
type IteratorInputMessage struct {
	StateHistories   []*StateHistory
	TimestepsHistory *CumulativeTimestepsHistory
}

// Implementations defines all of the types that must be implemented in
// order to configure a stochastic process defined by the stochadex.
type Implementations struct {
	Iterations           []Iteration
	OutputCondition      OutputCondition
	OutputFunction       OutputFunction
	TerminationCondition TerminationCondition
	TimestepFunction     TimestepFunction
}

// ImplementationStrings is the yaml-loadable config which consists of string type
// names to insert into templating.
type ImplementationStrings struct {
	Iterations           []string `yaml:"iterations"`
	OutputCondition      string   `yaml:"output_condition"`
	OutputFunction       string   `yaml:"output_function"`
	TerminationCondition string   `yaml:"termination_condition"`
	TimestepFunction     string   `yaml:"timestep_function"`
}

// Settings is the yaml-loadable config which defines all of the
// settings that can be set for a stochastic process defined by the
// stochadex.
type Settings struct {
	OtherParams           []*OtherParams `yaml:"other_params"`
	InitStateValues       [][]float64    `yaml:"init_state_values"`
	InitTimeValue         float64        `yaml:"init_time_value"`
	Seeds                 []uint64       `yaml:"seeds"`
	StateWidths           []int          `yaml:"state_widths"`
	StateHistoryDepths    []int          `yaml:"state_history_depths"`
	TimestepsHistoryDepth int            `yaml:"timesteps_history_depth"`
}
