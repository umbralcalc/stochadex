package solarfleet

// Data-free SDK stub of the aggregate solar-PV fleet generative core (the
// full-covariance form): a deterministic clear-sky driver, a distance-coupled
// stochastic clear-sky-index field, and the fleet aggregate. Lifted from the
// downstream repo (https://github.com/umbralcalc/solar-fleet), whose forward model
// is itself pure stochadex config — so this Go stub and the declarative.yaml twin
// are two expressions of one model.
//
// Partition graph (clearsky is replayed as data; sites reads it within-step and
// reads its own previous row lag-1; fleet sums this-step generation):
//
//	clearsky ──poa──▶ sites ──s──▶ fleet
//	                    ▲ │
//	                    └─┘ (lag-1 self read of log K)
//
// The one scientifically-interesting driver BuildStub exposes is cloud volatility;
// every other input is a Default* constant. These are illustrative, not calibrated.

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

const stepSize = 1.0

// BuildStub assembles the fleet forward model over numSteps steps at the given
// cloud volatility (the swept driver), using the illustrative DefaultFleet.
func BuildStub(cloudVolatility float64, numSteps int, seed uint64) *simulator.ConfigGenerator {
	sites := DefaultFleet()
	kernel := DefaultKernel(cloudVolatility)
	poaRows := ClearSkyDriverRows(sites, numSteps)
	return buildFleet(sites, kernel, poaRows, numSteps, seed)
}

// buildFleet is the shared assembler behind BuildStub and the behaviour helpers: it
// wires a given fleet, kernel and clear-sky driver into the three-partition graph.
// The fleet, kernel and driver together carry every site-dependent input as data,
// so a different fleet (e.g. a compact one) is a data change, not a wiring change.
func buildFleet(
	sites []Site,
	kernel CouplingKernel,
	poaRows [][]float64,
	numSteps int,
	seed uint64,
) *simulator.ConfigGenerator {
	s := len(sites)
	mu := make([]float64, s)
	kwp := make([]float64, s)
	eta := make([]float64, s)
	for i, site := range sites {
		mu[i], kwp[i], eta[i] = site.Mu, site.Kwp, site.Eta
	}
	lflat := CholeskyFactorFlat(sites, kernel)

	// Init row: log K at its stationary mean, generation evaluated there.
	poa0 := poaRows[0]
	sitesInit := make([]float64, 2*s)
	fleet0 := 0.0
	for i := 0; i < s; i++ {
		kidx := clamp(math.Exp(mu[i]), 0, DefaultKMax)
		gen0 := clamp(kwp[i]*poa0[i]/IStandardTestConditions*kidx*eta[i], 0, 1e12)
		sitesInit[i] = mu[i]
		sitesInit[s+i] = gen0
		fleet0 += gen0
	}

	// clearsky: replay the precomputed per-site POA driver by step number.
	clearsky := &simulator.PartitionConfig{
		Name:              "clearsky",
		Iteration:         &general.FromStorageIteration{Data: poaRows},
		Params:            simulator.NewParams(map[string][]float64{}),
		InitStateValues:   append([]float64(nil), poa0...),
		StateHistoryDepth: 1,
		Seed:              seed,
	}

	// sites: the wide full-covariance clear-sky-index field.
	sitesPart := &simulator.PartitionConfig{
		Name:      "sites",
		Iteration: &FleetSkyIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"theta": {DefaultReversion},
			"mu":    mu,
			"kwp":   kwp,
			"eta":   eta,
			"i_stc": {IStandardTestConditions},
			"k_max": {DefaultKMax},
			"lflat": lflat,
		}),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"poa": {Upstream: "clearsky"}, // within-step per-site clear-sky POA
		},
		InitStateValues:   sitesInit,
		StateHistoryDepth: 1,
		Seed:              seed + 1,
	}

	// fleet: aggregate generation (a first-class state).
	fleet := &simulator.PartitionConfig{
		Name:      "fleet",
		Iteration: &FleetAggregateIteration{},
		Params:    simulator.NewParams(map[string][]float64{}),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"s": {Upstream: "sites"},
		},
		InitStateValues:   []float64{fleet0},
		StateHistoryDepth: 1,
		Seed:              seed + 2,
	}

	gen := simulator.NewConfigGenerator()
	for _, p := range []*simulator.PartitionConfig{clearsky, sitesPart, fleet} {
		gen.SetPartition(p)
	}
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: numSteps},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: stepSize},
		InitTimeValue:        0.0,
	})
	return gen
}
