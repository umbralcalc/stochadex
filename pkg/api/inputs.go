package api

import (
	"fmt"
	"sort"
	"strings"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// The inputs: tier (PLAN.md Phase 1). An input is a named, read-only storage
// that exists before the runtime is built: loaded from a source (a file, a
// database), or produced by a labelled pre-pass simulation. A main: partition
// replays one of an input's partitions with {type: from_input}; the run's clock
// can follow an input's timestamps with timestep_function {type: from_input}
// and stop when it runs out with termination_condition {type: input_exhausted}.
//
// Inputs are read when a run starts, never when a config is loaded: loading
// resolves those three specs to unbound placeholders and validates the names
// they refer to, and bindInputs loads the inputs and binds the placeholders
// immediately before the run's generator is built. A missing input is therefore
// ErrUnavailable and a partition the input lacks is ErrData, both at run time.

// InputConfig is one entry of inputs:. Exactly one of Source, Simulation and
// Stream is set.
type InputConfig struct {
	// Source loads the input from a file or database (the data.source spellings).
	Source *DataSource `yaml:"source,omitempty"`
	// Simulation produces the input by running a sub-simulation first.
	Simulation *DataConfig `yaml:"simulation,omitempty"`
	// Stream reads the input live, from outside the run, while it runs (see
	// streams.go). Only params_from_input reads a stream input.
	Stream *StreamConfig `yaml:"stream,omitempty"`
	// Decode is how a stream's messages are read: "json" (the default), each
	// message a json_log entry or a JSON array of them; or
	// "protobuf_action_state", each message a simulator.ActionState.
	Decode string `yaml:"decode,omitempty"`
	// OnEmpty is what a stream input gives a step that no new message has
	// reached: "hold_last" (the default, and for now the only choice) holds the
	// last value.
	OnEmpty string `yaml:"on_empty,omitempty"`
	// Record writes the values a stream input gave each step to this json_log
	// path, so the run can be replayed from it as a source input. Under
	// run: {mode: serve} it must contain {connection}, as output views must.
	Record string `yaml:"record,omitempty"`
}

// StreamConfig is a stream input's transport. Exactly one field is set.
type StreamConfig struct {
	// Websocket connects to a websocket server as a client and reads its
	// messages.
	Websocket *WebsocketStreamConfig `yaml:"websocket,omitempty"`
	// Connection reads the messages of the client a run is served to, under
	// run: {mode: serve}: the connection its {type: connection} view streams
	// to, read in the other direction.
	Connection *ConnectionStreamConfig `yaml:"connection,omitempty"`
}

// ConnectionStreamConfig selects a served client as a stream. It has no fields:
// spell it connection: {}.
type ConnectionStreamConfig struct{}

// WebsocketStreamConfig is a websocket stream's server address.
type WebsocketStreamConfig struct {
	URL string `yaml:"url"`
}

// load reads a stored input. Validation keeps stream inputs from reaching it.
func (i *InputConfig) load() (*simulator.StateTimeStorage, error) {
	if i.Source != nil {
		storage, err := i.Source.load()
		return storage, inputError(err)
	}
	return i.Simulation.buildStorage()
}

// unboundInputIteration is what {type: from_input} resolves to at load: a
// placeholder bindInputs replaces with the input's rows when a run starts.
type unboundInputIteration struct {
	input     string
	partition string
}

func (u *unboundInputIteration) Configure(int, *simulator.Settings) {
	panic(fmt.Sprintf("api: a from_input partition (input %q) was run unbound; "+
		"run the config through api.Run, RunWith or RunToStorage, which bind "+
		"inputs when the run starts", u.input))
}

func (u *unboundInputIteration) Iterate(
	*simulator.Params, int, []*simulator.StateHistory, *simulator.CumulativeTimestepsHistory,
) []float64 {
	panic("api: unbound from_input partition")
}

// unboundInputTimesteps is timestep_function {type: from_input} before binding.
type unboundInputTimesteps struct{ input string }

func (u *unboundInputTimesteps) NextIncrement(*simulator.CumulativeTimestepsHistory) float64 {
	panic(fmt.Sprintf("api: timestep_function from_input (input %q) was run unbound", u.input))
}

// unboundInputExhausted is termination_condition {type: input_exhausted} before
// binding.
type unboundInputExhausted struct{ input string }

func (u *unboundInputExhausted) Terminate(
	[]*simulator.StateHistory, *simulator.CumulativeTimestepsHistory,
) bool {
	panic(fmt.Sprintf("api: termination_condition input_exhausted (input %q) was run unbound", u.input))
}

// fromStoragePrefix names the inputs that inline from_storage data becomes.
const fromStoragePrefix = "from_storage/"

// desugarFromStorage makes inline data an input (PLAN.md rule 14, IO.4): a main
// partition whose iteration is {type: from_storage, data: rows} becomes a
// from_input partition reading an inline input of those rows, and a
// timestep_function {type: from_storage, data: times} a from_input clock
// reading an inline input of those times. Both bind to the same replay they
// always did. One that also sets init_steps_taken, which from_input has no
// equivalent of, is left as written; so is one whose data does not parse, so
// that its own builder reports it.
func desugarFromStorage(config *ApiRunConfig) error {
	add := func(name string, source *InlineSource) error {
		if _, exists := config.Inputs[name]; exists {
			return fmt.Errorf("api: input %q is the name inline from_storage data "+
				"takes; rename the input", name)
		}
		if config.Inputs == nil {
			config.Inputs = map[string]InputConfig{}
		}
		config.Inputs[name] = InputConfig{Source: &DataSource{Inline: source}}
		return nil
	}
	for index := range config.Main.Partitions {
		partition := &config.Main.Partitions[index]
		spec := partition.IterationSpec
		if spec.Type != "from_storage" || len(spec.Fields) != 1 {
			continue
		}
		rows, err := floatMatrix("from_storage", "data", spec.Fields["data"])
		if err != nil || len(rows) == 0 {
			continue
		}
		times := make([]float64, len(rows))
		for i := range times {
			times[i] = float64(i)
		}
		name := fromStoragePrefix + partition.Name
		if err := add(name, &InlineSource{Times: times,
			Partitions: map[string][][]float64{partition.Name: rows}}); err != nil {
			return err
		}
		partition.IterationSpec = simulator.ComponentSpec{Type: "from_input",
			Fields: map[string]interface{}{"input": name}}
	}
	clock := &config.Main.SimulationStrings.TimestepFunction
	if clock.Type == "from_storage" && len(clock.Fields) == 1 {
		if times, err := floatRow("from_storage", "data", clock.Fields["data"]); err == nil && len(times) > 0 {
			name := fromStoragePrefix + "timestep_function"
			if err := add(name, &InlineSource{Times: times}); err != nil {
				return err
			}
			*clock = simulator.ComponentSpec{Type: "from_input",
				Fields: map[string]interface{}{"input": name}}
		}
	}
	return nil
}

func buildFromInput(fields map[string]interface{}) (simulator.Iteration, error) {
	iteration := &unboundInputIteration{}
	for key, value := range fields {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("from_input: %s must be a string, got %T", key, value)
		}
		switch key {
		case "input":
			iteration.input = text
		case "partition":
			iteration.partition = text
		default:
			return nil, fmt.Errorf("from_input: unknown field %q", key)
		}
	}
	if iteration.input == "" {
		return nil, fmt.Errorf("from_input: needs input: naming an entry of inputs:")
	}
	return iteration, nil
}

