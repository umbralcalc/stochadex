package stochadex

import "stochadex/simulator"

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
