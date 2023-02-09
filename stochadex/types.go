package stochadex

type PartitionName string
type StateTypeName string
type OutputConditionTypeName string
type OutputFunctionTypeName string
type TerminationConditionTypeName string
type TimestepFunctionTypeName string

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
