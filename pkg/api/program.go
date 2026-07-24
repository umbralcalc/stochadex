package api

import (
	"fmt"
	"os"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gopkg.in/yaml.v2"
)

// ExpressionConfig binds a declarative expression specification to a partition by name, so
// that a partition's whole update can be written as data in the config file.
//
// An expression specification is just data: it is loaded straight from the YAML and
// evaluated at run time, so a config using only expressions needs no compilation at all.
// This is what lets a simulation be specified by something that does not write Go.
//
// A partition named here may omit its iteration field, exactly as a partition backed by an
// embedded run may. The specification is inlined, so its keys are those of
// general.ExpressionIteration:
//
//	expressions:
//	  - partition: battery
//	    fields:
//	      - {name: soc}
//	      - {name: actual_dispatch}
//	    bindings:
//	      - {name: dispatch, expr: "clamp(dispatch_mw, -power_rating_mw, power_rating_mw)"}
//	    outputs: ["clamp(soc + dispatch * dt, 0, energy_capacity_mwh)", "dispatch"]
type ExpressionConfig struct {
	Partition                   string `yaml:"partition"`
	general.ExpressionIteration `yaml:",inline"`
}

// RunConfig represents a complete simulation run configuration with partitions
// and simulation settings.
//
// This struct combines partition configurations with simulation control parameters
// to define a complete simulation run. It serves as the primary configuration
// structure for YAML-based simulation setup.
//
// Fields:
//   - Partitions: List of partition configurations defining the simulation state
//   - Expressions: Declarative iterations bound to partitions by name
//   - SimulationStrings: The simulation block as loaded (component data specs)
//   - Simulation: The resolved simulation config (not loaded from YAML directly)
//
// YAML Structure:
//
//	partitions:
//	  - name: "process1"
//	    iteration: {type: wiener_process}
//	    params:
//	      variances: [0.1, 0.2]
//	    init_state_values: [0.0, 0.0]
//	  - name: "process2"
//	    iteration: {type: poisson_process}
//	    params:
//	      rates: [0.5, 1.0]
//	    init_state_values: [0.0, 0.0]
//
// Related Types:
//   - See simulator.PartitionConfig for partition configuration details
//   - See simulator.SimulationConfig for simulation control parameters
//   - See ApiRunConfig for API-level configuration with embedded runs
//   - See ExpressionConfig for supplying a partition's iteration as data instead of Go
type RunConfig struct {
	Partitions []simulator.PartitionConfig `yaml:"partitions"`
	// Expressions declaratively supply the iteration for the partitions they name.
	Expressions []ExpressionConfig `yaml:"expressions,omitempty"`
	// SimulationStrings holds the simulation block as loaded (component fields are
	// {type: ...} data specs). It is resolved into Simulation at load time.
	SimulationStrings simulator.SimulationConfigStrings `yaml:"simulation"`
	// Simulation is the resolved simulation config used to build the generator.
	Simulation simulator.SimulationConfig `yaml:"-"`
}

// resolve fills the run's data-spec components at load time: the simulation
// components and each partition whose iteration: was given as a data spec.
func (r *RunConfig) resolve() error {
	resolved, err := r.SimulationStrings.ResolveDataComponents()
	if err != nil {
		return err
	}
	r.Simulation = *resolved
	for index := range r.Partitions {
		if !r.Partitions[index].IterationSpec.IsData() {
			continue
		}
		iteration, err := ResolveIteration(r.Partitions[index].IterationSpec)
		if err != nil {
			return fmt.Errorf(
				"partition %q: %w", r.Partitions[index].Name, err,
			)
		}
		r.Partitions[index].Iteration = iteration
	}
	return nil
}

// GetConfigGenerator constructs a ConfigGenerator preloaded with the run's
// SimulationConfig and Partitions, and gives any partition named by an Expressions entry a
// declarative ExpressionIteration built from that entry.
func (r *RunConfig) GetConfigGenerator() *simulator.ConfigGenerator {
	generator := simulator.NewConfigGenerator()
	generator.SetSimulation(&r.Simulation)
	for _, partition := range r.Partitions {
		generator.SetPartition(&partition)
	}
	// Index rather than range by value: the iteration is the embedded struct, so it has to
	// be the one owned by this config, not a copy of the loop variable.
	for i := range r.Expressions {
		expression := &r.Expressions[i]
		partition := generator.GetPartition(expression.Partition)
		if partition == nil {
			panic("api: expression names partition " + expression.Partition +
				" but no partition of that name is defined")
		}
		partition.Iteration = &expression.ExpressionIteration
		generator.ResetPartition(expression.Partition, partition)
	}
	return generator
}

// EmbeddedRunConfig names and embeds an additional RunConfig that can be
// wired into a partition in the main run.
type EmbeddedRunConfig struct {
	Name string    `yaml:"name"`
	Run  RunConfig `yaml:",inline"`
}

