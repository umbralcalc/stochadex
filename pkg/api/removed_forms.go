package api

import (
	"fmt"
	"strings"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	yaml3 "gopkg.in/yaml.v3"
)

// Removed shorthand. A config has one place for data coming in, inputs:, and one
// for results going out, outputs:. Two older spellings of those were removed,
// and a config still using one fails to load with an error that gives its
// replacement, written out from the config itself where it can be:
//
//   - data:, which was shorthand for a single input named data;
//   - the main run's simulation output_condition / output_function pair, which
//     was shorthand for one outputs: view named output.
//
// An embedded run's own output pair is not shorthand and still loads: it is
// that run's debug log.

// rejectRemovedData fails a config document with a top-level data: block, naming
// the input it becomes. It reads the document before anything decodes it, so
// the error names the replacement rather than reporting a key nothing reads.
func rejectRemovedData(document []byte) error {
	var top map[string]yaml3.Node
	if yaml3.Unmarshal(document, &top) != nil {
		// Decoding reports the document's own error.
		return nil
	}
	data, found := top["data"]
	if !found {
		return nil
	}
	replacement := "inputs: {data: {simulation: ...}}, its contents moved under simulation:"
	var block map[string]yaml3.Node
	if data.Decode(&block) == nil {
		if _, source := block["source"]; source {
			replacement = "inputs: {data: {source: ...}}"
		}
	}
	return fmt.Errorf("api: data: was removed: it was shorthand for one input, so "+
		"declare it under inputs: as %s, which macros read the same way", replacement)
}

// rejectRemovedOutputPair fails a config whose main simulation sets
// output_condition or output_function, giving the outputs: view they become.
func rejectRemovedOutputPair(config *ApiRunConfig) error {
	pair := config.Main.SimulationStrings
	if pair.OutputCondition.IsZero() && pair.OutputFunction.IsZero() {
		return nil
	}
	view := "{name: output"
	if !pair.OutputCondition.IsZero() {
		view += ", condition: " + flowSpec(pair.OutputCondition)
	}
	function := pair.OutputFunction
	if function.IsZero() {
		function = simulator.ComponentSpec{Type: "stdout"}
	}
	view += ", function: " + flowSpec(function) + "}"
	return fmt.Errorf("api: main.simulation's output_condition / output_function was "+
		"removed: declare the output as a view, outputs: [%s]", view)
}

// flowSpec writes a component spec as one line of YAML, its type first. Its
// fields came from a loaded config, so YAML can hold every value.
func flowSpec(spec simulator.ComponentSpec) string {
	mapping := &yaml3.Node{Kind: yaml3.MappingNode, Style: yaml3.FlowStyle}
	add := func(key string, value interface{}) {
		// Nested inside a flow mapping, collections are written in flow style too.
		var node yaml3.Node
		node.Encode(value)
		mapping.Content = append(mapping.Content,
			&yaml3.Node{Kind: yaml3.ScalarNode, Value: key}, &node)
	}
	add("type", spec.Type)
	for _, key := range sortedKeys(spec.Fields) {
		add(key, spec.Fields[key])
	}
	// A node of scalars, lists and mappings always encodes.
	out, _ := yaml3.Marshal(mapping)
	return strings.TrimSpace(string(out))
}
