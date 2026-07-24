// Package api is the configuration tier: it turns one YAML document into a running
// simulation. Everything the engine can do — the forward model, how its partitions are
// wired, how many seeded members to run, where observations come from, where results go,
// and the inference, aggregation or optimisation layered on top — is expressible here as
// data, so a whole run becomes a single artifact that can be versioned, diffed and
// executed by a prebuilt binary with no Go toolchain present.
//
// # How a component is named
//
// Every position in a config that holds a framework component is a data spec — a mapping
// selecting a registered name, e.g. iteration: {type: wiener_process} or
// timestep_function: {type: constant, stepsize: 1.0}, resolved at load time by this
// package's registries. There is no Go-expression spelling: the whole document is data, so
// LoadApiRunConfigFromYaml resolves it and RunWithParsedArgs runs it in-process with no Go
// toolchain. A component given as a scalar Go string is rejected at load.
//
// A partition's bespoke maths is data too: an expressions: entry (ExpressionConfig, inlining
// general.ExpressionIteration) states the per-step update as expressions. The registries are
// for the framework's own catalogue; the expressions DSL is for a model's arithmetic. Genuinely
// novel algorithmic iterations that are neither in the catalogue nor expressible in the DSL
// belong in a downstream repo that embeds the engine as a Go library (Settings +
// Implementations), not in a config.
//
// # The config surface
//
//	main:     partitions and the simulation block (output condition and function,
//	          termination condition, timestep function) — see RunConfig.
//	embedded: named sub-runs, each a whole RunConfig (EmbeddedRunConfig). A main-run
//	          partition whose name matches one is replaced by an embedded simulation
//	          iteration wired to it, which is how a simulation nests inside a partition.
//	run:      execution mode — batch or ensemble, with seeds and concurrency (RunModeConfig).
//	data:     a StateTimeStorage, produced either by a sub-simulation or by a pre-recorded
//	          source (DataSource: csv, json_log, postgres, plus registered ones).
//	macros:   each entry expands one pkg/analysis constructor into a set of partitions over
//	          that storage, or runs live with no data: block at all.
//
// # Where the registries live
//
//	registry.go          data-only iterations — a name maps to a constructed type, params
//	                     carry the rest.
//	registry_compose.go  composable iterations, whose interface-typed fields (kernel,
//	                     likelihood, jump distribution, prior, nested iteration, named
//	                     function) are themselves specs, resolved recursively. The
//	                     "expression" builder here makes the whole expressions DSL usable
//	                     as an inline iteration spec, so maths can appear anywhere an
//	                     iteration is expected — inside a macro's window, or an embedded run.
//	macros*.go           the analysis tier: one file per family (aggregation, inference,
//	                     smc, optimisation, stats, data). Macro inputs are typed spec
//	                     structs decoded straight from YAML.
//
// # Staying honest, and staying lean
//
// Two drift tests guard the iteration registry (registry_test.go and
// registry_coverage_test.go): every registered name must construct the type it claims, and a
// go/ast scan requires every Iterate-implementing type in the candidate packages to be either
// registered or listed in excludedIterations with a reason. A newly-added iteration therefore
// fails CI until it is classified, which is what stops the registry silently lagging the
// framework.
//
// Imports drive go.mod, so components with heavy dependencies are not named here directly.
// RegisterDataSource (and simulator.RegisterComponent for sinks) lets a package layered above
// this one contribute a source or output spelling without the engine depending on it; the
// Arrow, S3 and DuckDB spellings are registered this way by cmd/stochadex-full. An unknown
// key reports the spellings the running binary actually has.
//
// # Pre-flight
//
// CheckForDeadlock runs before any batch or ensemble simulation. Within-step wiring
// (params_from_upstream) that forms a dependency cycle would otherwise surface as an opaque
// runtime "all goroutines are asleep" with no indication of which partitions are at fault;
// the check names them and says how to break the cycle. It runs no simulation. See pkg/graph.
//
// # Scope
//
// Inference as forward simulation — a posterior stepped as a partition — is in scope, which
// is why posterior_estimation and the other inference macros live here. Inference against
// real data is the data: resource, which a downstream repo supplies. The decision layer stays
// in Go on purpose: an agents.Environment is arbitrary game rules, not representable as data.
package api
