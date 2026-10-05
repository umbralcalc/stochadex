package lob

import (
	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ActivityIteration is the shared latent-activity driver: a persistent AR(1)
// process pulled towards a Gamma-distributed innovation each step. It is the one
// source of randomness the whole book ultimately co-moves with — arrivals,
// churn and cancellations all scale with it — which is what makes quote flow on
// the two sides correlated rather than independent.
//
// State (width 1): [activity]. It reads its own previous value as row 0 of its
// state history and touches no other partition.
//
// The update is act = persistence·act_prev + (1−persistence)·gamma(shape, rate),
// matching the `shared(...)` expression in declarative.yaml exactly: one Gamma
// draw per step from a sampler seeded by the partition's own Seed. The Gamma mean
// shape/rate sets the long-run activity level (here ≈ act_ref = 4).
//
// Params:
//   - persistence: AR(1) coefficient in [0, 1); higher means slower-moving activity.
//   - activity_shape, activity_rate: Gamma shape and RATE (mean = shape/rate).
type ActivityIteration struct {
	sampler *rng.Sampler
}

func (a *ActivityIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	a.sampler = rng.New(settings.Iterations[partitionIndex].Seed)
}

func (a *ActivityIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	persistence := params.GetIndex("persistence", 0)
	shape := params.GetIndex("activity_shape", 0)
	rate := params.GetIndex("activity_rate", 0)

	actPrev := stateHistories[partitionIndex].Values.At(0, 0)
	innovation := a.sampler.Gamma(shape, rate)
	act := persistence*actPrev + (1-persistence)*innovation
	return []float64{act}
}
