package solarfleet

// FleetSkyIteration is the full-covariance form (b): one wide partition carrying
// the whole fleet's clear-sky index and generation. Its state is
// [logK_0..S-1, gen_0..S-1] (width 2S). Each step it draws one innovation vector
// xi ~ N(0,1)^S, correlates it with the flattened Cholesky factor L (innov = L·xi),
// advances an isotropic OU on log K, and converts to per-site generation against the
// clear-sky driver. Lifted from the downstream repo (solarfleet/compose.py's
// build_covariance_config), whose DSL twin proves this is config-expressible.
//
// Draw order is load-bearing for the declarative twin: the S normals of xi are drawn
// first, in order, exactly as `iid(S, normal(0,1))` does, before any are used — so
// the bespoke stream and the evaluator's stream align exactly.

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

type FleetSkyIteration struct {
	sampler  *rng.Sampler
	numSites int
	xi       []float64
	out      []float64
}

func (f *FleetSkyIteration) Configure(partitionIndex int, settings *simulator.Settings) {
	f.sampler = rng.New(settings.Iterations[partitionIndex].Seed)
	// The state is [logK_0..S-1, gen_0..S-1]; its width fixes S once at setup.
	f.numSites = len(settings.Iterations[partitionIndex].InitStateValues) / 2
	f.xi = make([]float64, f.numSites)
	f.out = make([]float64, 2*f.numSites)
}

func clamp(x, lo, hi float64) float64 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

func (f *FleetSkyIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	s := f.numSites
	theta := params.GetIndex("theta", 0)
	iStc := params.GetIndex("i_stc", 0)
	kMax := params.GetIndex("k_max", 0)
	mu := params.Get("mu")
	kwp := params.Get("kwp")
	eta := params.Get("eta")
	lflat := params.Get("lflat")
	poa := params.Get("poa") // per-site clear-sky POA, within-step from clearsky
	dt := timestepsHistory.NextIncrement
	sqrtDt := math.Sqrt(dt)

	// Own previous committed row (lag-1 self read): the first S entries are log K.
	prev := stateHistories[partitionIndex].Values.RawRowView(0)

	// Draw the whole innovation vector once, in order — this must precede any use so
	// the stream matches `iid(S, normal(0,1))` in the twin.
	for i := 0; i < s; i++ {
		f.xi[i] = f.sampler.Normal(0, 1)
	}

	for i := 0; i < s; i++ {
		// innov_i = (L · xi)_i, with L flattened row-major.
		innov := 0.0
		row := lflat[i*s : i*s+s]
		for j := 0; j < s; j++ {
			innov += row[j] * f.xi[j]
		}
		logkNext := prev[i] + theta*(mu[i]-prev[i])*dt + sqrtDt*innov
		kidx := clamp(math.Exp(logkNext), 0, kMax)
		f.out[i] = logkNext
		f.out[s+i] = clamp(kwp[i]*poa[i]/iStc*kidx*eta[i], 0, 1e12)
	}
	return f.out
}
