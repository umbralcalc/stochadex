package simulator

func NewStochadexConfig(
	otherParams []OtherParams,
	initStateValues [][]float64,
	seeds []int,
	iterations []Iteration,
	stateWidths []int,
	stateHistoryDepths []int,
	outputCondition OutputCondition,
	outputFunction OutputFunction,
	terminationCondition TerminationCondition,
	timestepFunction TimestepFunction,
	timestepsHistoryDepth int,
) *StochadexConfig {
	partitions := make([]*StateConfig, 0)
	for index, iteration := range iterations {
		partitions = append(
			partitions,
			&StateConfig{
				Iteration: iteration,
				Params: ParamsConfig{
					Other:           otherParams,
					InitStateValues: initStateValues[index],
					Seed:            seeds[index],
				},
				Width:        stateWidths[index],
				HistoryDepth: stateHistoryDepths[index],
			},
		)
	}
	return &StochadexConfig{
		Partitions: partitions,
		Output: OutputConfig{
			Condition: outputCondition,
			Function:  outputFunction,
		},
		Steps: StepsConfig{
			TerminationCondition:  terminationCondition,
			TimestepFunction:      timestepFunction,
			TimestepsHistoryDepth: timestepsHistoryDepth,
		},
	}
}
