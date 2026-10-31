package api

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	yaml3 "gopkg.in/yaml.v3"
)

// Macros as config → config shortcuts. A macro expands into
// core-language partitions — data specs only, as a hand-written config would
// state them — so a macros: config becomes one ordinary run:
//
//	inputs:  the config's inputs, unchanged
//	main:    a {type: from_input} replay of every input partition, then each
//	         macro's partitions, in order, clocked by the inputs' timestamps
//	outputs: the config's outputs, unchanged
//
// A first macro with a clock of its own (evolution strategy, MCTS, SMC) clocks
// the run instead, its steps of its timestep from time zero; the run then
// replays no input, which only such a macro's own runs may read (SMC's
// particles replay their observed data from an input, on its clock).
//
// An expansion sees only config: the partitions it may read and their widths
// (an expansionContext), never stored values. A later macro reads an earlier
// one's partitions live, in the same run, instead of a replay of its recorded
// output, and the history a macro reads over (its window) becomes the history
// depth of the partition it reads.

// expansionContext is what an expansion may know: the partitions visible to
// it — the inputs', then those of the macros before it — their widths, and
// their first rows (the state each starts in, which a window replaying it
// starts from too).
type expansionContext struct {
	widths    map[string]int
	firstRows map[string][]float64
	// inputs are the visible partitions an input supplies, and stored loads
	// the inputs' rows, for the one expansion that reads them: discovering a
	// grouped_aggregation's groups when the config declares none.
	inputs map[string]bool
	stored func() (*simulator.StateTimeStorage, error)
	// source is the macro being expanded, as the config writes it.
	source *yaml3.Node
	// owners names the input each input partition comes from, and initTime
	// is the time the inputs start at.
	owners   map[string]string
	initTime float64
	// inputSteps is the number of steps a replay of the inputs runs for, when
	// the run is clocked by them and that is known (0 otherwise).
	inputSteps int
}

// firstRow is a visible partition's first row.
func (c *expansionContext) firstRow(partition string) ([]float64, error) {
	if _, err := c.width(partition); err != nil {
		return nil, err
	}
	return c.firstRows[partition], nil
}

// initialState is the state an expanded partition starts the run from: its
// init_state_values, or the part of its upstream's initial state that
// init_state_from_upstream names.
func (c *expansionContext) initialState(partition expandedPartition) []float64 {
	if partition.InitStateFromUp == "" {
		return partition.InitStateValues
	}
	upstream := partition.ParamsFromUpstream[partition.InitStateFromUp]
	first := c.firstRows[upstream.Upstream]
	state := make([]float64, len(upstream.Indices))
	for i, index := range upstream.Indices {
		state[i] = first[index]
	}
	return state
}

// width is a visible partition's state width.
func (c *expansionContext) width(partition string) (int, error) {
	width, ok := c.widths[partition]
	if !ok {
		names := make([]string, 0, len(c.widths))
		for name := range c.widths {
			names = append(names, name)
		}
		sort.Strings(names)
		return 0, fmt.Errorf("no partition %q to read (the inputs and earlier macros have: %v)",
			partition, names)
	}
	return width, nil
}

// indices are the state indices a data reference reads: those it names, or
// all of its partition's.
func (c *expansionContext) indices(ref dataRefSpec) ([]int, error) {
	width, err := c.width(ref.PartitionName)
	if err != nil {
		return nil, err
	}
	if len(ref.ValueIndices) == 0 {
		indices := make([]int, width)
		for i := range indices {
			indices[i] = i
		}
		return indices, nil
	}
	for _, index := range ref.ValueIndices {
		if index < 0 || index >= width {
			return nil, fmt.Errorf("value index %d is outside partition %q's %d values",
				index, ref.PartitionName, width)
		}
	}
	return ref.ValueIndices, nil
}

