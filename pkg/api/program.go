package api

import (
	"fmt"
	"os"
	"text/template"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gopkg.in/yaml.v2"
)

// PartitionConfigStrings describes a partition in its string-templated form
// for code generation and dynamic program construction.
//
// This struct is used to generate Go code from YAML configurations, allowing
// for flexible simulation setups without hardcoded partition types. It supports
// dynamic iteration creation, package imports, and variable injection.
//
// Fields:
//   - Name: Partition name (must be unique within a simulation)
//   - Iteration: Go expression that constructs the iteration (e.g., "&continuous.WienerProcessIteration{}")
//   - ExtraPackages: Import paths required by the Iteration expression or ExtraVars
//   - ExtraVars: Ad-hoc variable declarations injected into the generated main function
//
// Code Generation:
// The Iteration field is evaluated as Go code, allowing for parameterized
// iteration construction. ExtraVars provide additional context for the
// generated code.
//
// Example:
//
//	config := PartitionConfigStrings{
//	    Name: "brownian_motion",
//	    Iteration: "&continuous.WienerProcessIteration{}",
//	    ExtraPackages: []string{"github.com/umbralcalc/stochadex/pkg/continuous"},
//	    ExtraVars: []map[string]string{
//	        {"variance": "0.1"},
//	        {"dimensions": "2"},
//	    },
//	}
//
// Validation:
//   - Iteration must be a valid Go expression
//   - ExtraPackages must be valid import paths
//   - ExtraVars must be valid Go variable declarations
type PartitionConfigStrings struct {
	Name string `yaml:"name"`
	// Iteration is the ComponentSpec union: a Go-expression string (resolved by
	// codegen) or a {type: ...} data spec (resolved at load by the registry).
	Iteration     simulator.ComponentSpec `yaml:"iteration,omitempty"`
	ExtraPackages []string                `yaml:"extra_packages,omitempty"`
	ExtraVars     []map[string]string     `yaml:"extra_vars,omitempty"`
}

// ExpressionConfig binds a declarative expression specification to a partition by name, so
// that a partition's whole update can be written as data in the config file.
//
// Unlike the iteration field — which is a Go expression, and so requires this package's
// code-generation step and a Go toolchain — an expression specification is just data: it is
// loaded straight from the YAML and evaluated at run time. A config using only expressions
// therefore needs no compilation at all, which is what lets a simulation be specified by
// something that does not write Go.
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
//   - Simulation: Simulation control parameters (not loaded from YAML directly)
//
// YAML Structure:
//
//	partitions:
//	  - name: "process1"
//	    iteration: "&continuous.WienerProcessIteration{}"
//	    params:
//	      variances: [0.1, 0.2]
//	    init_state_values: [0.0, 0.0]
//	  - name: "process2"
//	    iteration: "&discrete.PoissonProcessIteration{}"
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
	// ComponentSpec unions). Data-spec components are resolved into Simulation at
	// load time; Go-expression components are filled by the code-generation step.
	SimulationStrings simulator.SimulationConfigStrings `yaml:"simulation"`
	// Simulation is the resolved simulation config used to build the generator.
	Simulation simulator.SimulationConfig `yaml:"-"`
}

// resolve fills the run's data-spec components at load time: the simulation
// components given as data specs, and each partition whose iteration: was given
// as a data spec. Go-expression components and iterations are left for codegen.
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

// RunConfigStrings provides the string-templated inputs required to generate
// a runnable main for a simulation run.
type RunConfigStrings struct {
	Partitions []PartitionConfigStrings `yaml:"partitions"`
	// Expressions are data, so this view holds the same type as RunConfig. Validation reads
	// it to know which partitions may legitimately omit an iteration.
	Expressions []ExpressionConfig                `yaml:"expressions,omitempty"`
	Simulation  simulator.SimulationConfigStrings `yaml:"simulation"`
}

// EmbeddedRunConfigStrings names and provides string-templated inputs for an
// embedded simulation run.
type EmbeddedRunConfigStrings struct {
	Name string           `yaml:"name"`
	Run  RunConfigStrings `yaml:",inline"`
}

