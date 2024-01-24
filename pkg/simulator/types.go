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
