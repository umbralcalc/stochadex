package lob

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// Levels is the number of price levels tracked on each side of the book.
const Levels = 8

// FlowsIteration draws every stochastic order-flow event for one step: limit
// arrivals and quote churn at each of the Levels levels on both sides, the
// cancellations that churn implies, and a single marketable order split into a
// buy and a sell leg. It is the only partition besides the driver that draws
// randomness; book.go and observables.go are deterministic functions of its output.
//
// Two reads feed it, by the two distinct mechanisms the model rests on:
//   - "activity", the driver's CURRENT-step value (a params_from_upstream read), scales
//     every rate.
//   - the book's PREVIOUS-step resting depth (a lag read of the book partition's state
//     history, addressed by the "book_partition" index), which damps arrivals and clips
//     cancellations. This is a lag-1 read on purpose: the monolith this model was split
//     from damped arrivals by the previous step's depth, so reading the resting book one
//     step back is what reproduces it — and it is what keeps flows→book the only
//     within-step edge, so the wiring is not a cycle.
//
// Output layout (width 4*Levels + 3 = 35): arr_bid[Levels], arr_ask[Levels],
// can_bid[Levels], can_ask[Levels], buy_size, sell_size, clip_binds.
//
// Draw order is fixed and matches declarative.yaml's binding order exactly, so the
// two forms run on one shared stream: arr_bid (Levels Poissons, level 0..Levels-1),
// then arr_ask, then churn_bid, then churn_ask, then the cancellation Poissons on the
// bid then the ask, then one Poisson for the marketable count, then one Binomial for its
// buy/sell split. No draw is guarded away — a zero-rate Poisson still consumes its draw,
// exactly as the evaluator's elementwise draw does — which is what makes the streams align.
//
// Params: limit_rate, arrival_decay, cancel_rate, arrival_scale, churn_rate, act_ref,
// damping_gamma, market_rate, market_size, half, shock_step, shock_size.
type FlowsIteration struct {
	sampler        *rng.Sampler
	binomial       distuv.Binomial
	bookPartition  int
	arrBid, arrAsk []float64
	chBid, chAsk   []float64
	canBid, canAsk []float64
	out            []float64
}

func (f *FlowsIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	f.sampler = rng.New(settings.Iterations[partitionIndex].Seed)
	// Binomial draws from the same generator as the Poisson/Gamma methods, exactly as
	// the evaluator's `binomial` draws from c.sampler.Rand().
	f.binomial = distuv.Binomial{N: 1, P: 0.5, Src: f.sampler.Rand()}
	f.bookPartition = int(
		settings.Iterations[partitionIndex].Params.Get("book_partition")[0],
	)
	f.arrBid = make([]float64, Levels)
	f.arrAsk = make([]float64, Levels)
	f.chBid = make([]float64, Levels)
	f.chAsk = make([]float64, Levels)
	f.canBid = make([]float64, Levels)
	f.canAsk = make([]float64, Levels)
	f.out = make([]float64, 4*Levels+3)
}

func (f *FlowsIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	limitRate := params.GetIndex("limit_rate", 0)
	arrivalDecay := params.GetIndex("arrival_decay", 0)
	cancelRate := params.GetIndex("cancel_rate", 0)
	arrivalScale := params.GetIndex("arrival_scale", 0)
	churnRate := params.GetIndex("churn_rate", 0)
	actRef := params.GetIndex("act_ref", 0)
	dampingGamma := params.GetIndex("damping_gamma", 0)
	marketRate := params.GetIndex("market_rate", 0)
	marketSize := params.GetIndex("market_size", 0)
	half := params.GetIndex("half", 0)
	shockStep := params.GetIndex("shock_step", 0)
	shockSize := params.GetIndex("shock_size", 0)

	activity := params.GetIndex("activity", 0)
	dt := timestepsHistory.NextIncrement
	step := float64(timestepsHistory.CurrentStepNumber)

	// The resting book one step back: bid = [0, Levels), ask = [Levels, 2*Levels).
	book := stateHistories[f.bookPartition].Values.RawRowView(0)
	bid := book[0:Levels]
	ask := book[Levels : 2*Levels]

	// Depth damps arrivals by the same activity-dependent factor on both sides.
	damp := math.Pow(activity/actRef, dampingGamma)

	// 1. Limit arrivals, bid then ask, thinning with level depth and resting size.
	for i := 0; i < Levels; i++ {
		decay := math.Exp(-arrivalDecay * float64(i))
		f.arrBid[i] = f.sampler.Poisson(
			limitRate * decay * activity * dt / (1 + bid[i]*damp/arrivalScale))
	}
	for i := 0; i < Levels; i++ {
		decay := math.Exp(-arrivalDecay * float64(i))
		f.arrAsk[i] = f.sampler.Poisson(
			limitRate * decay * activity * dt / (1 + ask[i]*damp/arrivalScale))
	}
	// 2. Quote churn (undamped), bid then ask.
	for i := 0; i < Levels; i++ {
		f.chBid[i] = f.sampler.Poisson(churnRate * math.Exp(-arrivalDecay*float64(i)) * activity * dt)
	}
	for i := 0; i < Levels; i++ {
		f.chAsk[i] = f.sampler.Poisson(churnRate * math.Exp(-arrivalDecay*float64(i)) * activity * dt)
	}
	// 3. Cancellations = min(resting, churn + a depth-proportional Poisson), bid then ask.
	//    The Poisson is drawn per level even at cancel_rate 0 (it returns 0 after one draw),
	//    which keeps the stream identical to poisson(cancel_rate*bid*dt) over a vector.
	for i := 0; i < Levels; i++ {
		extra := f.sampler.Poisson(cancelRate * bid[i] * dt)
		f.canBid[i] = math.Min(bid[i], f.chBid[i]+extra)
	}
	for i := 0; i < Levels; i++ {
		extra := f.sampler.Poisson(cancelRate * ask[i] * dt)
		f.canAsk[i] = math.Min(ask[i], f.chAsk[i]+extra)
	}
	// 4. One marketable order, split into buy/sell legs.
	mkt := f.sampler.Poisson(marketRate * dt)
	f.binomial.N = mkt
	f.binomial.P = half
	mktBuy := f.binomial.Rand()

	buySize := mktBuy*marketSize + where(step == shockStep, shockSize, 0)
	sellSize := (mkt - mktBuy) * marketSize

	// clip_binds counts the levels where churn wanted to cancel more than was resting —
	// the cancellation clip biting, a diagnostic the observables read out.
	clipBinds := 0.0
	for i := 0; i < Levels; i++ {
		if f.chBid[i] > bid[i] {
			clipBinds++
		}
		if f.chAsk[i] > ask[i] {
			clipBinds++
		}
	}

	copy(f.out[0:Levels], f.arrBid)
	copy(f.out[Levels:2*Levels], f.arrAsk)
	copy(f.out[2*Levels:3*Levels], f.canBid)
	copy(f.out[3*Levels:4*Levels], f.canAsk)
	f.out[4*Levels] = buySize
	f.out[4*Levels+1] = sellSize
	f.out[4*Levels+2] = clipBinds
	return f.out
}

// where mirrors the DSL's scalar where(cond, a, b): the untaken branch is never used.
func where(cond bool, a, b float64) float64 {
	if cond {
		return a
	}
	return b
}