// RunModeConfig selects what a run *does* with the assembled simulation — the
// one thing that is not a partition and that the partition tiers cannot express.
// It is pure data (no Go expressions), so the same type serves both the concrete
// and the code-generation views of a config.
//
// Modes:
//   - "" or "batch": run once to completion (or serve a websocket when a socket
//     config is active). This is the default and matches pre-run:-tier behaviour
//     exactly, so no existing config changes meaning.
//   - "ensemble": run one member per seed concurrently, varying the global seed,
//     via simulator.RunSeededEnsemble. Currently supports data-only (expressions)
//     configs, because each member is rebuilt by re-loading the source file to get
//     fresh, non-shared iteration instances.
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

// LoadApiRunConfigFromYaml loads simulation configuration from YAML file.
//
// This function reads a complete API run configuration from a YAML file,
// including main simulation configuration and optional embedded runs.
// It automatically initializes partition defaults and validates the configuration.
//
// Parameters:
//   - path: Path to the YAML configuration file (must exist and be readable)
//
// Returns:
//   - *ApiRunConfig: Loaded and initialized configuration ready for execution
//
// YAML File Format:
// The YAML file must contain:
//
//	main:
//	  partitions:
//	    - name: "process1"
//	      iteration: "&continuous.WienerProcessIteration{}"
//	      params:
//	        variances: [0.1, 0.2]
//	      init_state_values: [0.0, 0.0]
//	      state_history_depth: 10
//	      state_width: 2
//	      seed: 42
//	  simulation:
//	    output_condition: "&simulator.StepCountOutputCondition{MaxSteps: 100}"
//	    output_function: "&simulator.StateTimeStorageOutputFunction{Store: store}"
//	    termination_condition: "&simulator.StepCountTerminationCondition{MaxSteps: 1000}"
//	    timestep_function: "&simulator.ConstantTimestepFunction{Timestep: 0.01}"
//	    init_time_value: 0.0
//	embedded:
//	  - name: "sub_simulation"
//	    partitions: [...]
//	    simulation: [...]
//
// Error Handling:
//   - Panics on file read errors (file not found, permission denied)
//   - Panics on YAML parsing errors (malformed YAML, type mismatches)
//   - Automatically initializes partition defaults on success
//
// Example:
//
//	config := LoadApiRunConfigFromYaml("simulation_config.yaml")
//	generator := config.GetConfigGenerator()
//	// Use generator to run the simulation
//
// Performance Notes:
//   - Loads entire file into memory
//   - O(n) time complexity where n is the YAML file size
//   - Memory usage scales with configuration complexity
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
	// Resolve any data-spec simulation components and data-spec iterations at load
	// time, so a config that is fully data needs no code generation.
	if simErr := config.Main.resolve(); simErr != nil {
		panic(simErr)
	}
	for index := range config.Embedded {
		if simErr := config.Embedded[index].Run.resolve(); simErr != nil {
			panic(simErr)
		}
	}
	config.sourcePath = path
	return &config
}

// ApiRunConfigStrings is the string-templated configuration used to generate
// code for an API run (imports, variables, iteration factories).
type ApiRunConfigStrings struct {
	Main     RunConfigStrings           `yaml:"main"`
	Embedded []EmbeddedRunConfigStrings `yaml:"embedded,omitempty"`
	// Run is pure data and identical across views, so the templated view holds it
	// too — both so the code-generation path can honour it and so the dead-key
	// check (which flags a key only when BOTH views reject it) accepts `run:`.
	Run RunModeConfig `yaml:"run,omitempty"`
	// Data and Macros are pure data; the templated view holds them so the dead-key
	// check accepts data: and macros:. Analysis runs are always in-process.
	Data   *DataConfig   `yaml:"data,omitempty"`
	Macros []MacroConfig `yaml:"macros,omitempty"`
}

