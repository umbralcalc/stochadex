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
// # The declarative twin, and the gap that used to be here
//
// This entry was the catalogue's one worked example of a category 2 capability
// gap: the model that could not be stated as data. That gap is closed, and
// [declarative.yaml] is the twin.
//
// What was missing was structure, not vocabulary — general.ExpressionIteration
// was elementwise over a partition's current row, and a cohort flow is precisely
// the update that breaks that alignment. Element i of the next register reads
// element i-1 of the current one (a business ages one month), the top bucket is
// absorbing, and the state is a flattened sector × age grid whose per-sector
// blocks the evaluator had no way to address. The diagnosis held: no set of
// functions rescued it, and what closed it was an operator that enlarges the
// DSL. That operator is each(n, i, expr), which builds a width-n value whose
// element i is expr with the lane index i bound. It answers all three parts at
// once. The lane index carries both grid coordinates, so sec = floor(k/60) and
// age = k%60 reproduce offset(sec, age) exactly and the per-sector blocks become
// addressable. A lane may read element k-1, which is the index shift itself. And
// everything inside a lane is a scalar, so where is lazy there: the age 0 lane
// never evaluates register[-1], and a guarded draw is genuinely skipped rather
// than merely discarded. concat and slice supply the shifted denominator that
// turns the ONS survival curve into monthly hazards, which is the other place the
// old evaluator had to reach past the current element.
//
// # The sampler note, which is still a category 1 finding
//
// This iteration hand-rolls poissonSample and binomialSample beside the samplers
// pkg/rng now ships. That remains standardisation debt in the model, to be paid
// on its own merits and never as a side effect of making an equivalence test
// agree.
//
// It is worth recording precisely what it did NOT cost, because the natural
// reading is wrong and the twin's oracle turns on it. In
// antimicrobial-resistance a hand-rolled sampler is what forces a weaker,
// distributional oracle: its Knuth loop is a genuinely different algorithm from
// the engine's, consuming a different number of draws, so the streams cannot be
// aligned. Not so here. These two methods are thin wrappers that set Lambda /
// N / P on a distuv distribution and call its Rand, and pkg/rng is a deliberate
// bit-identical reimplementation of distuv's own algorithms. Same generator,
// same algorithm, same draw order — so the twin is checked EXACTLY, on the
// stochastic path as well as the mean-field one, and the debt is cosmetic rather
// than semantic. See expression_equivalence_test.go.
//
// One real difference between the two is recorded rather than patched: the Go
// caches its monthly hazard table, and its deterministic flag, in Configure,
// where the twin derives both per step from the same params. Every assembly sets
// those params before Configure, so the two agree everywhere either is used, but
// a mid-run change to survival_fracs would be ignored by the Go and honoured by
// the twin. That is a property of the implementation and not of the demography.
package bizsurvival
