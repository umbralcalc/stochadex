package bizsurvival

import "github.com/umbralcalc/stochadex/pkg/simulator"

// Default generative parameters for the flagship scenario: a single English
// local authority's business register, stratified by six coarse sectors and age
// in months, evolving under monthly formation, ONS-style exit hazards, neutral
// macroeconomics, and a support-policy hazard multiplier. These are illustrative
// values chosen so the generative core runs with zero external inputs — NOT
// calibrated posteriors. In the downstream repo the survival curve is taken from
// ONS business demography, per-LA formation from the Companies House live
// register, and the birth/hazard elasticities from a NOMIS / Bank of England
// panel (FD regression + SMC); see card.md.
//
// The economic elasticities default to zero (macro-neutral) and the covariates
// are held constant, so the baseline run isolates demography and policy. The one
// scientifically-interesting driver, hazardScale (the support-policy multiplier
// on the monthly exit hazard), is what the CI test sweeps: a support package that
// cuts the exit hazard (hazardScale < 1) raises the standing register stock, and
// raising the hazard (hazardScale > 1) shrinks it.
const (
	// DefaultPolicyHazardScale is the baseline (no-intervention) value of the
	// swept driver: a multiplier of 1 leaves the ONS-derived hazard unchanged.
	DefaultPolicyHazardScale = 1.0

	// Economic reference levels and (neutral) elasticities. The stub holds the
	// covariates constant at their reference so the baseline is macro-neutral;
	// behaviour tests activate these channels by overriding the elasticities.
	DefaultBankRate                = 0.5
	DefaultClaimantCount           = 12000.0
	DefaultRateRef                 = 0.5
	DefaultClaimantRef             = 12000.0
	DefaultBirthElasticityRate     = 0.0
	DefaultBirthElasticityClaimant = 0.0
	DefaultDeathElasticityRate     = 0.0

	// NumAges is the number of monthly age buckets per sector (0..59, top bucket
	// aggregates all ages ≥59 months).
	NumAges = 60

	// DefaultNumSteps is the flagship horizon: 120 monthly steps (ten years),
	// matching the downstream evaluator's default run length.
	DefaultNumSteps = 120

	// populationPartition is the (single) partition index within the stub.
	populationPartition = 0
)

// SectorNames are the six coarse sectors (fixed index order), matching the
// downstream lifecycle/explore SIC→sector mapping.
var SectorNames = []string{
	"Construction",
	"Hospitality",
	"Other",
	"Professional",
	"Retail",
	"Technology",
}

// DefaultSurvivalFracs are ONS-style cumulative business survival fractions at
// years 1–5 (UK 2019 birth cohort ≈ 94.6% → 38.4% five-year survival). These set
// the baseline monthly exit hazard before any policy or macro multiplier.
var DefaultSurvivalFracs = []float64{0.946, 0.747, 0.559, 0.45, 0.384}

// DefaultSectorHazardScales are per-sector multipliers on the baseline hazard
// (all 1 = every sector shares the same ONS curve until a driver differentiates
// them). Length must match SectorNames.
var DefaultSectorHazardScales = []float64{1.0, 1.0, 1.0, 1.0, 1.0, 1.0}

// DefaultBaseBirthRates are illustrative per-sector monthly Poisson formation
// rates. Length must match SectorNames.
var DefaultBaseBirthRates = []float64{5.0, 4.0, 6.0, 4.0, 5.0, 3.0}

// numSectors is the fixed sector count derived from SectorNames.
var numSectors = len(SectorNames)

// BuildStub constructs the data-free generative core of the business-survival
// model: a single monthly, sector-by-age Leslie process for one local
// authority's business register. New businesses arrive per sector (Poisson
// formation), age one month per step, and exit at ONS-derived monthly hazards;
// a support-policy multiplier and (by default neutral) macroeconomic covariates
// scale formation and hazards.
//
// This is the generative core only — no data ingestion, no panel/SMC inference,
// no scenario or portfolio decision layer. The one scientifically-interesting
// driver, hazardScale, enters as the support-policy hazard multiplier
// (policy_death_hazard_scale): < 1 models support that lowers business exits,
// > 1 an adverse shock. It is exactly the parameter the CI test sweeps to check
// the model's headline claim: cutting the exit hazard raises the standing
// register stock.
//
// The register starts empty and fills from formation, so over the run it
// approaches a quasi-steady stock set by formation × mean business lifetime —
// which is what the hazard multiplier moves. seed varies the Poisson/binomial
// draws for ensemble averaging.
//
// Partitions (declaration order):
//
//	population  monthly sector×age Leslie register (nSectors×60 state components)
func BuildStub(hazardScale float64, numSteps int, seed uint64) *simulator.ConfigGenerator {
	width := numSectors * NumAges

	population := &simulator.PartitionConfig{
		Name:      "population",
		Iteration: &SingleLAPopulationIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"survival_fracs":            append([]float64(nil), DefaultSurvivalFracs...),
			"sector_hazard_scales":      append([]float64(nil), DefaultSectorHazardScales...),
			"base_birth_rates":          append([]float64(nil), DefaultBaseBirthRates...),
			"covariate_bank_rates":      {DefaultBankRate},
			"covariate_claimants":       {DefaultClaimantCount},
			"rate_ref":                  {DefaultRateRef},
			"claimant_ref":              {DefaultClaimantRef},
			"birth_elasticity_rate":     {DefaultBirthElasticityRate},
			"birth_elasticity_claimant": {DefaultBirthElasticityClaimant},
			"death_elasticity_rate":     {DefaultDeathElasticityRate},
			// The one swept driver: support-policy hazard multiplier.
			"policy_death_hazard_scale": {hazardScale},
		}),
		InitStateValues:   make([]float64, width),
		StateHistoryDepth: 1,
		Seed:              seed,
	}

	gen := simulator.NewConfigGenerator()
	gen.SetPartition(population)
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition: &simulator.EveryStepOutputCondition{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: numSteps,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:    0.0,
	})
	return gen
}
