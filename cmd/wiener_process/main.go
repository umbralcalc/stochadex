package main

import "github.com/umbralcalc/stochadex/pkg/simulator"

type WienerProcessIteration struct {
}

func (w *WienerProcessIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.TimestepsHistory,
) *simulator.State {
	return &simulator.State{}
}

func main() {
	config := simulator.NewStochadexConfig(
		otherParams: []OtherParams,
		initStateValues: [][]float64,
		seeds: []int,
		iterations: []Iteration,
		stateWidths: []int{1},
		stateHistoryDepths: []int{1},
		outputCondition: ,
		outputFunction: ,
		terminationCondition: ,
		timestepFunction: ,
		timestepsHistoryDepth: 1,
	)
	manager := simulator.NewPartitionManager(config)
	manager.Run()
}