// inputNameField reads the single required "input" field of the from_input
// timestep function and the input_exhausted termination condition.
func inputNameField(kind string, spec simulator.ComponentSpec) (string, error) {
	for key := range spec.Fields {
		if key != "input" {
			return "", fmt.Errorf("%s: unknown field %q", kind, key)
		}
	}
	name, ok := spec.Fields["input"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("%s: needs input: naming an entry of inputs:", kind)
	}
	return name, nil
}

func registerInputComponents() {
	iterationBuilders["from_input"] = buildFromInput
	simulator.RegisterComponent("timestep_function", "from_input",
		func(spec simulator.ComponentSpec) (interface{}, error) {
			name, err := inputNameField("timestep_function from_input", spec)
			return &unboundInputTimesteps{input: name}, err
		})
	simulator.RegisterComponent("termination_condition", "input_exhausted",
		func(spec simulator.ComponentSpec) (interface{}, error) {
			name, err := inputNameField("termination_condition input_exhausted", spec)
			return &unboundInputExhausted{input: name}, err
		})
}

func init() {
	registerInputComponents()
}

// validateInputs checks, at load, that the inputs: block and every reference
// to it are coherent — without reading any input.
func validateInputs(config *ApiRunConfig) error {
	streams, connections := false, 0
	serving := config.Run.Mode == "serve"
	for name, input := range config.Inputs {
		set := 0
		for _, present := range []bool{input.Source != nil, input.Simulation != nil, input.Stream != nil} {
			if present {
				set++
			}
		}
		switch {
		case set != 1:
			return fmt.Errorf("api: input %q needs exactly one of source:, simulation: or stream:", name)
		case input.Simulation != nil && input.Simulation.Source != nil:
			return fmt.Errorf("api: input %q: a simulation: input cannot also set source:", name)
		}
		if input.Source != nil && input.Source.Inline != nil {
			if _, err := input.Source.Inline.storage(); err != nil {
				return fmt.Errorf("api: input %q: %w", name, err)
			}
		}
		if err := validateStreamInput(name, input); err != nil {
			return err
		}
		streams = streams || input.Stream != nil
		if input.Stream != nil && input.Stream.Connection != nil {
			if !serving {
				return fmt.Errorf("api: input %q reads a served client (stream: "+
					"{connection: {}}), which only applies to run: {mode: serve}", name)
			}
			connections++
		}
		switch perConnection := strings.Contains(input.Record, "{connection}"); {
		case serving && input.Record != "" && !perConnection:
			return fmt.Errorf("api: input %q's record: would have every served connection "+
				"write to the same file; put {connection} in it, e.g. record: feed-{connection}.log", name)
		case !serving && perConnection:
			return fmt.Errorf("api: input %q's record: uses {connection}, which only "+
				"applies to run: {mode: serve}", name)
		}
	}
	if connections > 1 {
		return fmt.Errorf("api: %d inputs read the served client; a run is served one "+
			"connection, so declare one and bind each partition to its partitions", connections)
	}
	if streams && len(config.Macros) > 0 {
		return fmt.Errorf("api: macros read their inputs' stored rows, which a stream " +
			"input does not have; record the stream and use the record as a source input")
	}
	if config.Run.Mode == "ensemble" {
		feeds := streams
		for _, partition := range config.Main.Partitions {
			feeds = feeds || len(partition.ParamsFromInput) > 0
		}
		if feeds {
			return fmt.Errorf("api: params_from_input and stream inputs do not yet apply " +
				"to run: {mode: ensemble}")
		}
	}
	if len(config.Inputs) > 0 && config.Data != nil {
		return fmt.Errorf("api: a config sets both data: and inputs:; data: is " +
			"shorthand for a single input — move it into inputs:")
	}
	used := map[string]bool{}
	reference := func(where, input string) error {
		if _, ok := config.Inputs[input]; !ok {
			return fmt.Errorf("api: %s names input %q, which inputs: does not declare", where, input)
		}
		used[input] = true
		return nil
	}
	// storedOnly is reference for readers of an input's stored rows, which a
	// stream input does not have.
	storedOnly := func(where, input string) error {
		if err := reference(where, input); err != nil {
			return err
		}
		if config.Inputs[input].Stream != nil {
			return fmt.Errorf("api: %s reads input %q's stored rows, but it is a "+
				"stream input; set params from it with params_from_input instead", where, input)
		}
		return nil
	}
	for _, partition := range config.Main.Partitions {
		if unbound, ok := partition.Iteration.(*unboundInputIteration); ok {
			if err := storedOnly(fmt.Sprintf("partition %q", partition.Name), unbound.input); err != nil {
				return err
			}
		}
	}
	if err := validateParamsFromInput(config, reference); err != nil {
		return err
	}
	for _, embedded := range config.Embedded {
		for _, partition := range embedded.Run.Partitions {
			if _, ok := partition.Iteration.(*unboundInputIteration); ok {
				return fmt.Errorf("api: from_input is not yet supported inside embedded "+
					"runs (partition %q of %q)", partition.Name, embedded.Name)
			}
			if len(partition.ParamsFromInput) > 0 {
				return fmt.Errorf("api: params_from_input is not yet supported inside "+
					"embedded runs (partition %q of %q)", partition.Name, embedded.Name)
			}
		}
	}
	if timesteps, ok := config.Main.Simulation.TimestepFunction.(*unboundInputTimesteps); ok {
		if err := storedOnly("timestep_function from_input", timesteps.input); err != nil {
			return err
		}
	}
	if exhausted, ok := config.Main.Simulation.TerminationCondition.(*unboundInputExhausted); ok {
		if err := storedOnly("termination_condition input_exhausted", exhausted.input); err != nil {
			return err
		}
	}
	// A macros: config analyses every input (they are combined into the storage
	// its macros read), so only main: inputs can go unused.
	for name := range config.Inputs {
		if len(config.Macros) == 0 && !used[name] {
			return fmt.Errorf("api: input %q is never used; replay it with "+
				"{type: from_input, input: %s}, set params from it with "+
				"params_from_input, or remove it", name, name)
		}
	}
	return nil
}

