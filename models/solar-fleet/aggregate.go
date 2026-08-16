package solarfleet

// FleetAggregateIteration sums per-site generation into the fleet total — a
// first-class state so the aggregate is observable directly. It reads the sites
// partition's whole this-step row within-step (params_from_upstream) and sums its
// generation half. Deterministic; it draws nothing. Lifted from the downstream
// repo's fleet partition (solarfleet/compose.py).

import "github.com/umbralcalc/stochadex/pkg/simulator"

type FleetAggregateIteration struct{}

func (f *FleetAggregateIteration) Configure(partitionIndex int, settings *simulator.Settings) {}

func (f *FleetAggregateIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	s := params.Get("s") // sites' this-step row [logK_0..S-1, gen_0..S-1]
	half := len(s) / 2
	total := 0.0
	for i := half; i < len(s); i++ {
		total += s[i]
	}
	return []float64{total}
}
