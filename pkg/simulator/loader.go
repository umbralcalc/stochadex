package simulator

type LoadSettingsConfig struct {
	OtherParams           []OtherParams
	InitStateValues       [][]float64
	Seeds                 []int
	StateWidths           []int
	StateHistoryDepths    []int
	TimestepsHistoryDepth int
}

type LoadImplementationsConfig struct {
	Iterations           []Iteration
	OutputCondition      OutputCondition
	OutputFunction       OutputFunction
	TerminationCondition TerminationCondition
	TimestepFunction     TimestepFunction
}

func NewStochadexConfig(
	settings LoadSettingsConfig,
	implementations LoadImplementationsConfig,
) *StochadexConfig {
	partitions := make([]*StateConfig, 0)
	for index, iteration := range implementations.Iterations {
		partitions = append(
			partitions,
			&StateConfig{
				Iteration: iteration,
				Params: ParamsConfig{
					Other:           settings.OtherParams[index],
					InitStateValues: settings.InitStateValues[index],
					Seed:            settings.Seeds[index],
				},
				Width:        settings.StateWidths[index],
				HistoryDepth: settings.StateHistoryDepths[index],
			},
		)
	}
	return &StochadexConfig{
		Partitions: partitions,
		Output: OutputConfig{
			Condition: implementations.OutputCondition,
			Function:  implementations.OutputFunction,
		},
		Steps: StepsConfig{
			TerminationCondition:  implementations.TerminationCondition,
			TimestepFunction:      implementations.TimestepFunction,
			TimestepsHistoryDepth: settings.TimestepsHistoryDepth,
		},
	}
}
