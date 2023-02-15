package stochadex

import "gonum.org/v1/gonum/mat"

type TypeName string
type PartitionName string
type StateTypeName TypeName
type OutputConditionTypeName TypeName
type OutputFunctionTypeName TypeName
type TerminationConditionTypeName TypeName
type TimestepFunctionTypeName TypeName

type ParamsConfig struct {
	InitStateValues []float64
	Seed            int
}

type StateConfig struct {
	TypeName     StateTypeName
	Params       ParamsConfig
	Width        int
	HistoryDepth int
}

type StepsConfig struct {
	TerminationCondition TerminationConditionTypeName
	TimestepFunction     TimestepFunctionTypeName
}

type OutputConfig struct {
	Condition OutputConditionTypeName
	Function  OutputFunctionTypeName
}

type StochadexConfig struct {
	PartitionByName map[PartitionName]StateConfig
	Output          OutputConfig
	Steps           StepsConfig
}

type State struct {
	Values     *mat.VecDense
	StateWidth int
}

type StateHistory struct {
	// each row is a different state in the history, by convention,
	// starting with the most recent
	Values            *mat.Dense
	StateWidth        int
	StateHistoryDepth int
}