// validateStreamInput checks a stream input's transport and options, and that
// the stream options are not set on any other kind of input.
func validateStreamInput(name string, input InputConfig) error {
	if input.Stream == nil {
		if input.Decode != "" || input.OnEmpty != "" || input.Record != "" {
			return fmt.Errorf("api: input %q: decode:, on_empty: and record: only "+
				"apply to stream: inputs", name)
		}
		return nil
	}
	websocketClient := input.Stream.Websocket != nil
	if websocketClient == (input.Stream.Connection != nil) ||
		websocketClient && input.Stream.Websocket.URL == "" {
		return fmt.Errorf("api: input %q: stream: needs one transport, e.g. "+
			"stream: {websocket: {url: \"ws://localhost:9000/feed\"}}, or "+
			"stream: {connection: {}} for a served client", name)
	}
	switch input.Decode {
	case "", "json", "protobuf_action_state":
	default:
		return fmt.Errorf("api: input %q: unknown decode %q (expected json or "+
			"protobuf_action_state)", name, input.Decode)
	}
	if input.OnEmpty != "" && input.OnEmpty != "hold_last" {
		return fmt.Errorf("api: input %q: on_empty %q is not supported yet; a stream "+
			"input holds its last value between messages (hold_last)", name, input.OnEmpty)
	}
	return nil
}

