package bizsurvival

import (
	"sync"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// runStubOverride runs the stub with an override hook applied to the generated
// config before the run, so a behaviour test can vary any partition's params
// (policy multipliers, elasticities, covariates, survival curve) without bloating
// BuildStub's signature — BuildStub still exposes only the one headline driver.
func runStubOverride(
	t *testing.T,
	hazardScale float64,
	numSteps int,
	seed uint64,
	override func(*simulator.ConfigGenerator),
) *simulator.StateTimeStorage {
	t.Helper()
	gen := BuildStub(hazardScale, numSteps, seed)
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
	t *testing.T,
	hazardScale float64,
	extra func(*simulator.ConfigGenerator),
) float64 {
	t.Helper()
	store := runStubOverride(t, hazardScale, DefaultNumSteps, 7001, func(g *simulator.ConfigGenerator) {
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
	t *testing.T,
	hazardScale float64,
	extra func(*simulator.ConfigGenerator),
) float64 {
	t.Helper()
	store := runStubOverride(t, hazardScale, 60, 7001, func(g *simulator.ConfigGenerator) {
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

// TestBusinessSurvivalExpectedBehaviour is the expected-behaviour suite: each
// subtest name states, in plain language, a response the model is claimed to
// produce, and the body checks it. Together they specify how the model behaves
// for a downstream policy decision-maker (actionable support levers) and why it
// should be trusted off-sample (structural demographic and macro drivers).
func TestBusinessSurvivalExpectedBehaviour(t *testing.T) {
	// ----- Decision-path responses (actionable support levers a downstream controls) -----

	// The headline support lever, on the model's signature metric: a package that
	// cuts the monthly exit hazard raises five-year cohort survival. A wrong sign
	// here would rank the support portfolios backwards.
	t.Run("lower_death_hazard_scale_raises_five_year_cohort_survival", func(t *testing.T) {
		supported := cohortSurvival(t, 0.85, nil)
		adverse := cohortSurvival(t, 1.15, nil)
		if !(supported > adverse) {
			t.Fatalf("expected a lower exit hazard to raise cohort survival: "+
				"supported(0.85)=%.4f adverse(1.15)=%.4f", supported, adverse)
		}
	})

	// Formation support (grants / first-year finance) lifts the birth rate, so more
	// businesses enter and the standing register grows.
	t.Run("higher_formation_support_raises_register_stock", func(t *testing.T) {
		base := deterministicBackHalfStock(t, DefaultPolicyHazardScale, nil)
		boosted := deterministicBackHalfStock(t, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
			setParam(g, "policy_birth_scale", 1.2)
		})
		if !(boosted > base) {
			t.Fatalf("expected formation support to raise register stock: "+
				"base=%.1f boosted(1.2)=%.1f", base, boosted)
		}
	})

	// First-year support (a lower infant hazard on the age 0→1 transition) helps
	// more of a cohort survive its riskiest month, raising cohort survival. The
	// effect is small — the first-month hazard is tiny — so it is checked
	// deterministically where the signed change is exact.
	t.Run("lower_infant_hazard_support_raises_cohort_survival", func(t *testing.T) {
		helped := cohortSurvival(t, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
			setParam(g, "policy_infant_hazard_scale", 0.3)
		})
		hurt := cohortSurvival(t, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
			setParam(g, "policy_infant_hazard_scale", 1.7)
		})
		if !(helped > hurt) {
			t.Fatalf("expected lower infant hazard to raise cohort survival: "+
				"helped(0.3)=%.5f hurt(1.7)=%.5f", helped, hurt)
		}
	})

	// (sector, action) → outcome: a sector-targeted formation subsidy raises *that*
	// sector's stock. Technology is index 5 in SectorNames.
	t.Run("targeted_sector_formation_support_raises_that_sector_stock", func(t *testing.T) {
		const tech = 5
		override := func(g *simulator.ConfigGenerator) {
			scale := make([]float64, numSectors)
			for i := range scale {
				scale[i] = 1.0
			}
			scale[tech] = 1.5
			setVec(g, "policy_sector_birth_scale", scale)
		}
		base := runStubOverride(t, DefaultPolicyHazardScale, DefaultNumSteps, 7001, deterministic)
		targeted := runStubOverride(t, DefaultPolicyHazardScale, DefaultNumSteps, 7001, func(g *simulator.ConfigGenerator) {
			deterministic(g)
			override(g)
		})
		baseTech := meanBackHalfSectorStock(base.GetValues("population"), tech)
		targetedTech := meanBackHalfSectorStock(targeted.GetValues("population"), tech)
		if !(targetedTech > baseTech) {
			t.Fatalf("expected targeted formation support to raise Technology stock: "+
				"base=%.1f targeted(1.5)=%.1f", baseTech, targetedTech)
		}
	})

	// (sector, action) → outcome, hazard side: sector-targeted hazard relief (e.g.
	// rates relief tilted to hospitality) raises that sector's stock. Hospitality
	// is index 1 in SectorNames.
	t.Run("targeted_sector_hazard_relief_raises_that_sector_stock", func(t *testing.T) {
		const hospitality = 1
		base := runStubOverride(t, DefaultPolicyHazardScale, DefaultNumSteps, 7001, deterministic)
		relieved := runStubOverride(t, DefaultPolicyHazardScale, DefaultNumSteps, 7001, func(g *simulator.ConfigGenerator) {
			deterministic(g)
			scale := make([]float64, numSectors)
			for i := range scale {
				scale[i] = 1.0
			}
			scale[hospitality] = 0.8
			setVec(g, "policy_sector_hazard_scale", scale)
		})
		baseHosp := meanBackHalfSectorStock(base.GetValues("population"), hospitality)
		relievedHosp := meanBackHalfSectorStock(relieved.GetValues("population"), hospitality)
		if !(relievedHosp > baseHosp) {
			t.Fatalf("expected targeted hazard relief to raise Hospitality stock: "+
				"base=%.1f relieved(0.8)=%.1f", baseHosp, relievedHosp)
		}
	})

	// ----- Structural-driver responses (non-actionable; out-of-sample credibility) -----

	// Demography: a worse baseline ONS survival curve (higher exit hazards the
	// world imposes, not a policy choice) leaves fewer businesses standing.
	t.Run("worse_baseline_survival_curve_lowers_stock", func(t *testing.T) {
		base := deterministicBackHalfStock(t, DefaultPolicyHazardScale, nil)
		worse := deterministicBackHalfStock(t, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
			scaled := make([]float64, len(DefaultSurvivalFracs))
			for i, v := range DefaultSurvivalFracs {
				scaled[i] = v * 0.9 // uniformly worse survival at every horizon
			}
			setVec(g, "survival_fracs", scaled)
		})
		if !(worse < base) {
			t.Fatalf("expected a worse survival curve to lower stock: "+
				"base=%.1f worse=%.1f", base, worse)
		}
	})

	// Macro birth channel: with a negative rate elasticity held fixed, a higher
	// Bank Rate suppresses formation, lowering the register. Only the covariate
	// moves between the two runs, so the sign is attributable to the rate channel.
	t.Run("higher_bank_rate_suppresses_formation", func(t *testing.T) {
		low := deterministicBackHalfStock(t, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
			setParam(g, "birth_elasticity_rate", -0.5)
			setVec(g, "covariate_bank_rates", []float64{0.5}) // = reference → neutral
		})
		high := deterministicBackHalfStock(t, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
			setParam(g, "birth_elasticity_rate", -0.5)
			setVec(g, "covariate_bank_rates", []float64{3.0}) // tighter → fewer births
		})
		if !(high < low) {
			t.Fatalf("expected a higher bank rate to suppress formation: "+
				"low(0.5%%)=%.1f high(3.0%%)=%.1f", low, high)
		}
	})

	// Macro birth channel, labour side: with a negative claimant elasticity held
	// fixed, a higher claimant count (weaker local economy) suppresses formation.
	t.Run("higher_claimant_count_suppresses_formation", func(t *testing.T) {
		low := deterministicBackHalfStock(t, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
			setParam(g, "birth_elasticity_claimant", -0.4)
			setVec(g, "covariate_claimants", []float64{12000.0}) // = reference → neutral
		})
		high := deterministicBackHalfStock(t, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
			setParam(g, "birth_elasticity_claimant", -0.4)
			setVec(g, "covariate_claimants", []float64{24000.0}) // weaker economy
		})
		if !(high < low) {
			t.Fatalf("expected a higher claimant count to suppress formation: "+
				"low(12k)=%.1f high(24k)=%.1f", low, high)
		}
	})

	// Macro death channel: with a positive rate–hazard elasticity held fixed, a
	// higher Bank Rate raises exit hazards, lowering cohort survival. This is the
	// recessionary-stress channel, checked on the survival metric.
	t.Run("higher_bank_rate_raises_exit_hazard_and_lowers_survival", func(t *testing.T) {
		low := cohortSurvival(t, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
			setParam(g, "death_elasticity_rate", 0.5)
			setVec(g, "covariate_bank_rates", []float64{0.5}) // = reference → neutral
		})
		high := cohortSurvival(t, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
			setParam(g, "death_elasticity_rate", 0.5)
			setVec(g, "covariate_bank_rates", []float64{3.0}) // tighter → higher hazard
		})
		if !(high < low) {
			t.Fatalf("expected a higher bank rate to lower cohort survival: "+
				"low(0.5%%)=%.4f high(3.0%%)=%.4f", low, high)
		}
	})

	// Distress leading-indicator channel: a positive distress-hazard boost lifts
	// the effective exit hazard, lowering cohort survival — the sharper early-warning
	// channel the downstream can drive from filing/claimant signals.
	t.Run("distress_signal_lowers_cohort_survival", func(t *testing.T) {
		calm := cohortSurvival(t, DefaultPolicyHazardScale, nil)
		distressed := cohortSurvival(t, DefaultPolicyHazardScale, func(g *simulator.ConfigGenerator) {
			setVec(g, "distress_hazard_boost", []float64{0.3}) // +30% hazard
		})
		if !(distressed < calm) {
			t.Fatalf("expected a distress boost to lower cohort survival: "+
				"calm=%.4f distressed(+0.3)=%.4f", calm, distressed)
		}
	})

	// Sector heterogeneity: a sector the world burdens with a higher baseline
	// hazard (structural, not a policy choice) holds less stock than its peers.
	// Retail is index 4 in SectorNames.
	t.Run("higher_sector_baseline_hazard_lowers_that_sector_stock", func(t *testing.T) {
		const retail = 4
		base := runStubOverride(t, DefaultPolicyHazardScale, DefaultNumSteps, 7001, deterministic)
		burdened := runStubOverride(t, DefaultPolicyHazardScale, DefaultNumSteps, 7001, func(g *simulator.ConfigGenerator) {
			deterministic(g)
			scale := append([]float64(nil), DefaultSectorHazardScales...)
			scale[retail] = 1.5
			setVec(g, "sector_hazard_scales", scale)
		})
		baseRetail := meanBackHalfSectorStock(base.GetValues("population"), retail)
		burdenedRetail := meanBackHalfSectorStock(burdened.GetValues("population"), retail)
		if !(burdenedRetail < baseRetail) {
			t.Fatalf("expected a higher baseline hazard to lower Retail stock: "+
				"base=%.1f burdened(1.5)=%.1f", baseRetail, burdenedRetail)
		}
	})
}
