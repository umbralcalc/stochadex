package simulator

import "gonum.org/v1/gonum/mat"

type OtherParams struct {
	FloatParams map[string][]float64 `mapstructure:"float_params"`
	IntParams   map[string][]int64   `mapstructure:"int_params"`
}

type ParamsConfig struct {
	Other           *OtherParams
	InitStateValues []float64
	Seed            uint64
}

type StateConfig struct {
	Iteration    *Iteration
	Params       *ParamsConfig
	Width        int
	HistoryDepth int
}

type StepsConfig struct {
	TerminationCondition  *TerminationCondition
	TimestepFunction      *TimestepFunction
	TimestepsHistoryDepth int
}

type OutputConfig struct {
	Condition *OutputCondition
	Function  *OutputFunction
}

type StochadexConfig struct {
	Partitions []*StateConfig
	Output     *OutputConfig
	Steps      *StepsConfig
}

type State struct {
	Values     *mat.VecDense
	StateWidth int
}

type StateHistory struct {
	// each row is a different state in the history, by convention,
	// starting with the most recent at index = 0
	Values            *mat.Dense
	StateWidth        int
	StateHistoryDepth int
}

type TimestepsHistory struct {
	Values            *mat.VecDense
	StateHistoryDepth int
}

type IteratorOutputMessage struct {
	PartitionIndex int
	State          *State
}