// validateParamsFromInput checks every main partition's params_from_input:
// each names a declared input (via reference), sets a key the partition
// declares in its params (whose value it has until the input gives it one),
// and is not also set by params_from_upstream. Partitions set from the same
// stream partition must declare the same initial value, since a recording of
// the stream holds one value for it per step.
func validateParamsFromInput(config *ApiRunConfig, reference func(where, input string) error) error {
	initial := map[string][]float64{}
	initialFrom := map[string]string{}
	for _, partition := range config.Main.Partitions {
		keys := make([]string, 0, len(partition.ParamsFromInput))
		for key := range partition.ParamsFromInput {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			binding := partition.ParamsFromInput[key]
			where := fmt.Sprintf("partition %q params_from_input %q", partition.Name, key)
			if err := reference(where, binding.Input); err != nil {
				return err
			}
			values, declared := partition.Params.Map[key]
			if !declared || len(values) == 0 {
				return fmt.Errorf("api: %s: declare %q in the partition's params with "+
					"the value it has until the input sets it", where, key)
			}
			if _, upstream := partition.ParamsFromUpstream[key]; upstream {
				return fmt.Errorf("api: %s: %q is also set by params_from_upstream", where, key)
			}
			if config.Inputs[binding.Input].Stream == nil {
				continue
			}
			source := binding.Input + "/" + sourcePartition(binding, key)
			if previous, seen := initial[source]; seen && !floatsEqual(previous, values) {
				return fmt.Errorf("api: %s and %s are both set from stream %s but declare "+
					"different initial values (%v vs %v); a recording holds one value for it",
					initialFrom[source], where, source, previous, values)
			}
			initial[source], initialFrom[source] = values, where
		}
	}
	return nil
}

