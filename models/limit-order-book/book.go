package lob

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// BookIteration is the deterministic matching engine: given the flows drawn this
// step and the resting book from last step, it applies arrivals and cancellations
// and walks each marketable order across price levels, consuming resting volume
// from the front until the order is exhausted. It draws no randomness — the book is
// a pure function of the flows — which is exactly why splitting it into its own
// partition costs nothing (the driver split is the only boundary that moves a number).
//
// State (width 2*Levels + 1 = 17): bid[Levels], ask[Levels], swept. bid/ask are the
// resting depth at each level; swept is the total volume matched by marketable orders
// this step. It reads the flows at the CURRENT step (a params_from_upstream read,
// "flow") and its own resting book as row 0 of its state history.
//
// Params: none of its own (a placeholder param keeps the partition well-formed).
type BookIteration struct {
	restBid, restAsk []float64
	out              []float64
}

func (b *BookIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	b.restBid = make([]float64, Levels)
	b.restAsk = make([]float64, Levels)
	b.out = make([]float64, 2*Levels+1)
}

func (b *BookIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	flow := params.Get("flow")
	arrBid := flow[0:Levels]
	arrAsk := flow[Levels : 2*Levels]
	canBid := flow[2*Levels : 3*Levels]
	canAsk := flow[3*Levels : 4*Levels]
	buySize := flow[4*Levels]
	sellSize := flow[4*Levels+1]

	prev := stateHistories[partitionIndex].Values.RawRowView(0)
	bid := prev[0:Levels]
	ask := prev[Levels : 2*Levels]

	// Resting depth after cancellations leave and arrivals join.
	for i := 0; i < Levels; i++ {
		b.restBid[i] = bid[i] - canBid[i] + arrBid[i]
		b.restAsk[i] = ask[i] - canAsk[i] + arrAsk[i]
	}

	// Walk each marketable order across levels: a sell eats bids, a buy eats asks,
	// front to back, taking up to the resting depth at each level after the volume
	// already consumed in front of it.
	swept := 0.0
	cumBid, cumAsk := 0.0, 0.0
	for i := 0; i < Levels; i++ {
		hitBid := clamp(sellSize-cumBid, 0, b.restBid[i])
		hitAsk := clamp(buySize-cumAsk, 0, b.restAsk[i])
		b.out[i] = b.restBid[i] - hitBid
		b.out[Levels+i] = b.restAsk[i] - hitAsk
		swept += hitBid + hitAsk
		cumBid += b.restBid[i]
		cumAsk += b.restAsk[i]
	}
	b.out[2*Levels] = swept
	return b.out
}

// clamp mirrors the DSL clamp(x, lo, hi) = min(max(x, lo), hi) exactly, including its
// behaviour when hi < lo (the high bound wins) — so the two forms agree even on inputs the
// live model never produces. In the model rest ≥ 0 always (cancellations never exceed
// resting depth), so hi ≥ lo and this is an ordinary clamp.
func clamp(x, lo, hi float64) float64 {
	return math.Min(math.Max(x, lo), hi)
}
