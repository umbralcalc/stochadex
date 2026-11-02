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
// LoadConfig resolves it and RunWith, RunToStorage or the CLI (Execute) runs it in-process
// with no Go toolchain. A component given as a scalar Go string is rejected at load.
// BuildCatalogue lists every name, with what each takes and reads.
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
// A config describes one simulation, inputs → simulation → outputs, and how many times to
// run it (docs/reference.md is the written semantics).
//
//	inputs:   named, read-only data the run reads when it starts: a source (csv, json_log,
//	          postgres, inline, plus registered ones), a pre-pass simulation, a nested run,
//	          a file, or a live stream (InputConfig). data: is deprecated shorthand for one.
//	main:     partitions, expressions and the simulation block (termination condition,
//	          timestep function, initial time, execution strategy) — see RunConfig.
//	embedded: named sub-runs, each a whole RunConfig (EmbeddedRunConfig). A main-run
//	          partition whose name matches one is replaced by an embedded simulation
//	          iteration wired to it, which is how a simulation nests inside a partition.
//	macros:   shortcuts: a run expands each entry into core-language partitions in the
//	          same simulation (Expand, expand*.go), which stochadex expand prints.
//	outputs:  views of the run, each a condition and a destination (OutputViewConfig).
//	run:      how many times the simulation runs, and how fast — batch, ensemble (seeds,
//	          concurrency) or serve (websocket, pace_ms; see serve.go) (RunModeConfig).
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
//	macros*.go           the macros: tier: decodes typed spec structs straight from YAML,
//	                     one file per family (aggregation, inference, smc, optimisation,
//	                     stats, data, mcts, planning).
//	expand*.go           expands each macro into core-language config, which runs as one
//	                     ordinary run (Expand); the run's partitions mirror the matching
//	                     pkg/macros constructors.
//	registry_environment.go
//	                     the environments the mcts_* iterations and the mcts_self_play
//	                     macro search.
//	                     Unlike the registries above this one is empty by default and filled
//	                     by downstream modules calling RegisterEnvironment: decision rules
//	                     are not part of the framework catalogue and have no data spelling,
//	                     so the engine passes an env: spec through to its registered builder
//	                     without interpreting it. The tictactoe fixture is the exception the
//	                     engine ships, so the path stays covered end-to-end in CI.
//
// # Staying honest, and staying lean
//
// Two drift tests guard the iteration registry (registry_test.go and
// registry_coverage_test.go): every registered name must construct the type it claims, and a
// go/ast scan requires every Iterate-implementing type in the candidate packages to be either
// registered or listed in excludedIterations with a reason. A newly-added iteration therefore
// fails CI until it is classified, which is what stops the registry silently lagging the
// framework. What stochadex catalogue says of each registered type (its description,
// spec fields and params) is read from source into catalogue_fields.json, which
// TestCatalogueFieldsUpToDate keeps current and TestCatalogueFieldsCoverUsage checks
// against every config in use.
//
// Imports drive go.mod, so components with heavy dependencies are not named here directly.
// RegisterDataSource (and simulator.RegisterComponent for sinks) lets a package layered above
// this one contribute a source or output spelling without the engine depending on it; the
// Arrow, S3 and DuckDB spellings are registered this way by cmd/stochadex. An unknown
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
// real data reads it as an input, which a downstream repo supplies. The decision layer stays
// in Go on purpose: an agents.Environment is arbitrary game rules, not representable as data.
package api
