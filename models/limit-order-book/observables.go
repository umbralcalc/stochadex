package lob

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ObservablesIteration derives the market summaries a downstream analysis reads —
// arrival, cancellation and marketable counts, the resting depth the flows were drawn
// against, the bid–ask spread, and the cancellation-clip diagnostic. It draws nothing.
//
// It is the partition that reaches another (book) by BOTH mechanisms at once: the book
// at its CURRENT step (a params_from_upstream read, "book_now") for the post-update
// ladder the spread is read off, and the book at its PREVIOUS step (a lag read via
// "book_partition") for the pre-update depth the arrivals responded to. Needing both the
// before and the after of one partition is what this partition exists to demonstrate.
//
// State (width 6): [n_limit, n_cancel, n_market, depth_start, spread_ticks, clip_binds].
//
// Params: empty_spread (the sentinel spread when the book is one-sided).
type ObservablesIteration struct {
	bookPartition int
}

func (o *ObservablesIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	o.bookPartition = int(
		settings.Iterations[partitionIndex].Params.Get("book_partition")[0],
	)
}

func (o *ObservablesIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	flow := params.Get("flow")
	bookNow := params.Get("book_now")
	emptySpread := params.GetIndex("empty_spread", 0)

	// The resting book one step back, the depth the arrivals this step were damped by.
	bookPrev := stateHistories[o.bookPartition].Values.RawRowView(0)

	nLimit := sum(flow[0 : 2*Levels])
	nCancel := sum(flow[2*Levels : 4*Levels])
	nMarket := bookNow[2*Levels] // the book's swept volume
	depthStart := sum(bookPrev[0 : 2*Levels])

	// Best (nearest-the-touch) occupied level on each side, and whether both sides quote.
	postBid := bookNow[0:Levels]
	postAsk := bookNow[Levels : 2*Levels]
	bestBid, bidOccupied := bestLevel(postBid)
	bestAsk, askOccupied := bestLevel(postAsk)

	spread := emptySpread
	if bidOccupied && askOccupied {
		spread = float64(bestBid+bestAsk) + 2
	}

	clipBinds := flow[4*Levels+2]

	return []float64{nLimit, nCancel, nMarket, depthStart, spread, clipBinds}
}

// bestLevel returns the index of the first occupied level (nearest the touch) and
// whether any level is occupied, matching the first-occupied selector the DSL builds
// from occ*front and reads out with dot(idx, ...).
func bestLevel(side []float64) (int, bool) {
	for i, v := range side {
		if v > 0 {
			return i, true
		}
	}
	return 0, false
}

// sum totals a slice, mirroring the DSL sum(...).
func sum(xs []float64) float64 {
	total := 0.0
	for _, x := range xs {
		total += x
	}
	return total
}