// expansion is what one macro contributes to the run.
type expansion struct {
	partitions []expandedPartition
	// depths is the history each partition the macro reads over needs: its
	// window.
	depths map[string]int
	// clock is the run's clock, for a macro that runs its own.
	clock *expandedClock
}

// expandedPartition is one partition of an expansion, as data.
type expandedPartition struct {
	Name               string                      `yaml:"name"`
	Iteration          *yaml3.Node                 `yaml:"iteration"`
	Params             map[string][]float64        `yaml:"params,omitempty,flow"`
	ParamsAsPartitions map[string][]string         `yaml:"params_as_partitions,omitempty,flow"`
	ParamsFromUpstream map[string]expandedUpstream `yaml:"params_from_upstream,omitempty"`
	InitStateValues    []float64                   `yaml:"init_state_values,omitempty,flow"`
	InitStateFromUp    string                      `yaml:"init_state_from_upstream,omitempty"`
	StateHistoryDepth  int                         `yaml:"state_history_depth"`
	Seed               uint64                      `yaml:"seed"`
}

type expandedUpstream struct {
	Upstream string `yaml:"upstream"`
	Indices  []int  `yaml:"indices,omitempty,flow"`
}

// component is a {type: ...} spec as YAML: its type, then its fields in name
// order.
func component(typeName string, fields map[string]interface{}) *yaml3.Node {
	node := &yaml3.Node{Kind: yaml3.MappingNode, Style: yaml3.FlowStyle}
	add := func(key string, value interface{}) {
		valueNode := &yaml3.Node{}
		// Every value here came from YAML, so it encodes back.
		_ = valueNode.Encode(value)
		node.Content = append(node.Content,
			&yaml3.Node{Kind: yaml3.ScalarNode, Value: key}, valueNode)
	}
	add("type", typeName)
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		add(key, fields[key])
	}
	return node
}

// componentOf is a decoded spec as a YAML component, or fallback when the
// spec is unset.
func componentOf(spec simulator.ComponentSpec, fallback string) *yaml3.Node {
	if spec.IsZero() {
		return component(fallback, nil)
	}
	return component(spec.Type, spec.Fields)
}

// floats converts state indices to the float params a partition reads them as.
func indexParams(indices []int) []float64 {
	values := make([]float64, len(indices))
	for i, index := range indices {
		values[i] = float64(index)
	}
	return values
}

// repeated is n copies of value.
func repeated(value float64, n int) []float64 {
	values := make([]float64, n)
	for i := range values {
		values[i] = value
	}
	return values
}

// expandedDocument is a macros: config written out in the core language.
type expandedDocument struct {
	Inputs   *yaml3.Node  `yaml:"inputs,omitempty"`
	Main     expandedMain `yaml:"main"`
	Embedded *yaml3.Node  `yaml:"embedded,omitempty"`
	Outputs  *yaml3.Node  `yaml:"outputs,omitempty"`
	Run      *yaml3.Node  `yaml:"run,omitempty"`
}

// expandedMain is the expanded run: its partitions, each an expandedPartition
// or, for one the config's main: states, its node as written.
type expandedMain struct {
	Partitions  []interface{} `yaml:"partitions"`
	Expressions *yaml3.Node   `yaml:"expressions,omitempty"`
	Simulation  interface{}   `yaml:"simulation"`
}

type expandedSimulation struct {
	TerminationCondition *yaml3.Node `yaml:"termination_condition"`
	TimestepFunction     *yaml3.Node `yaml:"timestep_function"`
	InitTimeValue        float64     `yaml:"init_time_value"`
}

// inputShape is what an expansion knows of an input: its partitions' widths,
// and the time its first row is at.
type inputShape struct {
	widths    map[string]int
	firstRows map[string][]float64
	initTime  float64
	// steps is how many steps a replay of the input runs for: one per row
	// after the first. It is 0 for a sub-simulation, whose rows are unknown
	// until it runs.
	steps int
}

