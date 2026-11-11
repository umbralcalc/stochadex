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
	"gopkg.in/yaml.v2"
)

// Inspecting a config without running it: Check validates it, and
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
// needs its inputs' shapes, so for one Check adds nothing to what loading
// checked.
//
// Check builds fresh instances to check, so config is left as it was.
// Failures are ErrConfig.
func Check(config *ApiRunConfig) error {
	if len(config.Macros) > 0 {
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
		checked.Embedded = append([]EmbeddedRunConfig(nil), config.Embedded...)
		for index := range checked.Embedded {
			checked.Embedded[index].Run.Partitions = append([]simulator.PartitionConfig(nil),
				config.Embedded[index].Run.Partitions...)
		}
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
// from it, and as wide as a partition that starts from all of it with
// init_state_from_upstream, so only the run, reading the input, checks those.
func standInInputs(config *ApiRunConfig) {
	needed := map[string]int{}
	for _, partition := range config.Main.Partitions {
		for key, upstream := range partition.ParamsFromUpstream {
			for _, index := range upstream.Indices {
				if index+1 > needed[upstream.Upstream] {
					needed[upstream.Upstream] = index + 1
				}
			}
			if key == partition.InitStateFromUpstream && len(upstream.Indices) == 0 {
				needed[upstream.Upstream] = max(needed[upstream.Upstream], len(partition.InitStateValues))
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
	// A partition that reads a file input, such as a model, reads it when it is
	// configured, so a check configures a stand-in instead.
	standIn := func(scope string, partitions []simulator.PartitionConfig) {
		for index := range partitions {
			if config.readsFiles[scope+"/"+partitions[index].Name] {
				partitions[index].Iteration = &inputStandIn{}
			}
		}
	}
	standIn("main", config.Main.Partitions)
	for _, embedded := range config.Embedded {
		standIn(embedded.Name, embedded.Run.Partitions)
	}
}

// inputStandIn stands in, while a config is checked, for a partition that
// would read an input: a from_input partition, or one reading a file input.
type inputStandIn struct{}

func (*inputStandIn) Configure(int, *simulator.Settings) {}

func (*inputStandIn) Iterate(
	*simulator.Params, int, []*simulator.StateHistory, *simulator.CumulativeTimestepsHistory,
) []float64 {
	panic("api: a checked config was stepped")
}

// IOManifest describes what a run of a config reads and writes, without
// running it. Its JSON form is what stochadex inspect --io prints.
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
	// Outputs are every place the run writes its results: the outputs: views
	// (including the default view) and each stream input's record file. Debug logs are listed apart, in Debug.
	Outputs []ManifestOutput `json:"outputs"`
	// Embedded describes each embedded run as the outer run sees it: a black
	// box whose host partition's row, each outer step, is its inner partitions'
	// final states concatenated in Columns' order. It lists the runs those runs
	// host too, each after its host, with what each costs the run in inner steps.
	Embedded []ManifestEmbedded `json:"embedded,omitempty"`
	// Debug lists the logs a run writes only because its config asks for an
	// embedded run's inner detail: every inner step of every outer step. They
	// are for debugging, not results, so provenance and
	// --skip-if-unchanged ignore them.
	Debug []ManifestOutput `json:"debug,omitempty"`
}

// ManifestEmbedded is an embedded run's host partition, its column layout, and
// what it costs: how many inner steps it takes each time its host steps, and so
// how many it takes at each step of the run.
type ManifestEmbedded struct {
	// Partition is the host partition, by its path from the main run: a run
	// hosted inside another run is named "<outer host>/<host>".
	Partition string `json:"partition"`
	// Depth is how many runs deep it sits: 1 for a run the main run hosts.
	Depth int `json:"depth"`
	// Termination is the inner run's termination condition type.
	Termination string `json:"termination"`
	// Steps is how many inner steps the run takes each time its host steps (a
	// whole run from its start, past any burn_in_steps), or null when its
	// termination condition is not a number of steps.
	Steps *int `json:"steps"`
	// Replicas is how many copies of the run a replicated_simulation_run
	// runs each time it steps; the run's columns are replica 0's, and replica
	// r's are offset r times their width.
	Replicas int `json:"replicas,omitempty"`
	// StepsPerRunStep is how many inner steps the run takes at each step of
	// the main run: Steps, times Replicas, times the same for each run it sits
	// in. It is null when any of those runs has no fixed number of steps.
	StepsPerRunStep *int              `json:"steps_per_run_step"`
	Columns         []ManifestColumns `json:"columns"`
}

// ManifestColumns are the columns of a host partition's row that hold one
// inner partition's final state: [Offset, Offset+Width).
type ManifestColumns struct {
	Partition string `json:"partition"`
	Offset    int    `json:"offset"`
	Width     int    `json:"width"`
}

// ManifestRun is how many times a config's simulation runs, and how.
type ManifestRun struct {
	// Mode is batch, ensemble or serve.
	Mode string `json:"mode"`
	// Runtime is main, or macros for a macros: config, which runs as its
	// expansion (Expand): one simulation of the partitions its macros expand
	// into.
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
	// DeclaredIn is where the config declares it: outputs, default (no output
	// declared), an embedded run's simulation, or a stream input's record.
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
	inputs := runInputs(config)
	for _, name := range sortedKeys(inputs) {
		input := inputs[name]
		entry := ManifestInput{Name: name, ReadBy: readers[name].by,
			Partitions: readers[name].partitions}
		switch {
		case input.File != nil:
			entry.Kind, entry.Location = "file", input.File.Path
			entry.ReadBy = config.fileReaders[name]
		case input.Stream != nil:
			entry.Kind = "stream"
			if transport, _, ok := input.Stream.registeredTransport(); ok {
				entry.Location = transport
			} else if input.Stream.Websocket != nil {
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
		if len(config.Macros) > 0 && input.File == nil {
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
		if view.isDefault {
			declared = "default"
		}
		location := sinkLocation(view.Function)
		manifest.Outputs = append(manifest.Outputs, ManifestOutput{
			Name: view.Name, DeclaredIn: declared, Condition: conditionName(view.Condition),
			Sink: view.Function.Type, Location: location,
			Locations: memberLocations(config, location),
		})
	}
	manifest.Embedded = hostedRuns(config.Main, config.Embedded, "", 1, intPointer(1), config.scope)
	for _, embedded := range config.Embedded {
		function := embedded.Run.SimulationStrings.OutputFunction
		if function.IsZero() || function.Type == "nil" {
			continue
		}
		manifest.Debug = append(manifest.Debug, ManifestOutput{
			Name:       embedded.Name,
			DeclaredIn: "embedded[name=" + embedded.Name + "].simulation",
			Condition:  conditionName(embedded.Run.SimulationStrings.OutputCondition),
			Sink:       function.Type, Location: sinkLocation(function),
		})
	}
	return manifest
}

// hostedRuns describes the runs that run's partitions host, at depth, each
// followed by the runs it hosts: an embedded_simulation_run's own run, a
// replicated_simulation_run's run, and a top-level embedded: run named like its
// host. prefix is the path of run's own host, and perStep how many steps run
// takes at each step of the main run, or nil when that is not fixed.
func hostedRuns(
	run RunConfig,
	embedded []EmbeddedRunConfig,
	prefix string,
	depth int,
	perStep *int,
	scope *runScope,
) []ManifestEmbedded {
	byHost := map[string]RunConfig{}
	for _, inner := range embedded {
		byHost[inner.Name] = inner.Run
	}
	var hosted []ManifestEmbedded
	for _, partition := range run.Partitions {
		var inner RunConfig
		var innerEmbedded []EmbeddedRunConfig
		replicas, output := 0, ""
		if nested, ok := nestedRunOf(partition, scope); ok {
			inner, innerEmbedded = nested.Main, nested.Embedded
		} else if spec, ok := replicatedRunOf(partition); ok {
			inner, replicas, output = spec.Run, spec.Replicas, spec.OutputPartition
		} else if run, ok := byHost[partition.Name]; ok {
			inner = run
		} else {
			continue
		}
		entry := ManifestEmbedded{
			Partition:   prefix + partition.Name,
			Depth:       depth,
			Termination: inner.SimulationStrings.TerminationCondition.Type,
			Steps:       fixedSteps(inner.SimulationStrings.TerminationCondition),
			Replicas:    replicas,
			Columns:     columnsOf(inner, output),
		}
		if perStep != nil && entry.Steps != nil {
			entry.StepsPerRunStep = intPointer(*perStep * *entry.Steps * max(replicas, 1))
		}
		hosted = append(hosted, entry)
		hosted = append(hosted, hostedRuns(inner, innerEmbedded, entry.Partition+"/",
			depth+1, entry.StepsPerRunStep, scope)...)
	}
	return hosted
}

// replicatedRunOf decodes a replicated_simulation_run partition's spec, enough
// to read its replicas and its run's partitions and simulation block.
func replicatedRunOf(partition simulator.PartitionConfig) (replicatedRunSpec, bool) {
	var spec replicatedRunSpec
	if partition.IterationSpec.Type != "replicated_simulation_run" {
		return spec, false
	}
	// Fields decoded from YAML always marshal back, and loading decoded them.
	raw, _ := yaml.Marshal(partition.IterationSpec.Fields)
	_ = yaml.Unmarshal(raw, &spec)
	return spec, true
}

// fixedSteps is how many steps a termination condition runs for, when it is a
// number of steps. Loading has checked max_steps is an integer.
func fixedSteps(termination simulator.ComponentSpec) *int {
	if steps, ok := termination.Fields["max_steps"].(int); ok && termination.Type == "number_of_steps" {
		return intPointer(steps)
	}
	return nil
}

func intPointer(value int) *int {
	return &value
}

// columnsOf is where each inner partition's final state sits in its host
// partition's row: in the inner run's partition order, each as wide as its
// state. A replicated run's row holds only its output partition's, so only
// output is listed when it is set.
func columnsOf(run RunConfig, output string) []ManifestColumns {
	columns := []ManifestColumns{}
	offset := 0
	for _, partition := range run.Partitions {
		if output != "" && partition.Name != output {
			continue
		}
		width := len(partition.InitStateValues)
		columns = append(columns, ManifestColumns{
			Partition: partition.Name, Offset: offset, Width: width})
		offset += width
	}
	return columns
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

// runInputs are every input a run reads: its inputs:, or a macros: config's
// data: as an input.
func runInputs(config *ApiRunConfig) map[string]InputConfig {
	if len(config.Inputs) > 0 || len(config.Macros) == 0 {
		return config.Inputs
	}
	return macroInputs(config)
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
	case source.Inline != nil:
		return "inline", ""
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
