package api

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

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
	return r.resolveWith(true)
}

// resolveWith is resolve, leaving the output pair unresolved when withOutputs is
// false: the main run's pair is resolved as an outputs: view instead (see
// ApiRunConfig.resolveOutputs), where a served {type: connection} is known.
func (r *RunConfig) resolveWith(withOutputs bool) error {
	components := r.SimulationStrings
	if !withOutputs {
		components.OutputCondition = simulator.ComponentSpec{}
		components.OutputFunction = simulator.ComponentSpec{}
	}
	resolved, err := components.ResolveDataComponents()
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
//   - "" or "batch": run once to completion. This is the default.
//   - "ensemble": run one member per seed concurrently, varying the global seed,
//     via simulator.RunSeededEnsemble. Each member is rebuilt by re-loading the
//     source file to get fresh, non-shared iteration instances.
//   - "serve": serve a websocket; each connection gets its own fresh run of the
//     model, streamed to it step by step (see serve.go).
type RunModeConfig struct {
	Mode string `yaml:"mode,omitempty"`
	// Seeds are the per-member global seeds for ensemble mode (one member each).
	Seeds []uint64 `yaml:"seeds,omitempty"`
	// Concurrency bounds how many ensemble members run at once; <= 0 defaults to
	// GOMAXPROCS.
	Concurrency int `yaml:"concurrency,omitempty"`
	// Websocket is where serve mode listens.
	Websocket *WebsocketServeConfig `yaml:"websocket,omitempty"`
	// PaceMs is serve mode's delay between steps, in milliseconds, so a client
	// sees the run at a watchable rate.
	PaceMs uint64 `yaml:"pace_ms,omitempty"`
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
	// Outputs are the run's output views: each a named sink gated by its own
	// condition, all fed from the one run. Mutually exclusive with
	// main.simulation's output_condition / output_function, which are shorthand
	// for a single unnamed view.
	Outputs []OutputViewConfig `yaml:"outputs,omitempty"`
	// Inputs are named, read-only storages read when a run starts and replayed
	// into main: partitions with {type: from_input} (see inputs.go).
	Inputs map[string]InputConfig `yaml:"inputs,omitempty"`
	// sourcePath records the file this config was loaded from, so ensemble mode
	// can re-load it to build fresh, isolated members. Empty for a config built
	// in-memory rather than via LoadApiRunConfigFromYaml.
	sourcePath string `yaml:"-"`
	// outputViews are the resolved outputs: views, including the one made from
	// the shorthand output pair or the default (see resolveOutputs). The main
	// path also installs them as its simulation's output; the macros path
	// replays its result through them (see replayThroughViews).
	outputViews *simulator.OutputViews
	// outputsDeclared reports whether the config itself wrote outputs:, as
	// opposed to Outputs holding the shorthand or default view.
	outputsDeclared bool
	// socketAlias marks a config served through the deprecated --socket alias
	// (see withSocketAlias).
	socketAlias bool
	// boundParamRows holds the stored input rows params_from_input keys read,
	// by "partition/key", once a run has read them (see storedParamRows).
	boundParamRows map[string][][]float64
}

// OutputViewConfig is one entry of outputs:. Condition defaults to every_step.
type OutputViewConfig struct {
	Name      string                  `yaml:"name"`
	Condition simulator.ComponentSpec `yaml:"condition,omitempty"`
	Function  simulator.ComponentSpec `yaml:"function"`
	// fromShorthand marks the view made from main.simulation's output pair, or
	// the default when no output is declared, so errors can name what the user
	// actually wrote.
	fromShorthand bool
}

// label names a view in errors as the user wrote it.
func (v OutputViewConfig) label() string {
	if v.fromShorthand {
		return "main.simulation's output_function"
	}
	return fmt.Sprintf("output view %q", v.Name)
}

// shorthandView names the view that main.simulation's output_condition /
// output_function pair, or the default when no output is declared, becomes.
const shorthandView = "output"

// resolveOutputs makes outputs: the one place a config's outputs are declared
// (PLAN.md rule 14), then builds its views and installs them as the main
// simulation's output, so every run path (batch, ensemble, serve, macros) uses
// them and every path that replaces the output (RunToStorage, serving a
// connection) suppresses them alike.
//
// main.simulation's output_condition / output_function pair is shorthand for
// one view named "output", and a config declaring no output at all gets the
// default view: every step to stdout.
func (a *ApiRunConfig) resolveOutputs() error {
	pair := &a.Main.SimulationStrings
	shorthand := !pair.OutputCondition.IsZero() || !pair.OutputFunction.IsZero()
	if len(a.Outputs) > 0 && shorthand {
		return fmt.Errorf("api: a config sets both outputs: and " +
			"main.simulation.output_condition / output_function; the latter is " +
			"shorthand for a single view — move it into outputs:")
	}
	a.outputsDeclared = len(a.Outputs) > 0
	// The run's own condition is what RunToStorage mirrors: the shorthand's, or
	// every step.
	var mirror simulator.OutputCondition = &simulator.EveryStepOutputCondition{}
	switch {
	case shorthand:
		function := pair.OutputFunction
		if function.IsZero() {
			function = simulator.ComponentSpec{Type: "stdout"}
		}
		a.Outputs = []OutputViewConfig{{Name: shorthandView, Condition: pair.OutputCondition,
			Function: function, fromShorthand: true}}
		if !pair.OutputCondition.IsZero() {
			resolved, err := simulator.ResolveOutputCondition(pair.OutputCondition)
			if err != nil {
				return fmt.Errorf("api: main.simulation's output_condition: %w", err)
			}
			mirror = resolved
		}
	case len(a.Outputs) == 0:
		a.Outputs = []OutputViewConfig{{Name: shorthandView,
			Function: simulator.ComponentSpec{Type: "stdout"}, fromShorthand: true}}
	}
	views := make([]simulator.OutputView, 0, len(a.Outputs))
	seen := make(map[string]bool, len(a.Outputs))
	for index, view := range a.Outputs {
		if view.Name == "" {
			return fmt.Errorf("api: outputs[%d] needs a name", index)
		}
		if seen[view.Name] {
			return fmt.Errorf("api: outputs: names view %q twice", view.Name)
		}
		seen[view.Name] = true
		if view.Function.IsZero() {
			return fmt.Errorf("api: output view %q needs a function: {type: ...}", view.Name)
		}
		if err := checkInstancePlaceholders(view, a.Run.Mode); err != nil {
			return err
		}
		var function simulator.OutputFunction = &simulator.NilOutputFunction{}
		if view.Function.Type == connectionSink {
			// Stands in until a served connection substitutes its websocket.
			if len(view.Function.Fields) > 0 {
				return fmt.Errorf("api: %s: {type: connection} takes no "+
					"fields; it sends to whichever client the run is served to", view.label())
			}
		} else {
			resolved, err := simulator.ResolveOutputFunction(view.Function)
			if err != nil {
				return fmt.Errorf("api: %s: %w", view.label(), err)
			}
			function = resolved
		}
		var condition simulator.OutputCondition = &simulator.EveryStepOutputCondition{}
		if !view.Condition.IsZero() {
			resolved, err := simulator.ResolveOutputCondition(view.Condition)
			if err != nil {
				return fmt.Errorf("api: %s condition: %w", view.label(), err)
			}
			condition = resolved
		}
		views = append(views, simulator.OutputView{
			Name: view.Name, Condition: condition, Function: function,
		})
	}
	a.outputViews = &simulator.OutputViews{Views: views}
	a.Main.Simulation.OutputFunction = a.outputViews
	// Not consulted for OutputViews (each view has its own condition).
	a.Main.Simulation.OutputCondition = mirror
	return nil
}

// connectionSink is the output function type that sends a view's rows to the
// client a run is served to (run: {mode: serve}).
const connectionSink = "connection"

// memberPlaceholders are substituted in an ensemble member's output views, and
// connectionPlaceholders in a served connection's.
var (
	memberPlaceholders     = []string{"{member}", "{seed}"}
	connectionPlaceholders = []string{"{connection}"}
)

// checkInstancePlaceholders applies the rule for run modes that write one set
// of output views per run instance — an ensemble member, a served connection.
// Each instance gets its own sinks, so a view must name its destination with
// that mode's placeholder or every instance writes to the same place; and a
// placeholder for another mode would never be substituted.
func checkInstancePlaceholders(view OutputViewConfig, mode string) error {
	member := hasPlaceholder(view.Function.Fields, memberPlaceholders)
	connection := hasPlaceholder(view.Function.Fields, connectionPlaceholders)
	// A nil sink writes nowhere, a connection sink is the instance's own, and
	// stdout tags each row with the instance that wrote it.
	shared := view.Function.Type != "nil" && view.Function.Type != connectionSink &&
		view.Function.Type != "stdout"
	switch {
	case mode != "serve" && view.Function.Type == connectionSink:
		return fmt.Errorf("api: %s sends to {type: connection}, which "+
			"only applies to run: {mode: serve}", view.label())
	case mode == "ensemble" && !member && shared:
		return fmt.Errorf("api: %s would have every ensemble member "+
			"write to the same destination; put {member} or {seed} in it, e.g. "+
			"path: run-{member}.log", view.label())
	case mode == "serve" && !connection && shared:
		return fmt.Errorf("api: %s would have every served connection "+
			"write to the same destination; put {connection} in it, e.g. "+
			"path: run-{connection}.log", view.label())
	case mode != "ensemble" && member:
		return fmt.Errorf("api: %s uses {member} / {seed}, which only "+
			"apply to run: {mode: ensemble}", view.label())
	case mode != "serve" && connection:
		return fmt.Errorf("api: %s uses {connection}, which only "+
			"applies to run: {mode: serve}", view.label())
	}
	return nil
}

// hasPlaceholder reports whether any of a spec's string fields contains one of
// placeholders. Sink specs are flat, so only top-level fields are read: a
// placeholder anywhere deeper is not found, and validation then rejects the
// view as shared by every instance rather than writing to the wrong place.
func hasPlaceholder(fields map[string]interface{}, placeholders []string) bool {
	for _, value := range fields {
		text, ok := value.(string)
		if !ok {
			continue
		}
		for _, placeholder := range placeholders {
			if strings.Contains(text, placeholder) {
				return true
			}
		}
	}
	return false
}

// substituteFields returns a copy of a spec's fields with replacer applied to
// its string fields.
func substituteFields(fields map[string]interface{}, replacer *strings.Replacer) map[string]interface{} {
	out := make(map[string]interface{}, len(fields))
	for key, value := range fields {
		if text, ok := value.(string); ok {
			value = replacer.Replace(text)
		}
		out[key] = value
	}
	return out
}

// memberOutputViews builds one ensemble member's own output views: fresh sink
// instances, with {member} (its index) and {seed} substituted in every view's
// function fields.
func (a *ApiRunConfig) memberOutputViews(member int, seed uint64) *simulator.OutputViews {
	return a.instanceOutputViews(fmt.Sprintf("member=%d seed=%d", member, seed),
		strings.NewReplacer("{member}", strconv.Itoa(member),
			"{seed}", strconv.FormatUint(seed, 10)), nil)
}

// connectionOutputViews builds one served connection's own output views: fresh
// sink instances, with {connection} (its index) substituted, and the connection
// view sending to stream, that connection's websocket.
func (a *ApiRunConfig) connectionOutputViews(
	connection int,
	stream simulator.OutputFunction,
) *simulator.OutputViews {
	return a.instanceOutputViews(fmt.Sprintf("connection=%d", connection),
		strings.NewReplacer("{connection}", strconv.Itoa(connection)), stream)
}

// instanceOutputViews builds fresh sink instances for every output view, with
// replacer applied to each function's fields; a connection view sends to stream,
// and a stdout view prefixes each row with instance ("member=0 seed=11"). The
// specs were validated at load, so a failure here is a bug.
func (a *ApiRunConfig) instanceOutputViews(
	instance string,
	replacer *strings.Replacer,
	stream simulator.OutputFunction,
) *simulator.OutputViews {
	views := &simulator.OutputViews{}
	for _, view := range a.Outputs {
		function := stream
		if view.Function.Type != connectionSink {
			spec := view.Function
			spec.Fields = substituteFields(view.Function.Fields, replacer)
			resolved, err := simulator.ResolveOutputFunction(spec)
			if err != nil {
				panic(fmt.Errorf("api: output view %q for %s: %w", view.Name, instance, err))
			}
			if stdout, ok := resolved.(*simulator.StdoutOutputFunction); ok {
				stdout.Prefix = instance
			}
			function = resolved
		}
		var condition simulator.OutputCondition = &simulator.EveryStepOutputCondition{}
		if !view.Condition.IsZero() {
			resolved, err := simulator.ResolveOutputCondition(view.Condition)
			if err != nil {
				panic(fmt.Errorf("api: output view %q condition: %w", view.Name, err))
			}
			condition = resolved
		}
		views.Views = append(views.Views, simulator.OutputView{
			Name: view.Name, Condition: condition, Function: function})
	}
	return views
}

// GetConfigGenerator returns a ConfigGenerator for the main run. Any partition
// whose name matches an embedded run is replaced by an embedded simulation
// iteration wired to that embedded run.
func (a *ApiRunConfig) GetConfigGenerator() *simulator.ConfigGenerator {
	return a.configGenerator(false)
}

// configGenerator builds the main run's generator. With silenceNested, each
// embedded run is built from a copy of its config whose output is nil, so its
// own sinks stay closed — how a run whose config outputs are suppressed keeps
// nested outputs suppressed too. The config itself is not modified.
func (a *ApiRunConfig) configGenerator(silenceNested bool) *simulator.ConfigGenerator {
	generator := a.Main.GetConfigGenerator()
	for _, embedded := range a.Embedded {
		run := embedded.Run
		if silenceNested {
			run.Simulation.OutputCondition = &simulator.NilOutputCondition{}
			run.Simulation.OutputFunction = &simulator.NilOutputFunction{}
		}
		partition := generator.GetPartition(embedded.Name)
		partition.Iteration = general.NewEmbeddedSimulationRunIteration(
			run.GetConfigGenerator().GenerateConfigs(),
		)
		generator.ResetPartition(embedded.Name, partition)
	}
	return generator
}

// validateApiRunConfig asserts the loaded config is coherent. Any partition
// without an iteration must correspond to a named embedded run or expression.
func validateApiRunConfig(config *ApiRunConfig) error {
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
				return fmt.Errorf("config omits iteration for partition name: %s"+
					" and no embedded simulation runs or expression specs have this name",
					partition.Name)
			}
		}
	}
	return validateEmbeddedForwarding(config)
}

