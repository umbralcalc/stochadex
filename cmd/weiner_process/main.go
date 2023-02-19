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
	manager := simulator.LoadNewPartitionManagerFromConfig()
	manager.Run()
}
