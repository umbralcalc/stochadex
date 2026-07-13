package anglersim

import (
	"math"
	"sync"

	"github.com/umbralcalc/stochadex/models/cardgen"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// This file holds the model's runnable expected-behaviour definitions, shared by
// two consumers: the behaviour test (which asserts each claim's direction) and
// cmd/model-graphs (which renders the observed numbers into card.md). Keeping the
// computation here — not in a _test.go file — is what lets the card show exactly
// the numbers the test asserts on, so the two can never disagree.

// runStub runs the stub to completion and returns the recorded state history for
// every partition, keyed by partition name.
func runStub(warmingTrend float64, numSteps int, seed uint64) *simulator.StateTimeStorage {
	settings, implementations := BuildStub(warmingTrend, numSteps, seed).GenerateConfigs()
	store := simulator.NewStateTimeStorage()
	implementations.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	coordinator := simulator.NewPartitionCoordinator(settings, implementations)
	var wg sync.WaitGroup
	for !coordinator.ReadyToTerminate() {
		coordinator.Step(&wg)
	}
	return store
}

// runStubOverride runs the stub with an override hook applied to the generated
// config before the run, so a behaviour can vary any partition's params without
// bloating BuildStub's signature — BuildStub still exposes only the one headline
// driver (the warming trend).
func runStubOverride(
	numSteps int,
	seed uint64,
	override func(*simulator.ConfigGenerator),
) *simulator.StateTimeStorage {
	gen := BuildStub(DefaultWarmingTrend, numSteps, seed)
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

// meanFinalLogDensity averages the population log-density over the final window of
// a run, damping the per-step process noise so the level is about the trajectory,
// not one noisy year.
func meanFinalLogDensity(rows [][]float64, window int) float64 {
	if window > len(rows) {
		window = len(rows)
	}
	sum := 0.0
	for i := len(rows) - window; i < len(rows); i++ {
		sum += rows[i][0]
	}
	return sum / float64(window)
}

// meanFinalLogDensityEnsemble averages meanFinalLogDensity across an ensemble of
// independent realisations (varying the seed) to make each claim about the
// distribution rather than a single trajectory.
func meanFinalLogDensityEnsemble(warmingTrend float64, numSteps, window, nMembers int) float64 {
	sum := 0.0
	for m := 0; m < nMembers; m++ {
		store := runStub(warmingTrend, numSteps, uint64(1000+m))
		sum += meanFinalLogDensity(store.GetValues("population"), window)
	}
	return sum / float64(nMembers)
}

// meanFinalDensityOverride returns the ensemble-mean final log-density under an
// override, averaging over both the final-window and an ensemble of seeds.
func meanFinalDensityOverride(
	numSteps, window, nMembers int,
	override func(*simulator.ConfigGenerator),
) float64 {
	sum := 0.0
	for m := 0; m < nMembers; m++ {
		store := runStubOverride(numSteps, uint64(4000+m), override)
		sum += meanFinalLogDensity(store.GetValues("population"), window)
	}
	return sum / float64(nMembers)
}

// stdFinalDensityOverride returns the across-ensemble standard deviation of the
// final log-density under an override — the spread of outcomes, used for the
// process-noise claim.
func stdFinalDensityOverride(
	numSteps, nMembers int,
	override func(*simulator.ConfigGenerator),
) float64 {
	finals := make([]float64, nMembers)
	mean := 0.0
	for m := 0; m < nMembers; m++ {
		store := runStubOverride(numSteps, uint64(7000+m), override)
		rows := store.GetValues("population")
		finals[m] = rows[len(rows)-1][0]
		mean += finals[m]
	}
	mean /= float64(nMembers)
	varSum := 0.0
	for _, v := range finals {
		d := v - mean
		varSum += d * d
	}
	return math.Sqrt(varSum / float64(nMembers))
}

// setCovariateBaseline overwrites one entry of the covariate baseline vector.
func setCovariateBaseline(gen *simulator.ConfigGenerator, index int, value float64) {
	b := gen.GetPartition("covariates").Params.Map["baseline_levels"]
	c := make([]float64, len(b))
	copy(c, b)
	c[index] = value
	gen.GetPartition("covariates").Params.Map["baseline_levels"] = c
}

// setPopulationParam overwrites a scalar param on the population partition.
func setPopulationParam(gen *simulator.ConfigGenerator, key string, value float64) {
	gen.GetPartition("population").Params.Map[key] = []float64{value}
}

// setWarmingTrend overwrites the covariate warming-trend driver.
func setWarmingTrend(gen *simulator.ConfigGenerator, value float64) {
	gen.GetPartition("covariates").Params.Map["warming_trend"] = []float64{value}
}

// ObservedBehaviour returns the model's named response claims, each with the
// ensemble numbers it produces. It is the single source of the card's "Observed
// behaviour" numbers AND the values the behaviour test asserts on, so the two can
// never disagree. Each claim's Monotone direction is what its binding test checks.
//
// The claim IDs match the subtest names under TestAnglersimExpectedBehaviour.
func ObservedBehaviour() []cardgen.Claim {
	const logDensity = "ensemble-mean final log-density"

	// Shared 60-year baseline, reused across the actionable/structural claims that
	// perturb one input against it (avoids recomputing the same base each time).
	base60 := meanFinalDensityOverride(60, 20, 12, nil)

	// Headline climate sweep at the horizon/ensemble the warming claim uses.
	warm0 := meanFinalDensityOverride(80, 20, 16, nil)
	warm4 := meanFinalDensityOverride(80, 20, 16, func(g *simulator.ConfigGenerator) { setWarmingTrend(g, 0.04) })
	warm8 := meanFinalDensityOverride(80, 20, 16, func(g *simulator.ConfigGenerator) { setWarmingTrend(g, 0.08) })

	higherFlow := meanFinalDensityOverride(60, 20, 12, func(g *simulator.ConfigGenerator) {
		setCovariateBaseline(g, 0, 2.0*DefaultBaselineFlow)
	})
	drought := meanFinalDensityOverride(60, 20, 12, func(g *simulator.ConfigGenerator) {
		setCovariateBaseline(g, 0, 0.25*DefaultBaselineFlow)
	})
	cleaner := meanFinalDensityOverride(60, 20, 12, func(g *simulator.ConfigGenerator) {
		setCovariateBaseline(g, 2, DefaultBaselineDO+3.0)
	})
	faster := meanFinalDensityOverride(60, 20, 12, func(g *simulator.ConfigGenerator) {
		setPopulationParam(g, "growth_rate", 1.0)
	})
	crowded := meanFinalDensityOverride(60, 20, 12, func(g *simulator.ConfigGenerator) {
		setPopulationParam(g, "density_dependence", 2.0)
	})

	lowNoise := stdFinalDensityOverride(60, 40, func(g *simulator.ConfigGenerator) {
		setPopulationParam(g, "process_noise_sd", 0.05)
	})
	highNoise := stdFinalDensityOverride(60, 40, func(g *simulator.ConfigGenerator) {
		setPopulationParam(g, "process_noise_sd", 0.6)
	})

	lowStart := func(g *simulator.ConfigGenerator) {
		setPopulationParam(g, "process_noise_sd", 0.001)
		g.GetPartition("population").InitStateValues = []float64{-8.0}
	}
	noAllee := meanFinalDensityOverride(8, 1, 8, func(g *simulator.ConfigGenerator) {
		lowStart(g)
		setPopulationParam(g, "allee_effect", 0.0)
	})
	withAllee := meanFinalDensityOverride(8, 1, 8, func(g *simulator.ConfigGenerator) {
		lowStart(g)
		setPopulationParam(g, "allee_effect", 30.0)
	})

	return []cardgen.Claim{
		{
			ID:        "climate_warming_reduces_density",
			Statement: "Climate warming reduces density",
			Unit:      logDensity,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "+0.00 °C/yr", Value: warm0},
				{Label: "+0.04", Value: warm4},
				{Label: "+0.08", Value: warm8},
			},
		},
		{
			ID:        "reduced_abstraction_higher_flow_raises_density",
			Statement: "Higher river flow (reduced abstraction) raises density",
			Unit:      logDensity,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "base flow", Value: base60},
				{Label: "flow ×2", Value: higherFlow},
			},
		},
		{
			ID:        "drought_lower_flow_reduces_density",
			Statement: "Drought (lower flow) reduces density",
			Unit:      logDensity,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "base flow", Value: base60},
				{Label: "flow ×0.25", Value: drought},
			},
		},
		{
			ID:        "water_quality_improvement_higher_dissolved_oxygen_raises_density",
			Statement: "Higher dissolved oxygen (pollution reduction) raises density",
			Unit:      logDensity,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "base DO", Value: base60},
				{Label: "DO +3 mg/l", Value: cleaner},
			},
		},
		{
			ID:        "higher_growth_rate_raises_density",
			Statement: "Higher intrinsic growth rate raises density",
			Unit:      logDensity,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "base r0", Value: base60},
				{Label: "r0=1.0", Value: faster},
			},
		},
		{
			ID:        "stronger_density_dependence_reduces_density",
			Statement: "Stronger density dependence reduces density",
			Unit:      logDensity,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "base α", Value: base60},
				{Label: "α=2.0", Value: crowded},
			},
		},
		{
			ID:        "higher_process_noise_widens_density_distribution",
			Statement: "Higher process noise widens the outcome distribution",
			Unit:      "ensemble std of final log-density",
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "σ=0.05", Value: lowNoise},
				{Label: "σ=0.6", Value: highNoise},
			},
		},
		{
			ID:        "allee_effect_slows_recovery_from_low_density",
			Statement: "The Allee effect slows recovery from low density",
			Unit:      logDensity + " from a low start",
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "standard Ricker", Value: noAllee},
				{Label: "Allee γ=30", Value: withAllee},
			},
		},
	}
}
