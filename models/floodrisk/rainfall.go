// Package floodrisk holds the flood-risk domain model: a data-free, SDK-built
// simulation stub of the generative core (stochastic rainfall driving a
// rainfall-runoff cascade) plus the bespoke stochadex Iterations it depends on.
//
// The iterations in this file (and runoff.go) are BESPOKE EXTENSIONS written
// against the core simulator.Iteration interface. They live beside the stub, not
// in the engine core, because that is where the catalogue stages the "should this
// be promoted into core?" question. They are lifted from the downstream project
// repo; the data-fitting / calibration helpers that accompany them there
// (parameter estimation from observed series) are inference concerns and stay
// downstream — see card.md.
package floodrisk

import (
	"math"
	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// StochasticRainfallIteration generates synthetic daily rainfall using a
// two-state Markov chain (wet/dry) with Gamma-distributed wet-day amounts.
//
// State vector: [rainfall_mm]
//
// Parameters:
//   - wet_day_shape:       Gamma shape parameter for wet-day amounts
//   - wet_day_scale:       Gamma scale parameter for wet-day amounts
//   - p_wet_given_dry:     transition probability dry→wet (P01)
//   - p_wet_given_wet:     transition probability wet→wet (P11)
//   - rainfall_multiplier: multiplicative change factor (default 1.0)
//     for climate perturbation — scales wet-day amounts
//   - wet_threshold:       threshold for wet/dry classification (mm, default 0.1)
//
// The rainfall_multiplier allows UKCP18 climate change factors to be
// applied: e.g. 1.2 means 20% increase in wet-day rainfall intensity.
// Transition probabilities can also be modified to represent changes in
// wet-day frequency under climate change.
type StochasticRainfallIteration struct {
	rng       *rand.Rand
	gammaDist distuv.Gamma
}

func (s *StochasticRainfallIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	seed := settings.Iterations[partitionIndex].Seed
	src := rand.NewPCG(seed, seed)
	s.rng = rand.New(src)
	// Gamma dist is configured per-step from params (in case params change).
	s.gammaDist = distuv.Gamma{
		Alpha: 1.0,
		Beta:  1.0,
		Src:   src,
	}
}

func (s *StochasticRainfallIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	shape := params.Map["wet_day_shape"][0]
	scale := params.Map["wet_day_scale"][0]
	p01 := params.Map["p_wet_given_dry"][0]
	p11 := params.Map["p_wet_given_wet"][0]

	multiplier := 1.0
	if m, ok := params.GetOk("rainfall_multiplier"); ok {
		multiplier = m[0]
	}

	threshold := 0.1
	if th, ok := params.GetOk("wet_threshold"); ok {
		threshold = th[0]
	}

	// Determine if previous day was wet.
	prevRainfall := stateHistories[partitionIndex].Values.At(0, 0)
	prevWet := prevRainfall > threshold

	// Markov transition: sample wet/dry for today.
	pWet := p01
	if prevWet {
		pWet = p11
	}

	todayWet := s.rng.Float64() < pWet

	if !todayWet {
		return []float64{0.0}
	}

	// Sample wet-day amount from Gamma distribution.
	s.gammaDist.Alpha = math.Max(shape, 0.01)
	s.gammaDist.Beta = 1.0 / math.Max(scale, 0.01) // gonum uses rate (1/scale)
	amount := s.gammaDist.Rand() * multiplier

	// Floor at threshold to ensure consistency.
	if amount < threshold {
		amount = threshold
	}

	return []float64{amount}
}
