package lob

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Default generative parameters for the flagship scenario: cryptobook's "counts
// route" model (cfg/lob_counts.yaml), the one it found best — it reproduces all three
// of the market's pooled correlation signatures and was the first to survive an
// out-of-sample test — in the four-partition modular form cfg/lob_counts_split.yaml.
//
// These are the calibrated best-model values, but they are still illustrative here, not
// a posterior with error bars: the downstream repo fits the damping exponent to a live
// Binance depth/trade feed and runs simulation-based inference around it. This stub ships
// the point values so the generative core runs with zero external inputs. See card.md and
// the downstream repo for the calibration.
const (
	// Activity driver (AR(1) latent). The Gamma mean shape/rate ≈ 4 sets the long-run
	// activity level, equal to DefaultActRef, so the damping factor (activity/act_ref)^γ
	// sits near 1 at rest and γ acts on activity FLUCTUATIONS rather than the mean.
	DefaultPersistence   = 0.8
	DefaultActivityShape = 0.152367
	DefaultActivityRate  = 0.038092

	// Order flow.
	DefaultLimitRate    = 3.381 // base limit-arrival intensity at the touch
	DefaultArrivalDecay = 0.35  // arrivals and churn thin by exp(-decay·level) with depth
	DefaultCancelRate   = 0.0   // depth-proportional cancellation rate (0 in the counts model)
	DefaultArrivalScale = 19.0  // resting depth at which damping halves the arrival rate
	DefaultChurnRate    = 1.900 // quote-churn intensity at the touch
	DefaultActRef       = 4.0   // reference activity the damping factor is measured against

	// DefaultDampingGamma is the one scientifically-interesting driver BuildStub exposes:
	// the exponent on (activity/act_ref) in the arrival-damping denominator. It is what the
	// downstream fits to the market's depth/arrival correlation, and the knob the CI test
	// sweeps. Higher γ damps arrivals harder where the book is deep, driving the depth/
	// arrival coupling negative.
	DefaultDampingGamma = 0.45

	DefaultMarketRate = 1.2 // marketable-order intensity per step
	DefaultMarketSize = 4.0 // lots per marketable order

	// DefaultEmptySpread is the sentinel spread reported when the book goes one-sided.
	DefaultEmptySpread = 99.0

	// DefaultNumSteps is the flagship horizon.
	DefaultNumSteps = 400

	// Internal constants: the marketable buy/sell split, and a disabled price-shock hook
	// (shock_step −1 never matches a step number, so no shock fires).
	half      = 0.5
	shockStep = -1.0
	shockSize = 0.0
	stepSize  = 1.0
)

// DefaultBookInit is the initial resting book: a triangular ladder of depth on each
// side (8 lots at the touch down to 1 at the back), plus a zero swept-volume slot.
var DefaultBookInit = []float64{
	8, 6, 4, 3, 2, 1, 1, 1,
	8, 6, 4, 3, 2, 1, 1, 1,
	0,
}

// DefaultObservablesInit seeds the derived observables with a plausible starting
// depth (52 lots) and spread (2 ticks) so the first recorded row is not degenerate.
var DefaultObservablesInit = []float64{0, 0, 0, 52, 2, 0}

// BuildStub constructs the data-free generative core of the limit-order-book
// microsimulation: four partitions turning a shared latent-activity driver into
// stochastic order flow, a matched book, and derived market observables.
//
//	activity     AR(1) latent driver (width 1)
//	flows        stochastic arrivals / churn / cancellations / marketable order (width 35)
//	book         deterministic matching engine: resting ladder + swept volume (width 17)
//	observables  derived counts, depth, spread, clip diagnostic (width 6)
//
// The one scientifically-interesting driver, dampingGamma, sets how hard arrivals are
// damped by resting depth — the model's stability brake and the parameter the downstream
// calibrates. It is exactly what the CI test sweeps to check the headline claim: stronger
// damping drives the depth/arrival correlation more negative.
//
// Cross-partition references are wired by name (ParamsFromUpstream for within-step reads,
// ParamsAsPartitions for the lag reads of the book), never by raw index, so pkg/graph sees
// the real dependency structure.
func BuildStub(dampingGamma float64, numSteps int, seed uint64) *simulator.ConfigGenerator {
	activity := &simulator.PartitionConfig{
		Name:      "activity",
		Iteration: &ActivityIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"persistence":    {DefaultPersistence},
			"activity_shape": {DefaultActivityShape},
			"activity_rate":  {DefaultActivityRate},
		}),
		InitStateValues:   []float64{DefaultActRef},
		StateHistoryDepth: 1,
		Seed:              seed,
	}

	flows := &simulator.PartitionConfig{
		Name:      "flows",
		Iteration: &FlowsIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"limit_rate":    {DefaultLimitRate},
			"arrival_decay": {DefaultArrivalDecay},
			"cancel_rate":   {DefaultCancelRate},
			"arrival_scale": {DefaultArrivalScale},
			"churn_rate":    {DefaultChurnRate},
			"act_ref":       {DefaultActRef},
			"damping_gamma": {dampingGamma},
			"market_rate":   {DefaultMarketRate},
			"market_size":   {DefaultMarketSize},
			"half":          {half},
			"shock_step":    {shockStep},
			"shock_size":    {shockSize},
		}),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"activity": {Upstream: "activity"},
		},
		ParamsAsPartitions: map[string][]string{
			"book_partition": {"book"},
		},
		InitStateValues:   make([]float64, 4*Levels+3),
		StateHistoryDepth: 1,
		Seed:              seed + 1,
	}

	book := &simulator.PartitionConfig{
		Name:      "book",
		Iteration: &BookIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"placeholder": {0.0},
		}),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"flow": {Upstream: "flows"},
		},
		InitStateValues:   append([]float64(nil), DefaultBookInit...),
		StateHistoryDepth: 1,
		Seed:              seed + 2,
	}

	observables := &simulator.PartitionConfig{
		Name:      "observables",
		Iteration: &ObservablesIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"empty_spread": {DefaultEmptySpread},
		}),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"flow":     {Upstream: "flows"},
			"book_now": {Upstream: "book"},
		},
		ParamsAsPartitions: map[string][]string{
			"book_partition": {"book"},
		},
		InitStateValues:   append([]float64(nil), DefaultObservablesInit...),
		StateHistoryDepth: 1,
		Seed:              seed + 3,
	}

	gen := simulator.NewConfigGenerator()
	for _, p := range []*simulator.PartitionConfig{activity, flows, book, observables} {
		gen.SetPartition(p)
	}
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition: &simulator.EveryStepOutputCondition{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: numSteps,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: stepSize},
		InitTimeValue:    0.0,
	})
	return gen
}