// runIsData reports whether a single run's partitions and simulation are entirely
// data (no Go): no partition names a Go iteration, extra_packages or extra_vars (a
// data-spec iteration or an expression is fine), and all four simulation
// components are data specs.
func runIsData(run RunConfigStrings) bool {
	for _, partition := range run.Partitions {
		if partition.Iteration.GoExpr != "" ||
			len(partition.ExtraPackages) > 0 ||
			len(partition.ExtraVars) > 0 {
			return false
		}
	}
	return run.Simulation.FullyData()
}

// IsFullyData reports whether the whole config — main and every embedded run — is
// data-only, so it can be resolved and run in-process with no code generation and
// no Go toolchain.
func (a *ApiRunConfigStrings) IsFullyData() bool {
	if !runIsData(a.Main) {
		return false
	}
	for _, embedded := range a.Embedded {
		if !runIsData(embedded.Run) {
			return false
		}
	}
	return true
}

// validateApiRunConfigStrings asserts the templated config is coherent.
// Any partition without an Iteration must correspond to a named embedded run.
func validateApiRunConfigStrings(config *ApiRunConfigStrings) {
	embeddedNames := make(map[string]bool)
	for _, embedded := range config.Embedded {
		embeddedNames[embedded.Name] = true
	}
	expressionNames := make(map[string]bool)
	for _, expression := range config.Main.Expressions {
		expressionNames[expression.Partition] = true
	}
	for _, partition := range config.Main.Partitions {
		if partition.Iteration.IsZero() {
			if !embeddedNames[partition.Name] && !expressionNames[partition.Name] {
				panic("config omits iteration for partition name: " +
					partition.Name +
					" and no embedded simulation runs or expression specs have this name")
			}
		}
	}
}

// LoadApiRunConfigStringsFromYaml loads the templated config from YAML and
// validates it for code generation.
func LoadApiRunConfigStringsFromYaml(path string) *ApiRunConfigStrings {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	if deadKeyErr := validateNoDeadKeys(yamlFile); deadKeyErr != nil {
		panic(deadKeyErr)
	}
	var config ApiRunConfigStrings
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		panic(err)
	}
	validateApiRunConfigStrings(&config)
	return &config
}

// simComponentAssignments emits a Go assignment for each simulation component
// given in Go-expression form, targeting fields of an already-loaded
// SimulationConfig (the target, e.g. "config.Main.Simulation"). Components given
// in data-spec form are skipped — LoadApiRunConfigFromYaml has already resolved
// them into the loaded config, so codegen must not overwrite them. InitTimeValue
// is likewise set at load and not re-emitted.
func simComponentAssignments(target string, sim simulator.SimulationConfigStrings) string {
	code := ""
	fields := []struct {
		name string
		spec simulator.ComponentSpec
	}{
		{"OutputCondition", sim.OutputCondition},
		{"OutputFunction", sim.OutputFunction},
		{"TerminationCondition", sim.TerminationCondition},
		{"TimestepFunction", sim.TimestepFunction},
	}
	for _, field := range fields {
		if field.spec.GoExpr == "" {
			continue
		}
		code += fmt.Sprintf("%s.%s = %s\n    ", target, field.name, field.spec.GoExpr)
	}
	if sim.ExecutionStrategy != "" {
		code += fmt.Sprintf("%s.ExecutionStrategy = %s\n    ", target, sim.ExecutionStrategy)
	}
	return code
}

// formatExtraCode serialises Iteration factories and the Go-expression simulation
// components into Go code fragments for main and embedded runs. Data-spec
// components are resolved at load and need no code here.
func formatExtraCode(args ParsedArgs) string {
	extraCode := ""
	for i, partition := range args.ConfigStrings.Main.Partitions {
		if partition.Iteration.GoExpr == "" {
			continue
		}
		extraCode += fmt.Sprintf(
			`config.Main.Partitions[%d].Iteration = %s`+"\n    ",
			i, partition.Iteration.GoExpr,
		)
	}
	extraCode += simComponentAssignments(
		"config.Main.Simulation", args.ConfigStrings.Main.Simulation,
	)
	for i, embedded := range args.ConfigStrings.Embedded {
		for j, partition := range embedded.Run.Partitions {
			if partition.Iteration.GoExpr == "" {
				continue
			}
			extraCode += fmt.Sprintf(
				`config.Embedded[%d].Run.Partitions[%d].Iteration = %s`+"\n    ",
				i, j, partition.Iteration.GoExpr,
			)
		}
		extraCode += simComponentAssignments(
			fmt.Sprintf("config.Embedded[%d].Run.Simulation", i),
			embedded.Run.Simulation,
		)
	}
	return extraCode
}