// inputShapeOf finds an input's shape: from its declaration when it is a
// sub-simulation, whose partitions state their widths, and otherwise by
// loading it.
func inputShapeOf(input InputConfig) (inputShape, error) {
	if input.Simulation != nil && input.Simulation.Source == nil {
		shape := inputShape{widths: map[string]int{}, firstRows: map[string][]float64{},
			initTime: input.Simulation.InitTime}
		for _, partition := range input.Simulation.Partitions {
			shape.widths[partition.Name] = len(partition.InitStateValues)
			shape.firstRows[partition.Name] = partition.InitStateValues
		}
		return shape, nil
	}
	storage, err := input.load()
	if err != nil {
		return inputShape{}, err
	}
	shape := inputShape{widths: map[string]int{}, firstRows: map[string][]float64{},
		initTime: storage.GetTimes()[0], steps: len(storage.GetTimes()) - 1}
	for _, name := range storage.GetNames() {
		first := storage.GetValues(name)[0]
		shape.widths[name], shape.firstRows[name] = len(first), first
	}
	return shape, nil
}

// sameInputTimes checks several inputs share one time axis, which the macro
// tier requires of the inputs it analyses together and the expanded run
// relies on: it is clocked by one input and replays the others step for step.
func sameInputTimes(inputs map[string]InputConfig, names []string) error {
	if len(names) < 2 {
		return nil
	}
	var first []float64
	for _, name := range names {
		input := inputs[name]
		storage, err := input.load()
		if err != nil {
			return fmt.Errorf("api: reading input %q's times: %w", name, err)
		}
		if first == nil {
			first = storage.GetTimes()
			continue
		}
		if err := sameTimes(names[0], first, name, storage.GetTimes()); err != nil {
			return err
		}
	}
	return nil
}

// calibrationSplit checks the config's macros and inputs give its run one
// clock, and returns where the run starts: at 0, the inputs' clock or a first
// macro's own; or at the one macro with a clock of its own that reads the
// inputs inside its own runs, when other macros come before it (calibrate,
// then plan), which then run first, as an input of their own.
func calibrationSplit(config *ApiRunConfig, hasInputs, mainClock bool) (int, error) {
	var clocked []int
	for index, macro := range config.Macros {
		if _, live := macro.Spec.(liveMacroSpec); live {
			clocked = append(clocked, index)
		}
	}
	which := func(index int) string {
		return fmt.Sprintf("macros[%d] (%s)", index, config.Macros[index].Type)
	}
	switch {
	case mainClock && len(clocked) > 0:
		return 0, configError(fmt.Errorf("api: %s runs its own clock, and main.simulation "+
			"states one too; one run has one clock, so set one of them", which(clocked[0])))
	case len(clocked) > 1:
		return 0, configError(fmt.Errorf("api: %s and %s each run their own clock; one run has "+
			"one clock, so a second macro with its own does not expand",
			which(clocked[0]), which(clocked[1])))
	case hasInputs && len(clocked) == 1 && !readsInputsInside(config.Macros[clocked[0]]):
		return 0, configError(fmt.Errorf("api: %s runs its own clock beside the config's "+
			"inputs, but reads none of them; one run has one clock, so it does not expand",
			which(clocked[0])))
	case hasInputs && len(clocked) == 1:
		return clocked[0], nil
	case !hasInputs && !mainClock && (len(clocked) == 0 || clocked[0] != 0):
		return 0, configError(fmt.Errorf("api: expanding needs a clock: the inputs the " +
			"macros read, main.simulation, or a first macro that runs its own"))
	}
	return 0, nil
}

// insideReader is a macro with its own clock that reads the inputs only inside
// runs of its own, so the run it expands into needs no replay of them.
type insideReader interface {
	readsInputsInside() bool
}

// readsInputsInside reports that a macro reads the inputs only inside runs of
// its own.
func readsInputsInside(macro MacroConfig) bool {
	reader, ok := macro.Spec.(insideReader)
	return ok && reader.readsInputsInside()
}