// validateEmbeddedForwarding checks, at load, that every params key a host
// partition forwards into its embedded run ("<inner_partition>/<param>", in its
// params or params_from_upstream) names a partition the
// embedded run has. Without it, a misspelled inner partition fails the run
// instead of the load.
func validateEmbeddedForwarding(config *ApiRunConfig) error {
	inner := map[string]map[string]bool{}
	for _, embedded := range config.Embedded {
		inner[embedded.Name] = map[string]bool{}
		for _, partition := range embedded.Run.Partitions {
			inner[embedded.Name][partition.Name] = true
		}
	}
	for _, host := range config.Main.Partitions {
		partitions, isHost := inner[host.Name]
		if !isHost {
			continue
		}
		keys := []string{}
		for key := range host.Params.Map {
			keys = append(keys, key)
		}
		// A params_from_input key is always declared in params too.
		for key := range host.ParamsFromUpstream {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			innerName, _, forwards := general.ForwardedParamTarget(key)
			if forwards && !partitions[innerName] {
				names := make([]string, 0, len(partitions))
				for name := range partitions {
					names = append(names, name)
				}
				sort.Strings(names)
				return fmt.Errorf("partition %q forwards params key %q into embedded run "+
					"%q, which has no partition %q (it has: %s)", host.Name, key, host.Name,
					innerName, strings.Join(names, ", "))
			}
		}
	}
	return nil
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
	config, err := LoadConfig(path)
	if err != nil {
		panic(err.(*Error).Err)
	}
	return config
}