// formatExtraVariables emits variable declarations from the templated config
// so they are available to code fragments.
func formatExtraVariables(args ParsedArgs) string {
	extraVariables := ""
	for _, partition := range args.ConfigStrings.Main.Partitions {
		if partition.ExtraVars == nil {
			continue
		}
		for _, extraVars := range partition.ExtraVars {
			for varName, varValue := range extraVars {
				extraVariables += varName + " := " + varValue + "\n    "
			}
		}
	}
	for _, embedded := range args.ConfigStrings.Embedded {
		for _, partition := range embedded.Run.Partitions {
			if partition.ExtraVars == nil {
				continue
			}
			for _, extraVars := range partition.ExtraVars {
				for varName, varValue := range extraVars {
					extraVariables += varName + " := " + varValue + "\n    "
				}
			}
		}
	}
	return extraVariables
}

// formatExtraPackages de-duplicates and renders import paths required by code
// fragments for main and embedded runs.
func formatExtraPackages(args ParsedArgs) string {
	extraPackages := ""
	extraPackagesSet := make(map[string]bool)
	if args.ConfigStrings.Embedded != nil {
		extraPackagesSet["github.com/umbralcalc/stochadex/pkg/general"] = true
	}
	for _, partition := range args.ConfigStrings.Main.Partitions {
		if partition.ExtraPackages == nil {
			continue
		}
		for _, packageName := range partition.ExtraPackages {
			extraPackagesSet[packageName] = true
		}
	}
	for _, embedded := range args.ConfigStrings.Embedded {
		for _, partition := range embedded.Run.Partitions {
			if partition.ExtraPackages == nil {
				continue
			}
			for _, packageName := range partition.ExtraPackages {
				extraPackagesSet[packageName] = true
			}
		}
	}
	for extraPackage := range extraPackagesSet {
		if extraPackage != "" {
			extraPackages += "\"" + extraPackage + "\"" + "\n    "
		}
	}
	return extraPackages
}

// ApiCodeTemplate is the Go source template used to generate a runnable
// temporary main program that executes the requested run configuration.
var ApiCodeTemplate = `package main

import (
	"github.com/umbralcalc/stochadex/pkg/api"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	{{.ExtraPackages}}
)

func main() {
	config := api.LoadApiRunConfigFromYaml("{{.ConfigFile}}")
	socket := api.LoadSocketConfigFromYaml("{{.SocketFile}}")
	{{.ExtraVars}}
	{{.ExtraCode}}
    api.Run(config, socket)
}`

// WriteMainProgram renders ApiCodeTemplate to a /tmp/*main.go and returns the
// file path.
//
// Usage hints:
//   - The generated program imports extra packages, declares extra variables,
//     and wires iterations per the templated config.
func WriteMainProgram(args ParsedArgs) string {
	fmt.Println("\nParsed run config ...")
	codeTemplate := template.New("stochadexMain")
	template.Must(codeTemplate.Parse(ApiCodeTemplate))
	file, err := os.CreateTemp("/tmp", "*main.go")
	if err != nil {
		err := os.Mkdir("/tmp", 0755)
		if err != nil {
			panic(err)
		}
		file, _ = os.CreateTemp("/tmp", "*main.go")
	}
	err = codeTemplate.Execute(
		file,
		map[string]string{
			"ConfigFile":    args.ConfigFile,
			"SocketFile":    args.SocketFile,
			"ExtraVars":     formatExtraVariables(args),
			"ExtraPackages": formatExtraPackages(args),
			"ExtraCode":     formatExtraCode(args),
		},
	)
	if err != nil {
		panic(err)
	}
	return file.Name()
}