// ExpandToYAML writes a macros: config out in the core language: one run, as
// YAML a person could have written. Every macro expands; a config whose macros
// cannot run as one run with one clock (see calibrationSplit) is an error
// saying why. Calibrate, then plan becomes a run: input, named calibration,
// holding the inputs and the macros before the planner, which the planner's
// run reads.
func ExpandToYAML(config *ApiRunConfig) ([]byte, error) {
	if len(config.Macros) == 0 {
		return nil, configError(fmt.Errorf("api: the config has no macros: to expand"))
	}
	inputs := macroInputs(config)
	mainClock := !simulationStringsZero(&config.Main.SimulationStrings)
	split, err := calibrationSplit(config, len(inputs) > 0, mainClock)
	if err != nil {
		return nil, err
	}
	if split > 0 && len(config.Main.Partitions) > 0 {
		return nil, configError(fmt.Errorf("api: main: partitions beside macros that " +
			"calibrate before a macro with its own clock (calibrate, then plan) do not " +
			"expand yet"))
	}
	source, err := sourceNodes(config)
	if err != nil {
		return nil, err
	}
	context, err := inputsContext(inputs)
	if err != nil {
		return nil, err
	}
	if mainClock {
		// main:'s own clock, not the inputs', says how long the run is.
		context.inputSteps = 0
	}
	// main:'s partitions run beside the macros', which may read them.
	for _, partition := range config.Main.Partitions {
		if input, replayed := context.owners[partition.Name]; replayed {
			return nil, configError(fmt.Errorf("api: main: defines partition %q, which "+
				"input %q has too; beside macros:, every input partition is replayed "+
				"under its own name, so rename the main: one or drop it", partition.Name, input))
		}
		context.widths[partition.Name] = len(partition.InitStateValues)
		context.firstRows[partition.Name] = partition.InitStateValues
	}
	document := expandedDocument{Inputs: source.inputs, Outputs: source.outputs,
		Embedded: source.embedded, Run: source.run}
	if split > 0 {
		calibration := expandedDocument{Inputs: source.inputs}
		if calibration.Main, err = expandRun(config.Macros, source.macros, 0, split, context, nil); err != nil {
			return nil, err
		}
		// The planner's run reads every partition the calibration recorded.
		for partition := range context.widths {
			context.owners[partition] = calibrationInput
		}
		document.Inputs = &yaml3.Node{}
		// The document is built from YAML-decoded values, so it always encodes.
		_ = document.Inputs.Encode(map[string]interface{}{
			calibrationInput: map[string]interface{}{"run": calibration}})
	}
	if document.Main, err = expandRun(config.Macros, source.macros, split, len(config.Macros),
		context, &source); err != nil {
		return nil, err
	}
	document.Main.Expressions = source.mainExpressions
	var written bytes.Buffer
	encoder := yaml3.NewEncoder(&written)
	encoder.SetIndent(2)
	// yaml.v3 quotes the text YAML 1.1, which configs load with, reads as a
	// boolean (a partition named y), so it loads back as written.
	_ = encoder.Encode(document)
	_ = encoder.Close()
	return written.Bytes(), nil
}

// calibrationInput names the run: input calibrate, then plan expands its
// calibration into.
const calibrationInput = "calibration"

