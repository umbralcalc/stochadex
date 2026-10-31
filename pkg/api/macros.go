package api

import (
	"fmt"
	"slices"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// The macros: tier. A macro is a config -> config shortcut: unlike an
// iteration, which adds one partition, a macro expands to a *set* of
// partitions in the core config language (see expand.go). A config with
// macros runs as its expansion, one ordinary run: its inputs replayed, any
// main: partitions, then each macro's partitions, which read the inputs, main:
// and earlier macros live. The whole thing is data, so it runs in-process with
// no Go toolchain.
//
// Each macro is a typed, YAML-decodable spec implementing macroSpec. Decoding
// straight into the typed spec (rather than an untyped map) is load-bearing: a
// bare YAML scalar like a partition named `y` is coerced to the boolean true when
// decoded into interface{}, but preserved as the string "y" when decoded into a
// string field.

// macroSpec is a typed macro configuration that expands into core-language
// config (see expand.go).
type macroSpec interface {
	expand(ctx *expansionContext) (*expansion, error)
}

// clockedMacroSpec is implemented by the macros that run on a clock of their
// own, steps of a timestep, rather than on the inputs': each is a live loop
// (an optimisation, an SMC run, a search) that a run steps as a simulation.
type clockedMacroSpec interface {
	macroSpec
	clock() *expandedClock
}

func (s *evolutionStrategySpec) clock() *expandedClock { return ownClock(s.Steps, s.Timestep) }
func (s *mctsSelfPlaySpec) clock() *expandedClock      { return ownClock(s.Steps, s.Timestep) }
func (s *mctsPlanningSpec) clock() *expandedClock      { return ownClock(s.Steps, s.Timestep) }

// An SMC run's clock is its rounds.
func (s *smcInferenceSpec) clock() *expandedClock { return ownClock(s.NumRounds, s.Timestep) }

// macroSpecFactories maps a macro type to a factory for its empty typed spec.
// Aggregation and inference/stats macros are against-storage analyses;
// evolution-strategy optimisation, SMC inference and MCTS self-play are live
// runs (see clockedMacroSpec). mcts_self_play is the odd one out in how its central
// component resolves: its environment is arbitrary decision rules with no data
// spelling, so it comes from the environment registry rather than a data
// registry — see registry_environment.go.
var macroSpecFactories = map[string]func() macroSpec{
	"vector_mean":                     func() macroSpec { return &vectorMeanSpec{} },
	"vector_variance":                 func() macroSpec { return &vectorVarianceSpec{} },
	"vector_covariance":               func() macroSpec { return &vectorCovarianceSpec{} },
	"grouped_aggregation":             func() macroSpec { return &groupedAggregationSpec{} },
	"scalar_regression_stats":         func() macroSpec { return &scalarRegressionStatsSpec{} },
	"likelihood_comparison":           func() macroSpec { return &likelihoodComparisonSpec{} },
	"posterior_estimation":            func() macroSpec { return &posteriorEstimationSpec{} },
	"likelihood_mean_function_fit":    func() macroSpec { return &likelihoodMeanFunctionFitSpec{} },
	"evolution_strategy_optimisation": func() macroSpec { return &evolutionStrategySpec{} },
	"smc_inference":                   func() macroSpec { return &smcInferenceSpec{} },
	"mcts_self_play":                  func() macroSpec { return &mctsSelfPlaySpec{} },
	"mcts_planning":                   func() macroSpec { return &mctsPlanningSpec{} },
}

// DataConfig is the data: tier: it produces the StateTimeStorage that macros
// analyse, either by running a sub-simulation (Partitions run for Steps) or by
// loading a file (Source). Sub-simulation partitions carry data-spec iterations
// (or expressions), like any other run.
type DataConfig struct {
	// Source loads storage from a file. When set, the sub-simulation fields below
	// are ignored.
	Source      *DataSource                 `yaml:"source,omitempty"`
	Partitions  []simulator.PartitionConfig `yaml:"partitions,omitempty"`
	Expressions []ExpressionConfig          `yaml:"expressions,omitempty"`
	Steps       int                         `yaml:"steps,omitempty"`
	Timestep    float64                     `yaml:"timestep,omitempty"`
	InitTime    float64                     `yaml:"init_time,omitempty"`
}

// macroTypeField is embedded (inline) in every macro spec so decoding a spec
// consumes the `type` discriminator rather than leaving it as an unknown key —
// which the strict dead-key check would otherwise flag.
type macroTypeField struct {
	Type string `yaml:"type"`
}

// MacroConfig is one entry in the macros: tier. It decodes its `type` and then
// the whole entry into that type's typed spec, so field values keep their YAML
// types (see the package note on the `y`->true coercion).
type MacroConfig struct {
	Type string
	Spec macroSpec
}

// UnmarshalYAML reads the macro type, then decodes the entry into the matching
// typed spec. The type is read via a map (which accepts any keys) so this does
// not trip the strict dead-key check; the typed spec decode that follows is what
// validates the remaining keys.
func (m *MacroConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var header map[string]interface{}
	if err := unmarshal(&header); err != nil {
		return err
	}
	typeName, _ := header["type"].(string)
	factory, ok := macroSpecFactories[typeName]
	if !ok {
		return fmt.Errorf("api: unknown macro type %q", typeName)
	}
	m.Type = typeName
	spec := factory()
	if err := unmarshal(spec); err != nil {
		return fmt.Errorf("macro %q: %w", typeName, err)
	}
	m.Spec = spec
	return nil
}

// buildStorage produces the data: tier's storage: from a file source when one is
// configured, otherwise by running the sub-simulation to completion.
func (d *DataConfig) buildStorage() (*simulator.StateTimeStorage, error) {
	if d.Source != nil {
		storage, err := d.Source.load()
		return storage, inputError(err)
	}
	run := RunConfig{Partitions: d.Partitions, Expressions: d.Expressions}
	if err := resolveIterations(run.Partitions); err != nil {
		return nil, configError(err)
	}
	if err := checkExpressionNames(run, nil, nil, "sub-simulation", nil); err != nil {
		return nil, configError(err)
	}
	generator := run.GetConfigGenerator()
	// Pre-flight the data: sub-simulation's within-step wiring, so a cyclic data
	// block fails with a located error rather than an opaque runtime deadlock.
	if err := CheckForDeadlock(generator); err != nil {
		return nil, configError(err)
	}
	partitions := make([]*simulator.PartitionConfig, 0, len(d.Partitions))
	for _, name := range generator.PartitionNames() {
		partitions = append(partitions, generator.GetPartition(name))
	}
	if d.Timestep == 0 {
		d.Timestep = 1.0
	}
	storage := analysis.NewStateTimeStorageFromPartitions(
		partitions,
		&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: d.Steps},
		&simulator.ConstantTimestepFunction{Stepsize: d.Timestep},
		d.InitTime,
	)
	return storage, nil
}

