package api

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	yaml3 "gopkg.in/yaml.v3"
)

// Macros as config → config shortcuts (PLAN 2.1–2.3). A macro expands into
// core-language partitions — data specs only, as a hand-written config would
// state them — so a macros: config becomes one ordinary run:
//
//	inputs:  the config's inputs, unchanged
//	main:    a {type: from_input} replay of every input partition, then each
//	         macro's partitions, in order, clocked by the inputs' timestamps
//	outputs: the config's outputs, unchanged
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
}

// firstRow is a visible partition's first row.
func (c *expansionContext) firstRow(partition string) ([]float64, error) {
	if _, err := c.width(partition); err != nil {
		return nil, err
	}
	return c.firstRows[partition], nil
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

// expandingMacro is a macro that expands into core-language config.
type expandingMacro interface {
	expand(ctx *expansionContext) (*expansion, error)
}

// expandedPartition is one partition of an expansion, as data.
type expandedPartition struct {
	Name               string                      `yaml:"name"`
	Iteration          *yaml3.Node                 `yaml:"iteration"`
	Params             map[string][]float64        `yaml:"params,omitempty,flow"`
	ParamsAsPartitions map[string][]string         `yaml:"params_as_partitions,omitempty,flow"`
	ParamsFromUpstream map[string]expandedUpstream `yaml:"params_from_upstream,omitempty"`
	InitStateValues    []float64                   `yaml:"init_state_values,omitempty,flow"`
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
	Inputs  *yaml3.Node  `yaml:"inputs,omitempty"`
	Main    expandedMain `yaml:"main"`
	Outputs *yaml3.Node  `yaml:"outputs,omitempty"`
}

type expandedMain struct {
	Partitions []expandedPartition `yaml:"partitions"`
	Simulation expandedSimulation  `yaml:"simulation"`
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
		initTime: storage.GetTimes()[0]}
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

// oneClock checks the config's macros and inputs give its run one clock: the
// inputs', or a first macro's own. A macro with its own clock replaces what ran
// before it, in the macro tier; one run cannot, so that does not expand yet.
func oneClock(config *ApiRunConfig, hasInputs bool) error {
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
	case len(clocked) > 0 && hasInputs:
		return configError(fmt.Errorf("api: %s runs its own clock beside the config's "+
			"inputs, which run another; one run has one clock, and a macro with its own "+
			"beside inputs (calibrate, then plan) does not expand yet", which(clocked[0])))
	case len(clocked) > 1:
		return configError(fmt.Errorf("api: %s and %s each run their own clock; one run has "+
			"one clock, so a second macro with its own does not expand",
			which(clocked[0]), which(clocked[1])))
	case !hasInputs && (len(clocked) == 0 || clocked[0] != 0):
		return configError(fmt.Errorf("api: expanding needs the inputs the macros read, or " +
			"a first macro that runs its own clock"))
	}
	return nil
}

// ExpandToYAML writes a macros: config out in the core language: one run, as
// YAML a person could have written. Every macro must expand; one that does
// not yet is an error naming it.
func ExpandToYAML(config *ApiRunConfig) ([]byte, error) {
	if len(config.Macros) == 0 {
		return nil, configError(fmt.Errorf("api: the config has no macros: to expand"))
	}
	for _, macro := range config.Macros {
		if _, ok := macro.Spec.(expandingMacro); !ok {
			return nil, configError(fmt.Errorf("api: macro %q does not expand yet", macro.Type))
		}
	}
	inputs := macroInputs(config)
	if err := oneClock(config, len(inputs) > 0); err != nil {
		return nil, err
	}
	source, err := sourceNodes(config)
	if err != nil {
		return nil, err
	}
	names := sortedKeys(inputs)
	context := &expansionContext{widths: map[string]int{}, firstRows: map[string][]float64{},
		inputs: map[string]bool{}}
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
	owners := map[string]string{}
	var initTime float64
	if err := sameInputTimes(inputs, names); err != nil {
		return nil, err
	}
	for _, name := range names {
		shape, err := inputShapeOf(inputs[name])
		if err != nil {
			return nil, fmt.Errorf("api: reading input %q's shape: %w", name, err)
		}
		// Every input shares one time axis, so any gives the clock's start.
		initTime = shape.initTime
		for partition, width := range shape.widths {
			context.widths[partition] = width
			context.firstRows[partition] = shape.firstRows[partition]
			context.inputs[partition] = true
			owners[partition] = name
		}
	}

	var partitions []expandedPartition
	var clock *expandedClock
	depths := map[string]int{}
	depthFrom := map[string]string{}
	for index, macro := range config.Macros {
		which := fmt.Sprintf("macros[%d] (%s)", index, macro.Type)
		expander := macro.Spec.(expandingMacro)
		context.source = source.macros[index]
		var expanded *expansion
		err := expanding(macro.Type, func() (err error) {
			expanded, err = expander.expand(context)
			return err
		})
		if err != nil {
			return nil, err
		}
		if expanded.clock != nil {
			clock = expanded.clock
		}
		for _, name := range sortedKeys(expanded.depths) {
			depth := expanded.depths[name]
			if previous, set := depths[name]; set && previous != depth {
				return nil, configError(fmt.Errorf("api: %s reads %q over a window of %d, "+
					"but %s reads it over %d; one run keeps one history per partition, so "+
					"macros reading the same partition over different windows do not "+
					"expand yet", which, name, depth, depthFrom[name], previous))
			}
			depths[name], depthFrom[name] = depth, which
		}
		for _, partition := range expanded.partitions {
			if _, taken := context.widths[partition.Name]; taken {
				return nil, configError(fmt.Errorf("api: %s names its partition %q, which "+
					"the inputs or an earlier macro already have", which, partition.Name))
			}
			context.widths[partition.Name] = len(partition.InitStateValues)
			context.firstRows[partition.Name] = partition.InitStateValues
			partitions = append(partitions, partition)
		}
	}

	replays := make([]expandedPartition, 0, len(owners))
	for _, name := range sortedKeys(owners) {
		replays = append(replays, expandedPartition{
			Name:              name,
			Iteration:         component("from_input", map[string]interface{}{"input": owners[name]}),
			StateHistoryDepth: max(depths[name], 1),
		})
	}
	for index := range partitions {
		if depth, set := depths[partitions[index].Name]; set {
			partitions[index].StateHistoryDepth = max(partitions[index].StateHistoryDepth, depth)
		}
	}

	document := expandedDocument{Main: expandedMain{Partitions: append(replays, partitions...)}}
	if clock != nil {
		document.Main.Simulation = expandedSimulation{
			TerminationCondition: component("number_of_steps", map[string]interface{}{"max_steps": clock.steps}),
			TimestepFunction:     component("constant", map[string]interface{}{"stepsize": clock.timestep}),
		}
	} else {
		document.Main.Simulation = expandedSimulation{
			TerminationCondition: component("input_exhausted", map[string]interface{}{"input": names[0]}),
			TimestepFunction:     component("from_input", map[string]interface{}{"input": names[0]}),
			InitTimeValue:        initTime,
		}
	}
	document.Inputs, document.Outputs = source.inputs, source.outputs
	var written bytes.Buffer
	encoder := yaml3.NewEncoder(&written)
	encoder.SetIndent(2)
	// The document is built from YAML-decoded values, so it always encodes.
	_ = encoder.Encode(document)
	_ = encoder.Close()
	return written.Bytes(), nil
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
	inputs, outputs *yaml3.Node
	macros          []*yaml3.Node
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