// inputsContext is what an expansion knows of the inputs before any macro:
// their partitions' shapes, the input each comes from, and the time they start.
func inputsContext(inputs map[string]InputConfig) (*expansionContext, error) {
	context := &expansionContext{widths: map[string]int{}, firstRows: map[string][]float64{},
		inputs: map[string]bool{}, owners: map[string]string{}}
	var storage *simulator.StateTimeStorage
	context.stored = func() (*simulator.StateTimeStorage, error) {
		if storage == nil {
			loaded, err := loadMacroStorage(inputs)
			if err != nil {
				return nil, err
			}
			storage = loaded
		}
		return storage, nil
	}
	names := sortedKeys(inputs)
	if err := sameInputTimes(inputs, names); err != nil {
		return nil, err
	}
	for _, name := range names {
		shape, err := inputShapeOf(inputs[name])
		if err != nil {
			return nil, fmt.Errorf("api: reading input %q's shape: %w", name, err)
		}
		// Every input shares one time axis, so any gives the clock's start.
		context.initTime = shape.initTime
		context.inputSteps = shape.steps
		for partition, width := range shape.widths {
			if owner, taken := context.owners[partition]; taken {
				return nil, withKind(ErrData, fmt.Errorf(
					"api: partition %q is in both input %q and input %q", partition, owner, name))
			}
			context.widths[partition] = width
			context.firstRows[partition] = shape.firstRows[partition]
			context.inputs[partition] = true
			context.owners[partition] = name
		}
	}
	return context, nil
}

// expandRun expands macros[from:to] into one run: a replay of each input
// partition the context's owners name, then main:'s partitions as written
// (when hand is given), then the macros' partitions, clocked by a macro's own
// clock, in which case nothing is replayed, or by main:'s simulation, or by
// the first input.
func expandRun(
	macros []MacroConfig,
	sources []*yaml3.Node,
	from, to int,
	context *expansionContext,
	hand *expandedSource,
) (expandedMain, error) {
	var partitions []expandedPartition
	var clock *expandedClock
	depths := map[string]int{}
	depthFrom := map[string]string{}
	owners := context.owners
	for index := from; index < to; index++ {
		macro := macros[index]
		which := fmt.Sprintf("macros[%d] (%s)", index, macro.Type)
		context.source = sources[index]
		var expanded *expansion
		err := expanding(macro.Type, func() (err error) {
			expanded, err = macro.Spec.expand(context)
			return err
		})
		if err != nil {
			return expandedMain{}, err
		}
		if expanded.clock != nil {
			clock = expanded.clock
			// The run steps on this macro's clock, which only its own runs
			// read the inputs on, so nothing after it can read them.
			for partition := range owners {
				delete(context.widths, partition)
				delete(context.firstRows, partition)
				delete(context.inputs, partition)
			}
		}
		for _, name := range sortedKeys(expanded.depths) {
			depth := expanded.depths[name]
			if previous, set := depths[name]; set && previous != depth {
				return expandedMain{}, configError(fmt.Errorf("api: %s reads %q over a window of %d, "+
					"but %s reads it over %d; one run keeps one history per partition, so "+
					"macros reading the same partition over different windows do not "+
					"expand yet", which, name, depth, depthFrom[name], previous))
			}
			depths[name], depthFrom[name] = depth, which
		}
		for _, partition := range expanded.partitions {
			if _, taken := context.widths[partition.Name]; taken {
				return expandedMain{}, configError(fmt.Errorf("api: %s names its partition %q, which "+
					"main:, the inputs or an earlier macro already have", which, partition.Name))
			}
			context.widths[partition.Name] = len(partition.InitStateValues)
			context.firstRows[partition.Name] = context.initialState(partition)
			partitions = append(partitions, partition)
		}
	}
	for index := range partitions {
		if depth, set := depths[partitions[index].Name]; set {
			partitions[index].StateHistoryDepth = max(partitions[index].StateHistoryDepth, depth)
		}
	}
	var run expandedMain
	if clock == nil {
		for _, name := range sortedKeys(owners) {
			run.Partitions = append(run.Partitions, expandedPartition{
				Name:              name,
				Iteration:         component("from_input", map[string]interface{}{"input": owners[name]}),
				StateHistoryDepth: max(depths[name], 1),
			})
		}
	}
	if hand != nil {
		for _, partition := range hand.mainPartitions {
			if depth, set := depths[mappingValue(partition, "name").Value]; set {
				raiseHistoryDepth(partition, depth)
			}
			run.Partitions = append(run.Partitions, partition)
		}
	}
	for _, partition := range partitions {
		run.Partitions = append(run.Partitions, partition)
	}
	switch {
	case clock != nil:
		run.Simulation = expandedSimulation{
			TerminationCondition: component("number_of_steps", map[string]interface{}{"max_steps": clock.steps}),
			TimestepFunction:     component("constant", map[string]interface{}{"stepsize": clock.timestep}),
		}
	case hand != nil && hand.mainSimulation != nil:
		run.Simulation = hand.mainSimulation
	default:
		first := owners[sortedKeys(owners)[0]]
		run.Simulation = expandedSimulation{
			TerminationCondition: component("input_exhausted", map[string]interface{}{"input": first}),
			TimestepFunction:     component("from_input", map[string]interface{}{"input": first}),
			InitTimeValue:        context.initTime,
		}
	}
	return run, nil
}

