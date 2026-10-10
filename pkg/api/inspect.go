package api

import (
	"fmt"
	"maps"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Inspecting a config without running it (PLAN.md O.2): Check validates it, and
// Manifest describes what it reads and writes. Both are for an orchestrator, or
// an agent, deciding about a run before spending one.
//
// Neither reads an input, opens an output or touches the network. In a
// pipeline, a step's inputs usually do not exist until the step before it has
// run, and a dry run has to work then too.

// Check validates a loaded config as far as it can without running it or
// reading its inputs: everything LoadConfig checks, plus the run mode's
// preconditions, the partitions' wiring, the deadlock pre-flight, and each
// iteration's Configure. Only what needs an input's contents is left to the
// run: that the input has the partitions the config reads, and their widths
// where the config does not declare them. A macros: config's expansion also
// needs its inputs' contents, so Check validates only its context, until
// macros expand from the config alone (PLAN.md Phase 2).
//
// Check builds fresh instances to check, so config is left as it was.
// Failures are ErrConfig.
func Check(config *ApiRunConfig) error {
	if len(config.Macros) > 0 {
		if err := validateMacroContext(config); err != nil {
			return configError(err)
		}
		return nil
	}
	if err := validateMainContext(config); err != nil {
		return configError(err)
	}
	if config.Run.Mode == "ensemble" {
		if err := validateEnsemble(config); err != nil {
			return configError(err)
		}
	}
	checked := *config
	if config.source != nil {
		reloaded, err := config.reload()
		if err != nil {
			return err
		}
		checked = *reloaded
	} else {
		checked.Main.Partitions = append([]simulator.PartitionConfig(nil), config.Main.Partitions...)
	}
	standInInputs(&checked)
	return checkWiring(&checked)
}

// checkWiring builds the main run's generator and configures every iteration,
// classifying the wiring panics GenerateConfigs raises as config errors.
func checkWiring(config *ApiRunConfig) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = configError(fmt.Errorf("invalid config wiring: %v", recovered))
		}
	}()
	generator := config.configGenerator(false)
	if err := CheckForDeadlock(generator); err != nil {
		return configError(err)
	}
	generator.GenerateConfigs()
	return nil
}

// standInInputs replaces each from_input partition, which binding would fill
// from its input's rows, with a stand-in that is never stepped. A partition
// that does not declare its width gets one wide enough for every index read
// from it, so only the run, reading the input, checks those indices.
func standInInputs(config *ApiRunConfig) {
	needed := map[string]int{}
	for _, partition := range config.Main.Partitions {
		for _, upstream := range partition.ParamsFromUpstream {
			for _, index := range upstream.Indices {
				if index+1 > needed[upstream.Upstream] {
					needed[upstream.Upstream] = index + 1
				}
			}
		}
	}
	for index := range config.Main.Partitions {
		partition := &config.Main.Partitions[index]
		if _, ok := partition.Iteration.(*unboundInputIteration); !ok {
			continue
		}
		if len(partition.InitStateValues) == 0 {
			partition.InitStateValues = make([]float64, max(needed[partition.Name], 1))
		}
		partition.Iteration = &inputStandIn{}
	}
}

// inputStandIn stands in for a from_input partition while a config is checked.
type inputStandIn struct{}

func (*inputStandIn) Configure(int, *simulator.Settings) {}

func (*inputStandIn) Iterate(
	*simulator.Params, int, []*simulator.StateHistory, *simulator.CumulativeTimestepsHistory,
) []float64 {
	panic("api: a checked config was stepped")
}

// IOManifest describes what a run of a config reads and writes, without
// running it. Its JSON form is what `stochadex inspect --io` prints.
//
// Locations are given as the config writes them: a relative path is relative
// to the directory the run starts in. Nothing secret is reported: a sink's or
// source's fields are never copied, only its location, and the values of
// ${VAR} placeholders are withheld.
type IOManifest struct {
	// Config is the path the config was loaded from.
	Config string `json:"config"`
	// Overrides are the --set overrides applied, as written; Variables the
	// ${VAR} placeholders filled, by name.
	Overrides []string        `json:"overrides,omitempty"`
	Variables []string        `json:"variables,omitempty"`
	Run       ManifestRun     `json:"run"`
	Clock     *ManifestClock  `json:"clock,omitempty"`
	Inputs    []ManifestInput `json:"inputs"`
	// Outputs are every place the run writes: the outputs: views (including
	// the shorthand and default view), each embedded run's own output, and
	// each stream input's record file.
	Outputs []ManifestOutput `json:"outputs"`
}

// ManifestRun is how many runtimes a config makes, and of what kind.
type ManifestRun struct {
	// Mode is batch, ensemble or serve.
	Mode string `json:"mode"`
	// Runtime is main, or macros for a macros: config, which runs its macros in
	// turn against its inputs (until macros expand into the main runtime).
	Runtime     string   `json:"runtime"`
	Macros      []string `json:"macros,omitempty"`
	Seeds       []uint64 `json:"seeds,omitempty"`
	Concurrency int      `json:"concurrency,omitempty"`
	Address     string   `json:"address,omitempty"`
	Handle      string   `json:"handle,omitempty"`
}

