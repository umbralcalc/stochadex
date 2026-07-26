// Package macros builds multi-partition simulation topologies from a single
// declarative spec. Each entry point takes an Applied* struct describing what
// you want — a rolling likelihood comparison, a posterior estimation loop, a
// grouped aggregation — plus a *simulator.StateTimeStorage holding the data it
// reads, and returns the *simulator.PartitionConfig values that realise it.
//
// This is the tier the YAML `macros:` key expands into. Every constructor here
// is reachable from a config through pkg/api, which decodes the spec structs
// from YAML and calls into this package. NewMCTSSelfPlayPartitions is reachable
// too, but by a different route: its agents.Environment is arbitrary Go
// decision rules with no data spelling, so a config names an environment that a
// downstream module registered with api.RegisterEnvironment, and only the
// env-independent half of the spec (MCTSSearchSettings) comes from the config.
// The package is named for the shape of what it produces — spec in, partitions
// out — not for the YAML key alone.
//
// # Layering
//
// macros sits above pkg/analysis and depends on it one way only. pkg/analysis
// owns the data vocabulary: DataRef and IndexRange for addressing series
// inside storage, the NewStateTimeStorageFrom* loaders, GroupedStateTimeStorage,
// and the plotting and dataframe renderers. This package consumes that
// vocabulary and never the reverse, so the stack is
//
//	pkg/api → pkg/macros → pkg/analysis → pkg/simulator
//
// # What each file builds
//
//   - aggregation.go   — grouped and rolling vector mean/variance/covariance
//     partitions over an analysis.GroupedStateTimeStorage.
//   - likelihood.go    — the windowed-simulation substrate (WindowedPartitions,
//     ParameterisedModel) plus rolling likelihood comparison and mean-function
//     fitting. inference.go and optimisation.go build on its types.
//   - inference.go     — posterior mean/covariance/sampler partitions driving a
//     windowed likelihood, i.e. inference run as forward simulation.
//   - optimisation.go  — evolution-strategy optimisation over a windowed reward.
//   - smc.go           — sequential Monte Carlo inference, including the live
//     RunSMCInference driver. smc_particles.go holds the evaluator that runs one
//     model per particle over simulator.ReentrantSimulation, which is why the
//     model is stated once rather than replicated per particle.
//   - regression.go    — ScalarRegressionStatsIteration, the one true Iteration
//     in this package, streaming OLS sufficient statistics alongside a run.
//   - mcts.go          — MCTS self-play topology over a pkg/agents Environment.
//
// # Invariants
//
// Constructors panic rather than return errors: they run at config-assembly
// time, before any simulation starts, so a bad spec is a programming error and
// failing loudly beats a silently misshapen topology. Window depths are checked
// against the history depths the storage will actually carry —
// ValidateWindowDataHistoryDepth is the public form of that check, and callers
// wiring windows through analysis.AddPartitionsToStateTimeStorage should call it
// with the same map to fail fast instead of underflowing at step time.
package macros