// raiseHistoryDepth raises a partition node's state_history_depth to depth,
// leaving a deeper one as it is. Loading requires every main: partition to
// state a whole state_history_depth of at least 1.
func raiseHistoryDepth(partition *yaml3.Node, depth int) {
	node := mappingValue(partition, "state_history_depth")
	if current, _ := strconv.Atoi(node.Value); current < depth {
		node.Value = strconv.Itoa(depth)
	}
}

// Expand writes a macros: config out in the core language (ExpandToYAML) and
// loads the result, as a config the single runtime runs like any other.
func Expand(config *ApiRunConfig) (*ApiRunConfig, error) {
	expanded, err := ExpandToYAML(config)
	if err != nil {
		return nil, err
	}
	loaded, err := loadConfigData(expanded, config.sourcePath)
	if err != nil {
		return nil, fmt.Errorf("api: the expanded config does not load: %w", err)
	}
	return loaded, nil
}

// expandedSource is what an expansion keeps of the config as written.
type expandedSource struct {
	inputs, outputs, embedded, run *yaml3.Node
	macros                         []*yaml3.Node
	// mainPartitions, mainExpressions and mainSimulation are main:'s, as
	// written.
	mainPartitions                  []*yaml3.Node
	mainExpressions, mainSimulation *yaml3.Node
}

// sourceNodes takes the config's inputs (a data: block becoming the input
// named "data", as macroInputs reads it) and outputs from its source text, so
// they are written out exactly as given.
func sourceNodes(config *ApiRunConfig) (expandedSource, error) {
	if len(config.source) == 0 {
		return expandedSource{}, configError(fmt.Errorf("api: expanding writes the config's " +
			"inputs and outputs as they were written, and a config assembled in Go has no " +
			"source text; load it from YAML"))
	}
	// The source loaded, so it parses.
	var root yaml3.Node
	_ = yaml3.Unmarshal(config.source, &root)
	var kept expandedSource
	document := root.Content[0]
	for i := 0; i+1 < len(document.Content); i += 2 {
		key, value := document.Content[i].Value, document.Content[i+1]
		switch key {
		case "inputs":
			kept.inputs = value
		case "outputs":
			kept.outputs = value
		case "macros":
			kept.macros = value.Content
		case "embedded":
			kept.embedded = value
		case "run":
			kept.run = value
		case "main":
			if partitions := mappingValue(value, "partitions"); partitions != nil {
				kept.mainPartitions = partitions.Content
			}
			kept.mainExpressions = mappingValue(value, "expressions")
			kept.mainSimulation = mappingValue(value, "simulation")
		case "data":
			kind := "simulation"
			for j := 0; j+1 < len(value.Content); j += 2 {
				if value.Content[j].Value == "source" {
					kind, value = "source", value.Content[j+1]
					break
				}
			}
			kept.inputs = &yaml3.Node{Kind: yaml3.MappingNode, Content: []*yaml3.Node{
				{Kind: yaml3.ScalarNode, Value: "data"},
				{Kind: yaml3.MappingNode, Content: []*yaml3.Node{
					{Kind: yaml3.ScalarNode, Value: kind}, value}},
			}}
		}
	}
	return kept, nil
}
