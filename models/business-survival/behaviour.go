package bizsurvival

import (
	"sync"

	"github.com/umbralcalc/stochadex/models/cardgen"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// This file holds the model's runnable expected-behaviour definitions, shared by
// two consumers: the behaviour test (which verifies each claim's direction) and
// cmd/model-graphs (which renders the observed numbers into card.md). Keeping the
// computation here — not in a _test.go file — is what lets the card show exactly
// the numbers the test asserts on, so the two can never disagree.

// stubBuilder assembles the model, matching BuildStub's signature. The behaviour
// helpers take one rather than calling BuildStub directly so that an alternative
// assembly of the same model — notably the declarative one in declarative.yaml —
// can be put through this exact claim suite instead of a re-stated copy of it.
type stubBuilder func(hazardScale float64, numSteps int, seed uint64) *simulator.ConfigGenerator

// runStubOverride runs the stub with an override hook applied to the generated
// config before the run, so a behaviour can vary any partition's params (policy
// multipliers, elasticities, covariates, survival curve) without bloating
// BuildStub's signature — BuildStub still exposes only the one headline driver.
func runStubOverride(
	build stubBuilder,
	hazardScale float64,
	numSteps int,
	seed uint64,
	override func(*simulator.ConfigGenerator),
) *simulator.StateTimeStorage {
	gen := build(hazardScale, numSteps, seed)
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

// setParam overwrites a single scalar param on the population partition.
func setParam(gen *simulator.ConfigGenerator, key string, value float64) {
	gen.GetPartition("population").Params.Map[key] = []float64{value}
}

// setVec overwrites a vector param on the population partition.
func setVec(gen *simulator.ConfigGenerator, key string, values []float64) {
	gen.GetPartition("population").Params.Map[key] = values
}

// deterministic switches the population partition to mean-field updates, so a
// small, signed policy effect is exactly reproducible without ensemble noise —
// the "drive the branch hard, low noise" tactic for near-deterministic claims.
func deterministic(gen *simulator.ConfigGenerator) {
	setParam(gen, "deterministic", 1.0)
}

// totalStock sums every sector×age bucket in a state row (the standing register).
func totalStock(row []float64) float64 {
	var sum float64
	for _, v := range row {
		sum += v
	}
	return sum
}

// meanBackHalfStock averages the total register stock over the back half of the
// run (a rough quasi-steady-state window once formation has filled the register).
func meanBackHalfStock(rows [][]float64) float64 {
	start := len(rows) / 2
	var sum float64
	var n int
	for i := start; i < len(rows); i++ {
		sum += totalStock(rows[i])
		n++
	}
	return sum / float64(n)
}

// sectorStock sums the 60 age buckets of one sector in a state row.
func sectorStock(row []float64, sec int) float64 {
	var s float64
	for a := 0; a < NumAges; a++ {
		s += row[sec*NumAges+a]
	}
	return s
}

// meanBackHalfSectorStock averages one sector's stock over the back half of the run.
func meanBackHalfSectorStock(rows [][]float64, sec int) float64 {
	start := len(rows) / 2
	var sum float64
	var n int
	for i := start; i < len(rows); i++ {
		sum += sectorStock(rows[i], sec)
		n++
	}
	return sum / float64(n)
}

// deterministicBackHalfStock runs one deterministic stub and returns the back-half
// mean register stock. Deterministic mode makes a single run sufficient.
func deterministicBackHalfStock(
	build stubBuilder,
	hazardScale float64,
	extra func(*simulator.ConfigGenerator),
) float64 {
	store := runStubOverride(build, hazardScale, DefaultNumSteps, 7001, func(g *simulator.ConfigGenerator) {
		deterministic(g)
		if extra != nil {
			extra(g)
		}
	})
	return meanBackHalfStock(store.GetValues("population"))
}

// cohortPerSector seeds the isolated survival cohort at age 0 for each sector.
const cohortPerSector = 5000.0

// cohortInit builds an initial state with cohortPerSector businesses in each
// sector's age-0 bucket and zeros elsewhere.
func cohortInit() []float64 {
	v := make([]float64, numSectors*NumAges)
	for sec := 0; sec < numSectors; sec++ {
		v[sec*NumAges+0] = cohortPerSector
	}
	return v
}

// cohortSurvival runs the model as an isolated five-year cohort (formation off,
// deterministic), seeding one cohort at age 0 and returning the fraction still
// active after 60 months. This is the model's signature decision metric.
func cohortSurvival(
	build stubBuilder,
	hazardScale float64,
	extra func(*simulator.ConfigGenerator),
) float64 {
	store := runStubOverride(build, hazardScale, 60, 7001, func(g *simulator.ConfigGenerator) {
		deterministic(g)
		setVec(g, "base_birth_rates", make([]float64, numSectors)) // formation off
		g.GetPartition("population").InitStateValues = cohortInit()
		if extra != nil {
			extra(g)
		}
	})
	rows := store.GetValues("population")
	return totalStock(rows[len(rows)-1]) / (cohortPerSector * float64(numSectors))
}

// ObservedBehaviour returns the model's named response claims, each with the
// deterministic mean-field numbers it produces. It is the single source of the
// card's "Observed behaviour" numbers AND the values the behaviour test asserts
// on, so the two can never disagree. Each claim's Monotone direction is what its
// binding test checks — covering both the actionable support levers a downstream
// decision-maker controls and the structural drivers the world sets.
//
// The claim IDs match the subtest names under TestBusinessSurvivalExpectedBehaviour.
func ObservedBehaviour() []cardgen.Claim {
	return observedBehaviour(BuildStub)
}

// observedBehaviour computes the claims against a given assembly of the model. The
// equivalence test runs it against the declarative build so that the data-only
// model answers the same claims, measured the same way.
func observedBehaviour(build stubBuilder) []cardgen.Claim {
	const (
		stock       = "deterministic back-half register stock"
		sectorStk   = "deterministic back-half sector stock"
		survival    = "five-year cohort survival fraction"
		hospitality = 1
		retail      = 4
		tech        = 5
	)

	// Shared deterministic baselines, reused across the claims that perturb one
	// input against them (avoids recomputing the same base each time).
	detBase := deterministicBackHalfStock(build, DefaultPolicyHazardScale, nil)
	baseRun := runStubOverride(build, DefaultPolicyHazardScale, DefaultNumSteps, 7001, deterministic)
	baseRows := baseRun.GetValues("population")
	baseTech := meanBackHalfSectorStock(baseRows, tech)
	baseHosp := meanBackHalfSectorStock(baseRows, hospitality)
	baseRetail := meanBackHalfSectorStock(baseRows, retail)
	calm := cohortSurvival(build, DefaultPolicyHazardScale, nil)

	// ----- Decision-path responses (actionable support levers) -----

	supported := cohortSurvival(build, 0.85, nil)
	adverse := cohortSurvival(build, 1.15, nil)

	boostedFormation := deterministicBackHalfStock(build, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
		setParam(g, "policy_birth_scale", 1.2)
	})

	infantHelped := cohortSurvival(build, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
		setParam(g, "policy_infant_hazard_scale", 0.3)
	})
	infantHurt := cohortSurvival(build, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
		setParam(g, "policy_infant_hazard_scale", 1.7)
	})

	targetedTech := meanBackHalfSectorStock(
		runStubOverride(build, DefaultPolicyHazardScale, DefaultNumSteps, 7001, func(g *simulator.ConfigGenerator) {
			deterministic(g)
			scale := make([]float64, numSectors)
			for i := range scale {
				scale[i] = 1.0
			}
			scale[tech] = 1.5
			setVec(g, "policy_sector_birth_scale", scale)
		}).GetValues("population"), tech)

	relievedHosp := meanBackHalfSectorStock(
		runStubOverride(build, DefaultPolicyHazardScale, DefaultNumSteps, 7001, func(g *simulator.ConfigGenerator) {
			deterministic(g)
			scale := make([]float64, numSectors)
			for i := range scale {
				scale[i] = 1.0
			}
			scale[hospitality] = 0.8
			setVec(g, "policy_sector_hazard_scale", scale)
		}).GetValues("population"), hospitality)

	// ----- Structural-driver responses (non-actionable; out-of-sample credibility) -----

	worseCurve := deterministicBackHalfStock(build, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
		scaled := make([]float64, len(DefaultSurvivalFracs))
		for i, v := range DefaultSurvivalFracs {
			scaled[i] = v * 0.9 // uniformly worse survival at every horizon
		}
		setVec(g, "survival_fracs", scaled)
	})

	lowRate := deterministicBackHalfStock(build, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
		setParam(g, "birth_elasticity_rate", -0.5)
		setVec(g, "covariate_bank_rates", []float64{0.5}) // = reference → neutral
	})
	highRate := deterministicBackHalfStock(build, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
		setParam(g, "birth_elasticity_rate", -0.5)
		setVec(g, "covariate_bank_rates", []float64{3.0}) // tighter → fewer births
	})

	lowClaimant := deterministicBackHalfStock(build, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
		setParam(g, "birth_elasticity_claimant", -0.4)
		setVec(g, "covariate_claimants", []float64{12000.0}) // = reference → neutral
	})
	highClaimant := deterministicBackHalfStock(build, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
		setParam(g, "birth_elasticity_claimant", -0.4)
		setVec(g, "covariate_claimants", []float64{24000.0}) // weaker economy
	})

	lowDeathRate := cohortSurvival(build, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
		setParam(g, "death_elasticity_rate", 0.5)
		setVec(g, "covariate_bank_rates", []float64{0.5}) // = reference → neutral
	})
	highDeathRate := cohortSurvival(build, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
		setParam(g, "death_elasticity_rate", 0.5)
		setVec(g, "covariate_bank_rates", []float64{3.0}) // tighter → higher hazard
	})

	distressed := cohortSurvival(build, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
		setVec(g, "distress_hazard_boost", []float64{0.3}) // +30% hazard
	})

	burdenedRetail := meanBackHalfSectorStock(
		runStubOverride(build, DefaultPolicyHazardScale, DefaultNumSteps, 7001, func(g *simulator.ConfigGenerator) {
			deterministic(g)
			scale := append([]float64(nil), DefaultSectorHazardScales...)
			scale[retail] = 1.5
			setVec(g, "sector_hazard_scales", scale)
		}).GetValues("population"), retail)

	return []cardgen.Claim{
		{
			ID:        "lower_death_hazard_scale_raises_five_year_cohort_survival",
			Statement: "A lower support-policy exit-hazard scale raises five-year cohort survival",
			Unit:      survival,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "adverse (scale=1.15)", Value: adverse},
				{Label: "supported (scale=0.85)", Value: supported},
			},
		},
		{
			ID:        "higher_formation_support_raises_register_stock",
			Statement: "Formation support (higher birth scale) raises register stock",
			Unit:      stock,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "base", Value: detBase},
				{Label: "policy_birth_scale=1.2", Value: boostedFormation},
			},
		},
		{
			ID:        "lower_infant_hazard_support_raises_cohort_survival",
			Statement: "First-year (infant) hazard relief raises cohort survival",
			Unit:      survival,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "infant scale=1.7", Value: infantHurt},
				{Label: "infant scale=0.3", Value: infantHelped},
			},
		},
		{
			ID:        "targeted_sector_formation_support_raises_that_sector_stock",
			Statement: "A sector-targeted formation subsidy raises that sector's stock (Technology)",
			Unit:      sectorStk,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "base", Value: baseTech},
				{Label: "Technology birth scale=1.5", Value: targetedTech},
			},
		},
		{
			ID:        "targeted_sector_hazard_relief_raises_that_sector_stock",
			Statement: "Sector-targeted hazard relief raises that sector's stock (Hospitality)",
			Unit:      sectorStk,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "base", Value: baseHosp},
				{Label: "Hospitality hazard scale=0.8", Value: relievedHosp},
			},
		},
		{
			ID:        "worse_baseline_survival_curve_lowers_stock",
			Statement: "A worse baseline ONS survival curve lowers register stock",
			Unit:      stock,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "base curve", Value: detBase},
				{Label: "survival ×0.9", Value: worseCurve},
			},
		},
		{
			ID:        "higher_bank_rate_suppresses_formation",
			Statement: "A higher Bank Rate (negative birth elasticity) suppresses formation",
			Unit:      stock,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "Bank Rate 0.5%", Value: lowRate},
				{Label: "Bank Rate 3.0%", Value: highRate},
			},
		},
		{
			ID:        "higher_claimant_count_suppresses_formation",
			Statement: "A higher claimant count (negative birth elasticity) suppresses formation",
			Unit:      stock,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "claimants 12k", Value: lowClaimant},
				{Label: "claimants 24k", Value: highClaimant},
			},
		},
		{
			ID:        "higher_bank_rate_raises_exit_hazard_and_lowers_survival",
			Statement: "A higher Bank Rate (positive death elasticity) raises exit hazards and lowers cohort survival",
			Unit:      survival,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "Bank Rate 0.5%", Value: lowDeathRate},
				{Label: "Bank Rate 3.0%", Value: highDeathRate},
			},
		},
		{
			ID:        "distress_signal_lowers_cohort_survival",
			Statement: "A positive distress-hazard boost lowers cohort survival",
			Unit:      survival,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "calm", Value: calm},
				{Label: "distress boost +0.3", Value: distressed},
			},
		},
		{
			ID:        "higher_sector_baseline_hazard_lowers_that_sector_stock",
			Statement: "A higher structural sector baseline hazard lowers that sector's stock (Retail)",
			Unit:      sectorStk,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "base", Value: baseRetail},
				{Label: "Retail hazard scale=1.5", Value: burdenedRetail},
			},
		},
	}
}
