// Package bizsurvival is the catalogue stub for local-authority business
// demography: a monthly, sector-by-age Leslie model of the business register
// under formation, exit hazards, macroeconomic covariates and support-policy
// multipliers.
//
// The generative core is a single bespoke iteration,
// SingleLAPopulationIteration ([single_la_population.go]), with its survival →
// hazard helper ([hazard.go]). Both are lifted verbatim from the downstream
// business-survival repo. The downstream repo's data ingestion (ONS / Companies
// House / NOMIS / Bank of England), panel calibration, and SMC inference
// helpers are inference / ingestion concerns and were left downstream.
//
// # No declarative twin: a category 2 capability gap
//
// Every other catalogue entry ships a declarative.yaml stating its model as data
// (see models/CONVENTIONS.md §5). This one cannot, and the absence is the
// finding: it is the catalogue's worked example of a missing structure rather
// than a missing vocabulary, and one model is enough to prove it.
//
// general.ExpressionIteration is elementwise over a partition's current row:
// element i of an output is computed from element i of its inputs. A cohort flow
// is precisely the update that breaks that. Element i of the next register reads
// element i-1 of the current one (a business ages one month), the top bucket is
// absorbing (it takes both the inflow from below and its own survivors), and the
// state is a flattened sector × age grid whose per-sector blocks the evaluator
// has no way to address. No set of functions rescues this, because the alignment
// is built into what the evaluator is. Closing it means either an operator that
// enlarges the DSL (a shift, a slice, a reshape) or a core aged-cohort Iteration
// — a real design decision, and deliberately not taken on one instance.
//
// A second, smaller signal sits alongside it, and is a category 1 finding rather
// than a gap in core: this iteration hand-rolls poissonSample and binomialSample
// beside the samplers pkg/rng now ships. That is standardisation debt in the
// model, to be paid on its own merits and never as a side effect of making an
// equivalence test agree.
package bizsurvival
