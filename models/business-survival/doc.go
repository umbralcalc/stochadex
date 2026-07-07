// Package bizsurvival is the catalogue stub for local-authority business
// demography: a monthly, sector-by-age Leslie model of the business register
// under formation, exit hazards, macroeconomic covariates and support-policy
// multipliers.
//
// The generative core is a single bespoke iteration,
// SingleLAPopulationIteration ([single_la_population.go]), with its survival →
// hazard helper ([hazard.go]). Both are lifted verbatim from the downstream
// business-survival repo. They live here — beside the stub rather than in engine
// core — because the catalogue is the staging ground for the "should this be
// promoted into core?" question: a generic aged-cohort / Leslie primitive
// recurring across other models would be the signal to promote, but that waits
// for the recurrence. The downstream repo's data ingestion (ONS / Companies
// House / NOMIS / Bank of England), panel calibration, and SMC inference
// helpers are inference / ingestion concerns and were left downstream.
package bizsurvival