// ManifestClock is where the main run's time comes from, and what ends it. An
// input is named when a clock component reads one.
type ManifestClock struct {
	Timestep         string `json:"timestep"`
	TimestepInput    string `json:"timestep_input,omitempty"`
	Termination      string `json:"termination"`
	TerminationInput string `json:"termination_input,omitempty"`
}

// ManifestInput is one input: what kind it is, where it is, and what reads it.
type ManifestInput struct {
	Name string `json:"name"`
	// Kind is the source (csv, json_log, postgres, or a registered source such
	// as arrow or s3), simulation for an input made by a sub-simulation, or
	// stream.
	Kind     string `json:"kind"`
	Location string `json:"location,omitempty"`
	// Partitions are the input's partitions the run reads, where the config
	// names them; ReadBy says what reads the input, by config path.
	Partitions []string `json:"partitions,omitempty"`
	ReadBy     []string `json:"read_by,omitempty"`
}

// ManifestOutput is one place a run writes.
type ManifestOutput struct {
	Name string `json:"name"`
	// DeclaredIn is where the config declares it: outputs, main.simulation
	// (the shorthand pair), default (no output declared), an embedded run's
	// simulation, or a stream input's record.
	DeclaredIn string `json:"declared_in"`
	Condition  string `json:"condition"`
	Sink       string `json:"sink"`
	// Location is as declared, so under ensemble or serve it may hold
	// {member}, {seed} or {connection}. Locations lists an ensemble's, one per
	// member, with those filled in.
	Location  string   `json:"location,omitempty"`
	Locations []string `json:"locations,omitempty"`
}

// Manifest describes what config reads and writes. It reads nothing.
func Manifest(config *ApiRunConfig) *IOManifest {
	manifest := &IOManifest{
		Config:    config.sourcePath,
		Overrides: config.overrides,
		Variables: config.variables,
		Run:       manifestRun(config),
		Inputs:    []ManifestInput{},
		Outputs:   []ManifestOutput{},
	}
	if len(config.Macros) == 0 {
		manifest.Clock = &ManifestClock{
			Timestep:         config.Main.SimulationStrings.TimestepFunction.Type,
			TimestepInput:    inputField(config.Main.SimulationStrings.TimestepFunction),
			Termination:      config.Main.SimulationStrings.TerminationCondition.Type,
			TerminationInput: inputField(config.Main.SimulationStrings.TerminationCondition),
		}
	}
	readers := inputReaders(config)
	inputs := config.Inputs
	if len(config.Macros) > 0 {
		inputs = macroInputs(config)
	}
	for _, name := range sortedKeys(inputs) {
		input := inputs[name]
		entry := ManifestInput{Name: name, ReadBy: readers[name].by,
			Partitions: readers[name].partitions}
		switch {
		case input.Stream != nil:
			entry.Kind = "stream"
			if input.Stream.Websocket != nil {
				entry.Location = redactURL(input.Stream.Websocket.URL)
			} else {
				entry.Location = connectionSink
			}
		case input.Source != nil:
			entry.Kind, entry.Location = sourceLocation(input.Source)
			if input.Source.Csv != nil && len(entry.Partitions) == 0 {
				entry.Partitions = sortedKeys(input.Source.Csv.StateColumns)
			}
		default:
			entry.Kind = "simulation"
		}
		if len(config.Macros) > 0 {
			entry.ReadBy = []string{"macros"}
		}
		manifest.Inputs = append(manifest.Inputs, entry)
		if input.Record != "" {
			manifest.Outputs = append(manifest.Outputs, ManifestOutput{
				Name: name, DeclaredIn: "inputs." + name + ".record", Condition: "every_step",
				Sink: "json_log", Location: input.Record,
				Locations: memberLocations(config, input.Record),
			})
		}
	}
	for _, view := range config.Outputs {
		declared := "outputs"
		if view.fromShorthand {
			declared = "main.simulation"
			if config.Main.SimulationStrings.OutputFunction.IsZero() &&
				config.Main.SimulationStrings.OutputCondition.IsZero() {
				declared = "default"
			}
		}
		location := sinkLocation(view.Function)
		manifest.Outputs = append(manifest.Outputs, ManifestOutput{
			Name: view.Name, DeclaredIn: declared, Condition: conditionName(view.Condition),
			Sink: view.Function.Type, Location: location,
			Locations: memberLocations(config, location),
		})
	}
	for _, embedded := range config.Embedded {
		function := embedded.Run.SimulationStrings.OutputFunction
		if function.IsZero() || function.Type == "nil" {
			continue
		}
		manifest.Outputs = append(manifest.Outputs, ManifestOutput{
			Name:       embedded.Name,
			DeclaredIn: "embedded[name=" + embedded.Name + "].simulation",
			Condition:  conditionName(embedded.Run.SimulationStrings.OutputCondition),
			Sink:       function.Type, Location: sinkLocation(function),
		})
	}
	return manifest
}

