// Package macros builds multi-partition simulation topologies from a single
// declarative spec. Each entry point takes an Applied* struct describing what
// you want — a rolling likelihood comparison, a posterior estimation loop, a
// grouped aggregation — plus a *simulator.StateTimeStorage holding the data it
// reads, and returns the *simulator.PartitionConfig values that realise it.
//
// It is the Go form of what a config's macros: key states as data. In a config,
// a macro is a config → config shortcut over the core language: pkg/api expands
// each entry into ordinary partitions in the same run (api.Expand, which
// stochadex expand prints), the partitions these constructors build, with no
// storage to run against. From Go, a constructor here builds the same topology
// against a StateTimeStorage, to run with analysis.AddPartitionsToStateTimeStorage.
// pkg/api's golden digests pin what each expansion runs, and its tests build the
// posterior and mean-function-fit topologies here directly and check a config
// gives the same.
//
// Self-play is the one macro whose central component has no data form: its
// agents.Environment is arbitrary Go decision rules, so a config names an
// environment a downstream module registered with api.RegisterEnvironment, and
// only the environment-independent half of the spec (MCTSSearchSettings) comes
// from the config; NewMCTSSelfPlayLayout gives both routes the same wiring.
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
// with the same map to fail fast instead of underflowing at step time. The
// history depth of a window data partition must *equal* the window depth: a
// larger one cannot underflow, but it is the worse failure — the window replays
// the buffer's zero-filled tail and the likelihood comes out constant and
// uninformative — so it panics too.
//
// Every row of a macro's data counts exactly once, its initial row included.
// That row is the data partition's initial state, which a run never steps to,
// so a partition that folds in only each step's latest value starts from it
// (simulator.PartitionConfig.InitStateFromUpstream, or an initial state made
// from it as ScalarRegressionInitState makes one), and one reading history
// reads only the rows the run has reached. pkg/api's TestMacrosCountEveryRowOnce
// holds every registered macro to this, or excludes it with a reason.
package macros
