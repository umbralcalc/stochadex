package api

import (
	"fmt"
	"regexp"

	yaml3 "gopkg.in/yaml.v3"
)

// An expansion is written for a person to read as well as for a run to load,
// so the components it generates are laid out the way one would write them:
// a component that fits on a line stays on it, in flow style ({type: normal},
// [1.8, 5.0]); one that does not is written in block style, and the same
// choice is made again for each of its fields. A component that appears more
// than once (a planning environment both the tree and its apply read, say) is
// written once, anchored, and referred to by alias everywhere else.

// flowWidth is the longest a component may be, written in flow style, and
// still stay on one line.
const flowWidth = 80

// readable lays out the iterations of a run's generated partitions, and
// writes each component they repeat once.
func readable(partitions []expandedPartition) {
	iterations := make([]*yaml3.Node, len(partitions))
	for index := range partitions {
		lay(partitions[index].Iteration)
		iterations[index] = partitions[index].Iteration
	}
	share(iterations)
}

// MarshalYAML writes the partition with its params and wiring laid out as
// its iteration is.
func (p expandedPartition) MarshalYAML() (interface{}, error) {
	// fields is the partition without this method, so encoding it does not
	// recurse.
	type fields expandedPartition
	// Encoding writes text and reads it back, which an alias whose anchor is
	// in another partition cannot survive, so the iteration is set after.
	iteration := p.Iteration
	p.Iteration = nil
	node := &yaml3.Node{}
	// Every value here came from YAML, so it encodes back.
	_ = node.Encode(fields(p))
	for index := 1; index < len(node.Content); index += 2 {
		switch node.Content[index-1].Value {
		case "iteration":
			node.Content[index] = iteration
		case "params", "params_as_partitions", "params_from_upstream":
			lay(node.Content[index])
		}
	}
	return node, nil
}

// lay chooses each collection's style in node, outermost first.
func lay(node *yaml3.Node) {
	if node.Kind != yaml3.MappingNode && node.Kind != yaml3.SequenceNode {
		return
	}
	if scalarsOnly(node) || len(flowText(node)) <= flowWidth {
		node.Style = yaml3.FlowStyle
		return
	}
	node.Style = 0
	for _, child := range node.Content {
		lay(child)
	}
}

// scalarsOnly reports whether a sequence holds only scalars, as a list of
// values does: it is written in flow style however long.
func scalarsOnly(node *yaml3.Node) bool {
	if node.Kind != yaml3.SequenceNode {
		return false
	}
	for _, child := range node.Content {
		if child.Kind != yaml3.ScalarNode {
			return false
		}
	}
	return true
}

// flowText is node written entirely in flow style.
func flowText(node *yaml3.Node) string {
	flow := flowCopy(node)
	// The node was built from YAML-decoded values, so it always encodes.
	written, _ := yaml3.Marshal(flow)
	return string(written[:len(written)-1])
}

// flowCopy is a copy of node with every collection in flow style.
func flowCopy(node *yaml3.Node) *yaml3.Node {
	copied := *node
	if node.Kind == yaml3.MappingNode || node.Kind == yaml3.SequenceNode {
		copied.Style = yaml3.FlowStyle
		copied.Content = make([]*yaml3.Node, len(node.Content))
		for index, child := range node.Content {
			copied.Content[index] = flowCopy(child)
		}
	}
	return &copied
}

// share anchors the first of each mapping that appears more than once, too
// long for one line, and makes every later one an alias of it. The largest
// repeated mapping is shared whole, so a repeat inside it is not shared again.
func share(roots []*yaml3.Node) {
	texts := map[*yaml3.Node]string{}
	var measure func(node *yaml3.Node)
	measure = func(node *yaml3.Node) {
		if node.Kind == yaml3.MappingNode && node.Style != yaml3.FlowStyle {
			texts[node] = flowText(node)
		}
		for _, child := range node.Content {
			measure(child)
		}
	}
	for _, root := range roots {
		measure(root)
	}
	count := map[string]int{}
	for _, text := range texts {
		count[text]++
	}
	first := map[string]*yaml3.Node{}
	names := map[string]bool{}
	var visit func(node *yaml3.Node, key string)
	visit = func(node *yaml3.Node, key string) {
		if text, measured := texts[node]; measured && count[text] > 1 {
			if anchored, seen := first[text]; seen {
				if anchored.Anchor == "" {
					anchored.Anchor = anchorName(key, names)
				}
				*node = yaml3.Node{Kind: yaml3.AliasNode, Value: anchored.Anchor, Alias: anchored}
				return
			}
			first[text] = node
		}
		for index, child := range node.Content {
			childKey := key
			if node.Kind == yaml3.MappingNode && index%2 == 1 {
				childKey = node.Content[index-1].Value
			}
			visit(child, childKey)
		}
	}
	for _, root := range roots {
		visit(root, "iteration")
	}
}

// anchorUnsafe is what an anchor name may not hold.
var anchorUnsafe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// anchorName names an anchor after the field it is the value of, numbering a
// second of the same name, and records it in names.
func anchorName(key string, names map[string]bool) string {
	base := anchorUnsafe.ReplaceAllString(key, "_")
	name := base
	for index := 2; names[name]; index++ {
		name = fmt.Sprintf("%s_%d", base, index)
	}
	names[name] = true
	return name
}