// sourcePartition is the input partition a params_from_input binding reads:
// the one it names, or the params key's own name.
func sourcePartition(binding simulator.InputParamConfig, key string) string {
	if binding.Partition != "" {
		return binding.Partition
	}
	return key
}

func floatsEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// bindInputs loads the inputs a config's main run uses and binds its
// placeholders: each from_input partition to its input partition's rows (its
// init_state_values defaulting to the first row), a from_input timestep function
// to the input's times, and input_exhausted to the input's length. It runs when
// a run starts; a config with nothing to bind is left alone, and binding twice
// is a no-op. Failures are classified: a missing input is ErrUnavailable, a
// partition the input lacks or a width mismatch is ErrData.
func bindInputs(config *ApiRunConfig) error {
	return bindInputsWith(config, &inputLoader{config: config})
}

// inputLoader loads a config's inputs on first use, each at most once per run.
// Its map is made on the first load, so a run with no inputs allocates nothing.
type inputLoader struct {
	config   *ApiRunConfig
	storages map[string]*simulator.StateTimeStorage
}

func (l *inputLoader) storage(name string) (*simulator.StateTimeStorage, error) {
	if storage, ok := l.storages[name]; ok {
		return storage, nil
	}
	input := l.config.Inputs[name]
	storage, err := input.load()
	if err != nil {
		return nil, fmt.Errorf("api: loading input %q: %w", name, err)
	}
	if l.storages == nil {
		l.storages = map[string]*simulator.StateTimeStorage{}
	}
	l.storages[name] = storage
	return storage, nil
}

// bindInputsWith is bindInputs, loading through loader, so the run's
// params_from_input feeds share the inputs it loaded.
func bindInputsWith(config *ApiRunConfig, loader *inputLoader) error {
	return bindPlaceholders(config, loader.storage)
}

// bindPlaceholders binds a config's from_input placeholders, loading inputs
// through storageOf.
func bindPlaceholders(
	config *ApiRunConfig,
	storageOf func(name string) (*simulator.StateTimeStorage, error),
) error {
	for index := range config.Main.Partitions {
		partition := &config.Main.Partitions[index]
		unbound, ok := partition.Iteration.(*unboundInputIteration)
		if !ok {
			continue
		}
		storage, err := storageOf(unbound.input)
		if err != nil {
			return err
		}
		source := unbound.partition
		if source == "" {
			source = partition.Name
		}
		rows, err := inputRows(storage, unbound.input, source)
		if err != nil {
			return err
		}
		switch {
		case len(partition.InitStateValues) == 0:
			partition.InitStateValues = append([]float64(nil), rows[0]...)
		case len(partition.InitStateValues) != len(rows[0]):
			return withKind(ErrData, fmt.Errorf(
				"api: partition %q has %d init_state_values but input %q's %q rows have %d",
				partition.Name, len(partition.InitStateValues), unbound.input, source, len(rows[0])))
		}
		partition.Iteration = &general.FromStorageIteration{Data: rows}
	}
	if unbound, ok := config.Main.Simulation.TimestepFunction.(*unboundInputTimesteps); ok {
		storage, err := storageOf(unbound.input)
		if err != nil {
			return err
		}
		config.Main.Simulation.TimestepFunction = &general.FromStorageTimestepFunction{
			Data: storage.GetTimes()}
	}
	if unbound, ok := config.Main.Simulation.TerminationCondition.(*unboundInputExhausted); ok {
		storage, err := storageOf(unbound.input)
		if err != nil {
			return err
		}
		config.Main.Simulation.TerminationCondition = &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: len(storage.GetTimes()) - 1}
	}
	return nil
}

