package api

import (
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// The macro tier. A macro is a named partition-set-producing function over a
// config namespace (the constructors in pkg/analysis). Unlike an iteration, which
// adds one partition, a macro expands to a *set* of partitions. Analysis macros
// run against pre-recorded storage, so a config that uses them is a three-tier
// program:
//
//	data:    a sub-simulation, run once to produce a *StateTimeStorage
//	macros:  each expands (applied-config, storage) -> partitions, appended and
//	         run against that storage
//
// This is a distinct run context from the live coordinator: build storage, expand
// macros, run the expanded partitions against the storage, emit the result. The
// whole thing is data, so it runs in-process with no Go toolchain.
//
// Each macro is a typed, YAML-decodable spec implementing macroSpec. Decoding
// straight into the typed spec (rather than an untyped map) is load-bearing: a
// bare YAML scalar like a partition named `y` is coerced to the boolean true when
// decoded into interface{}, but preserved as the string "y" when decoded into a
// string field.

// macroSpec is a typed macro configuration that resolves, against storage, into
// the partitions the macro contributes and the window sizes those partitions need
// when replayed against storage.
type macroSpec interface {
	resolve(storage *simulator.StateTimeStorage) (
		partitions []*simulator.PartitionConfig, windows map[string]int, err error)
}

// macroSpecFactories maps a macro type to a factory for its empty typed spec.
// Aggregation and inference/stats macros are here; MCTS and SMC stay in Go (their
// models are user closures), and evolution-strategy optimisation is a live run,
// not an against-storage analysis.
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
		return d.Source.load()
	}
	run := RunConfig{Partitions: d.Partitions, Expressions: d.Expressions}
	if err := resolveIterations(run.Partitions); err != nil {
		return nil, err
	}
	generator := run.GetConfigGenerator()
	// Pre-flight the data: sub-simulation's within-step wiring, so a cyclic data
	// block fails with a located error rather than an opaque runtime deadlock.
	if err := CheckForDeadlock(generator); err != nil {
		return nil, err
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

// runMacros expands and runs each macro in turn, returning the resulting storage.
// A live macro (evolution_strategy_optimisation) runs its partitions as a fresh
// simulation; an against-storage macro runs against the data: storage, which is
// built lazily on first use — so a live-only config needs no data: block. Running
// in turn lets a later against-storage macro reference an earlier one's output.
func runMacros(config *ApiRunConfig) (*simulator.StateTimeStorage, error) {
	if len(config.Main.Partitions) > 0 {
		return nil, fmt.Errorf("api: a config sets both main.partitions and macros:; " +
			"macros run in their own context and ignore main — put data-generating " +
			"partitions under data:, not main")
	}
	var storage *simulator.StateTimeStorage
	ensureStorage := func() error {
		if storage != nil {
			return nil
		}
		if config.Data == nil {
			return fmt.Errorf("api: against-storage macros require a data: block to analyse")
		}
		built, err := config.Data.buildStorage()
		storage = built
		return err
	}
	for i := range config.Macros {
		macro := &config.Macros[i]
		if live, ok := macro.Spec.(liveMacroSpec); ok {
			// A live macro may still need observed data (smc_inference): build the
			// data: storage when one is configured, but tolerate its absence for
			// live macros that need none (evolution_strategy_optimisation).
			if storage == nil && config.Data != nil {
				if err := ensureStorage(); err != nil {
					return nil, err
				}
			}
			partitions, steps, timestep, err := live.resolveLive(storage)
			if err != nil {
				return nil, fmt.Errorf("macro %q: %w", macro.Type, err)
			}
			storage = analysis.NewStateTimeStorageFromPartitions(
				partitions,
				&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: steps},
				&simulator.ConstantTimestepFunction{Stepsize: timestep},
				0.0,
			)
			continue
		}
		if err := ensureStorage(); err != nil {
			return nil, err
		}
		partitions, windows, err := macro.Spec.resolve(storage)
		if err != nil {
			return nil, fmt.Errorf("macro %q: %w", macro.Type, err)
		}
		storage = analysis.AddPartitionsToStateTimeStorage(storage, partitions, windows)
	}
	if storage == nil {
		return nil, fmt.Errorf("api: no macros produced any output")
	}
	return storage, nil
}

// applyParams merges a macro's params: into a generated partition's Params — how
// kernel parameters such as exponential_weighting_timescale reach the iteration.
func applyParams(partition *simulator.PartitionConfig, params map[string][]float64) {
	for name, values := range params {
		partition.Params.Set(name, values)
	}
}