// LoadConfig loads and resolves a config like LoadApiRunConfigFromYaml, but
// returns a failure instead of panicking. Every failure is an ErrConfig *Error:
// an unreadable file, invalid YAML, a key nothing reads, an unknown type or
// field, or a partition with no iteration.
func LoadConfig(path string) (*ApiRunConfig, error) {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		return nil, configError(err)
	}
	if deadKeyErr := validateNoDeadKeys(yamlFile); deadKeyErr != nil {
		return nil, configError(deadKeyErr)
	}
	var config ApiRunConfig
	if err := yaml.Unmarshal(yamlFile, &config); err != nil {
		return nil, configError(err)
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
	if err := config.Main.resolveWith(false); err != nil {
		return nil, configError(err)
	}
	for index := range config.Embedded {
		if err := config.Embedded[index].Run.resolve(); err != nil {
			return nil, configError(err)
		}
	}
	if err := config.resolveOutputs(); err != nil {
		return nil, configError(err)
	}
	if err := validateInputs(&config); err != nil {
		return nil, configError(err)
	}
	if err := validateRunMode(&config); err != nil {
		return nil, configError(err)
	}
	if err := validateApiRunConfig(&config); err != nil {
		return nil, configError(err)
	}
	config.sourcePath = path
	return &config, nil
}
