package stochadex

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
	PartitionByName     map[PartitionName]StateConfig
	LinkagesByPartition map[PartitionName][]PartitionName
	Output              OutputConfig
	Steps               StepsConfig
}