func manifestRun(config *ApiRunConfig) ManifestRun {
	run := ManifestRun{Mode: config.Run.Mode, Runtime: "main", Seeds: config.Run.Seeds,
		Concurrency: config.Run.Concurrency}
	if run.Mode == "" {
		run.Mode = "batch"
	}
	if len(config.Macros) > 0 {
		run.Runtime = "macros"
		for _, macro := range config.Macros {
			run.Macros = append(run.Macros, macro.Type)
		}
	}
	if config.Run.Websocket != nil {
		run.Address, run.Handle = config.Run.Websocket.Address, config.Run.Websocket.Handle
	}
	return run
}

// readers is what reads one input: config paths, and the input partitions read.
type readers struct{ by, partitions []string }

// inputReaders finds, for each input, what in the main run reads it.
func inputReaders(config *ApiRunConfig) map[string]readers {
	found := map[string]readers{}
	add := func(input, by, partition string) {
		r := found[input]
		r.by = append(r.by, by)
		if partition != "" && !slices.Contains(r.partitions, partition) {
			r.partitions = append(r.partitions, partition)
		}
		found[input] = r
	}
	for _, partition := range config.Main.Partitions {
		at := "main.partitions[name=" + partition.Name + "]"
		if spec := partition.IterationSpec; spec.Type == "from_input" {
			input, _ := spec.Fields["input"].(string)
			source, _ := spec.Fields["partition"].(string)
			if source == "" {
				source = partition.Name
			}
			add(input, at, source)
		}
		for _, key := range sortedKeys(partition.ParamsFromInput) {
			binding := partition.ParamsFromInput[key]
			add(binding.Input, at+".params_from_input."+key, sourcePartition(binding, key))
		}
	}
	if input := inputField(config.Main.SimulationStrings.TimestepFunction); input != "" {
		add(input, "main.simulation.timestep_function", "")
	}
	if input := inputField(config.Main.SimulationStrings.TerminationCondition); input != "" {
		add(input, "main.simulation.termination_condition", "")
	}
	for name, r := range found {
		sort.Strings(r.partitions)
		found[name] = r
	}
	return found
}

// inputField is the input a clock component reads, if it reads one.
func inputField(spec simulator.ComponentSpec) string {
	if spec.Type != "from_input" && spec.Type != "input_exhausted" {
		return ""
	}
	name, _ := spec.Fields["input"].(string)
	return name
}

func conditionName(spec simulator.ComponentSpec) string {
	if spec.IsZero() {
		return "every_step"
	}
	return spec.Type
}

// memberLocations fills an ensemble's placeholders into location, once per
// member.
func memberLocations(config *ApiRunConfig, location string) []string {
	if config.Run.Mode != "ensemble" || !hasPlaceholder(
		map[string]interface{}{"": location}, memberPlaceholders) {
		return nil
	}
	locations := make([]string, len(config.Run.Seeds))
	for member, seed := range config.Run.Seeds {
		locations[member] = strings.NewReplacer("{member}", strconv.Itoa(member),
			"{seed}", strconv.FormatUint(seed, 10)).Replace(location)
	}
	return locations
}

// sinkLocation is where an output function writes.
func sinkLocation(spec simulator.ComponentSpec) string {
	switch spec.Type {
	case "stdout", connectionSink:
		return spec.Type
	case "postgres":
		return databaseLocation(spec.Fields)
	}
	return fieldsLocation(spec.Fields)
}

// sourceLocation is a stored input's kind and where it is read from.
func sourceLocation(source *DataSource) (kind, location string) {
	switch {
	case source.Csv != nil:
		return "csv", source.Csv.Path
	case source.JsonLog != nil:
		return "json_log", source.JsonLog.Path
	case source.Postgres != nil:
		return "postgres", databaseLocation(map[string]interface{}{
			"dbname": source.Postgres.Dbname, "table": source.Postgres.Table})
	}
	for _, name := range sortedKeys(source.Extra) {
		return name, fieldsLocation(source.Extra[name])
	}
	return "", ""
}

// fieldsLocation reads a location from the fields file and object stores use:
// an S3 bucket and key, a path, or a URL.
func fieldsLocation(fields map[string]interface{}) string {
	text := func(key string) string {
		value, _ := fields[key].(string)
		return value
	}
	switch {
	case text("bucket") != "" && text("key") != "":
		return "s3://" + text("bucket") + "/" + text("key")
	case text("path") != "" && text("table") != "":
		return text("path") + "#" + text("table")
	case text("path") != "":
		return text("path")
	case text("url") != "":
		return redactURL(text("url"))
	}
	return ""
}

// databaseLocation names a database table without its connection string, which
// can hold credentials.
func databaseLocation(fields map[string]interface{}) string {
	table, _ := fields["table"].(string)
	if dbname, _ := fields["dbname"].(string); dbname != "" {
		return "postgres:" + dbname + "." + table
	}
	return "postgres:" + table
}

// redactURL drops a URL's user information and query, either of which can
// carry a credential.
func redactURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parsed.User, parsed.RawQuery, parsed.Fragment = nil, "", ""
	return parsed.String()
}

func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}