// inputRows returns an input partition's rows, or ErrData naming what the input
// does contain.
func inputRows(storage *simulator.StateTimeStorage, input, partition string) ([][]float64, error) {
	names := storage.GetNames()
	sort.Strings(names)
	for _, name := range names {
		if name == partition {
			rows := storage.GetValues(name)
			if len(rows) == 0 {
				return nil, withKind(ErrData, fmt.Errorf(
					"api: input %q partition %q has no rows", input, partition))
			}
			return rows, nil
		}
	}
	return nil, withKind(ErrData, fmt.Errorf(
		"api: input %q has no partition %q (it has: %s)", input, partition, strings.Join(names, ", ")))
}

// macroInputs returns the inputs a macros: config analyses: its inputs:, or its
// data: block desugared to a single input named "data" (a data: source is a
// source input; a data: sub-simulation is a simulation input).
func macroInputs(config *ApiRunConfig) map[string]InputConfig {
	switch {
	case len(config.Inputs) > 0:
		return config.Inputs
	case config.Data == nil:
		return nil
	case config.Data.Source != nil:
		return map[string]InputConfig{"data": {Source: config.Data.Source}}
	default:
		return map[string]InputConfig{"data": {Simulation: config.Data}}
	}
}

// loadMacroStorage loads a macros: config's inputs into the one storage its
// macros analyse. A single input is used as it is. Several are combined: they
// must share one time axis (the storage has one), and a partition name may come
// from only one input; either mismatch is ErrData naming the inputs involved.
func loadMacroStorage(inputs map[string]InputConfig) (*simulator.StateTimeStorage, error) {
	names := make([]string, 0, len(inputs))
	for name := range inputs {
		names = append(names, name)
	}
	sort.Strings(names)
	load := func(name string) (*simulator.StateTimeStorage, error) {
		input := inputs[name]
		storage, err := input.load()
		if err != nil {
			return nil, fmt.Errorf("api: loading input %q: %w", name, err)
		}
		return storage, nil
	}
	if len(names) == 1 {
		return load(names[0])
	}
	merged := simulator.NewStateTimeStorage()
	owners := map[string]string{}
	var times []float64
	for _, name := range names {
		storage, err := load(name)
		if err != nil {
			return nil, err
		}
		if times == nil {
			times = storage.GetTimes()
		} else if err := sameTimes(names[0], times, name, storage.GetTimes()); err != nil {
			return nil, err
		}
		for _, partition := range storage.GetNames() {
			if owner, taken := owners[partition]; taken {
				return nil, withKind(ErrData, fmt.Errorf(
					"api: partition %q is in both input %q and input %q", partition, owner, name))
			}
			owners[partition] = name
			merged.SetValues(partition, storage.GetValues(partition))
		}
	}
	merged.SetTimes(times)
	return merged, nil
}

// sameTimes is ErrData unless two inputs' time axes are identical.
func sameTimes(firstName string, first []float64, otherName string, other []float64) error {
	if len(first) != len(other) {
		return withKind(ErrData, fmt.Errorf(
			"api: inputs %q and %q have different time axes (%d vs %d rows); the "+
				"macros analyse one storage, so every input must share one",
			firstName, otherName, len(first), len(other)))
	}
	for i := range first {
		if first[i] != other[i] {
			return withKind(ErrData, fmt.Errorf(
				"api: inputs %q and %q have different time axes (row %d: %v vs %v); the "+
					"macros analyse one storage, so every input must share one",
				firstName, otherName, i, first[i], other[i]))
		}
	}
	return nil
}
