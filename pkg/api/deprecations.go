package api

import (
	"fmt"
	"strings"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	yaml3 "gopkg.in/yaml.v3"
)

// Deprecated shorthand (PLAN.md IO.6). Rule 14 gives a config one place for
// data coming in, inputs:, and one for results going out, outputs:. The older
// spellings still load, and mean exactly what their replacements do, but each
// is deprecated and will be removed in a later v0.x minor:
//
//   - data:, shorthand for a single input named data;
//   - the main run's simulation output_condition / output_function pair,
//     shorthand for one outputs: view named output;
//   - --socket, shorthand for run: {mode: serve} (see withSocketAlias).
//
// LoadConfig records a notice for each deprecated form a config uses, and the
// CLI prints them once, on stderr. The library does not print them itself, so
// a program loading configs keeps control of its own output; it can read them
// with Deprecations. An embedded run's own output pair is not deprecated: it is
// that run's debug log.

// Deprecations are notices for the deprecated forms the config uses, each
// naming its replacement.
func (a *ApiRunConfig) Deprecations() []string {
	return a.deprecations
}

// findDeprecations records a notice for each deprecated form in config. It runs
// before resolveOutputs folds the output pair into outputs:.
func findDeprecations(config *ApiRunConfig) {
	if config.Data != nil {
		form := "source"
		if config.Data.Source == nil {
			form = "simulation"
		}
		config.deprecations = append(config.deprecations, fmt.Sprintf(
			"data: is deprecated: it is shorthand for one input, so declare it under inputs: "+
				"as inputs: {data: {%s: ...}}, which macros read the same way", form))
	}
	pair := config.Main.SimulationStrings
	if !pair.OutputCondition.IsZero() || !pair.OutputFunction.IsZero() {
		view := "{name: output"
		if !pair.OutputCondition.IsZero() {
			view += ", condition: " + flowSpec(pair.OutputCondition)
		}
		function := pair.OutputFunction
		if function.IsZero() {
			function = simulator.ComponentSpec{Type: "stdout"}
		}
		view += ", function: " + flowSpec(function) + "}"
		config.deprecations = append(config.deprecations,
			"main.simulation's output_condition / output_function is deprecated: declare the "+
				"output as a view, outputs: ["+view+"]")
	}
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
