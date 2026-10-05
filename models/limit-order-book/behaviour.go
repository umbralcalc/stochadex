package lob

import (
	"sync"

	"gonum.org/v1/gonum/stat"

	"github.com/umbralcalc/stochadex/models/cardgen"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// This file holds the model's runnable expected-behaviour definitions, shared by two
// consumers: the behaviour test (which asserts each claim's direction) and
// cmd/model-graphs (which renders the observed numbers into card.md). Keeping the
// computation here — not in a _test.go file — is what lets the card show exactly the
// numbers the test asserts on, so the two can never disagree.

// Observable state indices for the `observables` partition.
const (
	obsNLimit  = 0
	obsNCancel = 1
	obsNMarket = 2
	obsDepth   = 3
	obsSpread  = 4
	obsClip    = 5
)

// Behaviour-suite run knobs. Kept small so the whole suite stays a few seconds in CI.
const (
	behaviourSteps   = 400
	behaviourSkip    = 50 // discard the spin-up transient before measuring
	behaviourMembers = 8
)

// stubBuilder assembles the model, matching BuildStub's signature. The behaviour helpers
// take one rather than calling BuildStub directly so that an alternative assembly of the
// same model — notably the declarative one in declarative.yaml — can be put through this
// exact claim suite instead of a re-stated copy of it.
type stubBuilder func(dampingGamma float64, numSteps int, seed uint64) *simulator.ConfigGenerator

// runOverride runs a build to completion under an override hook applied to the generated
// config, returning the recorded state history. The override lets a behaviour vary any
// partition's params without bloating BuildStub, which still exposes only the one driver.
func runOverride(
	build stubBuilder,
	numSteps int,
	seed uint64,
	override func(*simulator.ConfigGenerator),
) *simulator.StateTimeStorage {
	gen := build(DefaultDampingGamma, numSteps, seed)
	if override != nil {
		override(gen)
	}
	settings, implementations := gen.GenerateConfigs()
	store := simulator.NewStateTimeStorage()
	implementations.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	coordinator := simulator.NewPartitionCoordinator(settings, implementations)
	var wg sync.WaitGroup
	for !coordinator.ReadyToTerminate() {
		coordinator.Step(&wg)
	}
	return store
}

// setFlowParam overrides one scalar param on the flows partition — the swept surface for
// every structural claim here.
func setFlowParam(key string, value float64) func(*simulator.ConfigGenerator) {
	return func(g *simulator.ConfigGenerator) {
		g.GetPartition("flows").Params.Map[key] = []float64{value}
	}
}

// observableSeries pulls one observable column past the spin-up window.
func observableSeries(store *simulator.StateTimeStorage, idx int) []float64 {
	rows := store.GetValues("observables")
	out := make([]float64, 0, len(rows)-behaviourSkip)
	for i := behaviourSkip; i < len(rows); i++ {
		out = append(out, rows[i][idx])
	}
	return out
}

// meanObservableEnsemble averages one observable's post-spin-up mean over an ensemble of
// seeds, under a flows-param override, so each claim is about the distribution not a run.
func meanObservableEnsemble(build stubBuilder, idx int, override func(*simulator.ConfigGenerator), seedBase uint64) float64 {
	total := 0.0
	for m := 0; m < behaviourMembers; m++ {
		store := runOverride(build, behaviourSteps, seedBase+uint64(m), override)
		series := observableSeries(store, idx)
		s := 0.0
		for _, v := range series {
			s += v
		}
		total += s / float64(len(series))
	}
	return total / float64(behaviourMembers)
}

// corrDepthArrivalsEnsemble averages the Pearson correlation between resting depth and
// limit-arrival count over an ensemble — the model's headline stability signature.
func corrDepthArrivalsEnsemble(build stubBuilder, gamma float64, seedBase uint64) float64 {
	total := 0.0
	for m := 0; m < behaviourMembers; m++ {
		store := runOverride(build, behaviourSteps, seedBase+uint64(m), setFlowParam("damping_gamma", gamma))
		depth := observableSeries(store, obsDepth)
		arrivals := observableSeries(store, obsNLimit)
		total += stat.Correlation(depth, arrivals, nil)
	}
	return total / float64(behaviourMembers)
}

// ObservedBehaviour returns the model's named response claims, each with the ensemble
// numbers it produces. It is the single source of the card's "Observed behaviour" numbers
// AND the values the behaviour test asserts on, so the two can never disagree. Each claim's
// Monotone direction is what its binding test checks.
//
// The claim IDs match the subtest names under TestLimitOrderBookExpectedBehaviour.
func ObservedBehaviour() []cardgen.Claim {
	return observedBehaviour(BuildStub)
}

// observedBehaviour computes the claims against a given assembly of the model. The
// equivalence test runs it against the declarative build so the data-only model answers
// the same claims, measured the same way.
func observedBehaviour(build stubBuilder) []cardgen.Claim {
	corrUnit := "ensemble-mean corr(resting depth, limit arrivals)"
	depthUnit := "ensemble-mean resting depth (lots)"
	spreadUnit := "ensemble-mean spread (ticks)"
	cancelUnit := "ensemble-mean cancellations per step (lots)"
	sweptUnit := "ensemble-mean swept volume per step (lots)"

	return []cardgen.Claim{
		{
			ID:        "stronger_damping_makes_the_depth_arrival_coupling_more_negative",
			Statement: "Stronger activity-dependent damping drives the depth/arrival coupling more negative (headline driver)",
			Unit:      corrUnit,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "γ=0.0", Value: corrDepthArrivalsEnsemble(build, 0.0, 1000)},
				{Label: "γ=0.45", Value: corrDepthArrivalsEnsemble(build, 0.45, 1000)},
				{Label: "γ=0.9", Value: corrDepthArrivalsEnsemble(build, 0.9, 1000)},
			},
		},
		{
			ID:        "higher_arrival_intensity_deepens_the_book",
			Statement: "Higher limit-arrival intensity deepens the resting book",
			Unit:      depthUnit,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "limit_rate=2.0", Value: meanObservableEnsemble(build, obsDepth, setFlowParam("limit_rate", 2.0), 2000)},
				{Label: "3.381", Value: meanObservableEnsemble(build, obsDepth, setFlowParam("limit_rate", 3.381), 2000)},
				{Label: "5.0", Value: meanObservableEnsemble(build, obsDepth, setFlowParam("limit_rate", 5.0), 2000)},
			},
		},
		{
			ID:        "heavier_marketable_flow_thins_the_book",
			Statement: "Heavier marketable-order flow thins the resting book",
			Unit:      depthUnit,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "market_rate=0.6", Value: meanObservableEnsemble(build, obsDepth, setFlowParam("market_rate", 0.6), 3000)},
				{Label: "1.2", Value: meanObservableEnsemble(build, obsDepth, setFlowParam("market_rate", 1.2), 3000)},
				{Label: "2.4", Value: meanObservableEnsemble(build, obsDepth, setFlowParam("market_rate", 2.4), 3000)},
			},
		},
		{
			ID:        "heavier_marketable_flow_widens_the_spread",
			Statement: "Heavier marketable-order flow widens the bid–ask spread",
			Unit:      spreadUnit,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "market_rate=0.6", Value: meanObservableEnsemble(build, obsSpread, setFlowParam("market_rate", 0.6), 4000)},
				{Label: "1.2", Value: meanObservableEnsemble(build, obsSpread, setFlowParam("market_rate", 1.2), 4000)},
				{Label: "2.4", Value: meanObservableEnsemble(build, obsSpread, setFlowParam("market_rate", 2.4), 4000)},
			},
		},
		{
			ID:        "faster_quote_churn_raises_cancellations",
			Statement: "Faster quote churn raises the cancellation count",
			Unit:      cancelUnit,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "churn_rate=1.0", Value: meanObservableEnsemble(build, obsNCancel, setFlowParam("churn_rate", 1.0), 5000)},
				{Label: "1.9", Value: meanObservableEnsemble(build, obsNCancel, setFlowParam("churn_rate", 1.9), 5000)},
				{Label: "3.0", Value: meanObservableEnsemble(build, obsNCancel, setFlowParam("churn_rate", 3.0), 5000)},
			},
		},
		{
			ID:        "larger_marketable_orders_sweep_more_volume",
			Statement: "Larger marketable orders sweep more volume from the book",
			Unit:      sweptUnit,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "market_size=2.0", Value: meanObservableEnsemble(build, obsNMarket, setFlowParam("market_size", 2.0), 6000)},
				{Label: "4.0", Value: meanObservableEnsemble(build, obsNMarket, setFlowParam("market_size", 4.0), 6000)},
				{Label: "8.0", Value: meanObservableEnsemble(build, obsNMarket, setFlowParam("market_size", 8.0), 6000)},
			},
		},
	}
}