// resolveIterations resolves any data-spec iterations on the given partitions in
// place (used by the data: tier, which has no simulation block to resolve).
func resolveIterations(partitions []simulator.PartitionConfig) error {
	for index := range partitions {
		if !partitions[index].IterationSpec.IsData() {
			continue
		}
		iteration, err := ResolveIteration(partitions[index].IterationSpec)
		if err != nil {
			return fmt.Errorf("data partition %q: %w", partitions[index].Name, err)
		}
		partitions[index].Iteration = iteration
	}
	return nil
}

// simulationStringsZero reports whether no simulation: field was set.
func simulationStringsZero(s *simulator.SimulationConfigStrings) bool {
	return s.OutputCondition.IsZero() && s.OutputFunction.IsZero() &&
		s.TerminationCondition.IsZero() && s.TimestepFunction.IsZero() &&
		s.ExecutionStrategy.IsZero() && s.InitTimeValue == 0
}

// expanding runs one macro's expansion, classifying any failure — returned or
// panicked — as ErrConfig: expansion only turns the config into partitions, so
// a failure there (a missing partition reference, a bad window) is the config's,
// not the simulation's. Failures while the expanded partitions run are not
// covered and stay ErrRuntime.
func expanding(macroType string, expand func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = configError(fmt.Errorf("macro %q: %v", macroType, recovered))
		}
	}()
	if err := expand(); err != nil {
		return configError(fmt.Errorf("macro %q: %w", macroType, err))
	}
	return nil
}

// isolateWiring gives a partition its own copies of its params and wiring maps.
// A partition built from a config shares those maps with it, and a run writes
// into them — resolved params_as_partitions indices, params_from_upstream values
// — so without the copies running a macro rewrites the caller's config.
func isolateWiring(partition *simulator.PartitionConfig) {
	partition.Params = simulator.NewParams(copyParamsMap(partition.Params.Map))
	partition.ParamsFromUpstream = copyUpstreams(partition.ParamsFromUpstream)
	partition.ParamsAsPartitions = copyAsPartitions(partition.ParamsAsPartitions)
}

// copyParamsMap copies a params map and its value slices.
func copyParamsMap(params map[string][]float64) map[string][]float64 {
	copied := make(map[string][]float64, len(params))
	for key, values := range params {
		copied[key] = slices.Clone(values)
	}
	return copied
}

// copyUpstreams copies a params_from_upstream map.
func copyUpstreams(
	upstreams map[string]simulator.NamedUpstreamConfig,
) map[string]simulator.NamedUpstreamConfig {
	copied := make(map[string]simulator.NamedUpstreamConfig, len(upstreams))
	for key, upstream := range upstreams {
		copied[key] = upstream
	}
	return copied
}

// copyAsPartitions copies a params_as_partitions map and its name slices.
func copyAsPartitions(asPartitions map[string][]string) map[string][]string {
	copied := make(map[string][]string, len(asPartitions))
	for key, names := range asPartitions {
		copied[key] = slices.Clone(names)
	}
	return copied
}