// RunModeConfig selects what a run *does* with the assembled simulation — the
// one thing that is not a partition and that the partition tiers cannot express.
//
// Modes:
//   - "" or "batch": run once to completion (or serve a websocket when a socket
//     config is active). This is the default.
//   - "ensemble": run one member per seed concurrently, varying the global seed,
//     via simulator.RunSeededEnsemble. Each member is rebuilt by re-loading the
//     source file to get fresh, non-shared iteration instances.
type RunModeConfig struct {
	Mode string `yaml:"mode,omitempty"`
	// Seeds are the per-member global seeds for ensemble mode (one member each).
	Seeds []uint64 `yaml:"seeds,omitempty"`
	// Concurrency bounds how many ensemble members run at once; <= 0 defaults to
	// GOMAXPROCS.
	Concurrency int `yaml:"concurrency,omitempty"`
}

// ApiRunConfig is the concrete, YAML-loadable configuration for an API run:
// a main RunConfig, optional embedded runs, and an optional run-mode selector.
type ApiRunConfig struct {
	Main     RunConfig           `yaml:"main"`
	Embedded []EmbeddedRunConfig `yaml:"embedded,omitempty"`
	Run      RunModeConfig       `yaml:"run,omitempty"`
	// Data is the optional data: tier — a sub-simulation run to produce storage
	// for the macros: tier to analyse.
	Data *DataConfig `yaml:"data,omitempty"`
	// Macros is the optional macros: tier — partition-set-producing analysis
	// functions expanded against Data's storage.
	Macros []MacroConfig `yaml:"macros,omitempty"`
	// sourcePath records the file this config was loaded from, so ensemble mode
	// can re-load it to build fresh, isolated members. Empty for a config built
	// in-memory rather than via LoadApiRunConfigFromYaml.
	sourcePath string `yaml:"-"`
}

// GetConfigGenerator returns a ConfigGenerator for the main run. Any partition
// whose name matches an embedded run is replaced by an embedded simulation
// iteration wired to that embedded run.
func (a *ApiRunConfig) GetConfigGenerator() *simulator.ConfigGenerator {
	generator := a.Main.GetConfigGenerator()
	for _, embedded := range a.Embedded {
		partition := generator.GetPartition(embedded.Name)
		partition.Iteration = general.NewEmbeddedSimulationRunIteration(
			embedded.Run.GetConfigGenerator().GenerateConfigs(),
		)
		generator.ResetPartition(embedded.Name, partition)
	}
	return generator
}

// validateApiRunConfig asserts the loaded config is coherent. Any partition
// without an iteration must correspond to a named embedded run or expression.
func validateApiRunConfig(config *ApiRunConfig) {
	embeddedNames := make(map[string]bool)
	for _, embedded := range config.Embedded {
		embeddedNames[embedded.Name] = true
	}
	expressionNames := make(map[string]bool)
	for _, expression := range config.Main.Expressions {
		expressionNames[expression.Partition] = true
	}
	for _, partition := range config.Main.Partitions {
		if partition.IterationSpec.IsZero() {
			if !embeddedNames[partition.Name] && !expressionNames[partition.Name] {
				panic("config omits iteration for partition name: " +
					partition.Name +
					" and no embedded simulation runs or expression specs have this name")
			}
		}
	}
}

// LoadApiRunConfigFromYaml loads simulation configuration from a YAML file.
//
// The whole config is data: partition iterations are {type: ...} specs or
// expressions:, and every simulation: component is a {type: ...} spec. It resolves
// and runs in-process — no code generation, no Go toolchain.
//
// Parameters:
//   - path: Path to the YAML configuration file (must exist and be readable)
//
// Returns:
//   - *ApiRunConfig: Loaded and initialized configuration ready for execution
//
// YAML File Format:
//
//	main:
//	  partitions:
//	    - name: "process1"
//	      iteration: {type: wiener_process}
//	      params:
//	        variances: [0.1, 0.2]
//	      init_state_values: [0.0, 0.0]
//	      state_history_depth: 10
//	      seed: 42
//	  simulation:
//	    output_condition: {type: every_step}
//	    output_function: {type: stdout}
//	    termination_condition: {type: number_of_steps, max_steps: 1000}
//	    timestep_function: {type: constant, stepsize: 0.01}
//	    init_time_value: 0.0
//	embedded:
//	  - name: "sub_simulation"
//	    partitions: [...]
//	    simulation: [...]
//
// Error Handling:
//   - Panics on file read errors (file not found, permission denied)
//   - Panics on YAML parsing errors (malformed YAML, type mismatches)
//   - Panics on data-spec resolution errors (unknown type, bad field)
func LoadApiRunConfigFromYaml(path string) *ApiRunConfig {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	if deadKeyErr := validateNoDeadKeys(yamlFile); deadKeyErr != nil {
		panic(deadKeyErr)
	}
	var config ApiRunConfig
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		panic(err)
	}
	for index := range config.Main.Partitions {
		config.Main.Partitions[index].Init()
	}
	for index := range config.Embedded {
		for pIndex := range config.Embedded[index].Run.Partitions {
			config.Embedded[index].Run.Partitions[pIndex].Init()
		}
	}
	// Resolve the data-spec simulation components and data-spec iterations at load
	// time, so the whole config runs in-process with no code generation.
	if simErr := config.Main.resolve(); simErr != nil {
		panic(simErr)
	}
	for index := range config.Embedded {
		if simErr := config.Embedded[index].Run.resolve(); simErr != nil {
			panic(simErr)
		}
	}
	validateApiRunConfig(&config)
	config.sourcePath = path
	return &config
}
