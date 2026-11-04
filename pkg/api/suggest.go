package api

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	yaml3 "gopkg.in/yaml.v3"
)

// A built-in iteration is compiled, and runs the same maths several times faster than an
// expression, so an expression partition that restates a built-in is worth replacing.
// Suggest finds them. A suggestion is made in two steps, and only the second is trusted:
//
//  1. The partition's update is matched against templates, each a built-in's rule
//     from the catalogue written as a pattern with slots (A, B, ... for any part that
//     neither reads the state nor draws, X for the state, Z and U for a standard normal
//     and uniform draw; see suggest_templates.go). Matching allows terms and factors in
//     any order, one slot taking several of them, a draw written at its scale, and a
//     comparison with a quotient multiplied out, and nothing else.
//  2. The replacement is written into the config's own text, both configs are run, and
//     the suggestion stands only if the partition's rows agree at every step. Built-ins
//     draw from their partition's seeded stream as the expressions they replace do, so a
//     real match agrees to rounding, and any match that is not one is dropped.
//
// A slot filled by a param becomes that param of the built-in, and one filled by a number
// becomes a param of that value. A slot filled by anything else (2 + sin(t), say) is
// factored out: it becomes a partition of its own, an expression computing it, which
// feeds the built-in's param through params_from_upstream.

// Suggestion is a built-in that does what an expression partition does.
type Suggestion struct {
	// Partition is the expression partition to replace.
	Partition string `json:"partition"`
	// Iteration is the built-in's type.
	Iteration string `json:"iteration"`
	// YAML is the replacement, partition entries to put in place of Partition's: any
	// factored-out partitions, then the built-in. The partition's expressions: entry, if
	// it has one, goes.
	YAML string `json:"yaml"`
}

// suggestionStepCap is the most steps a suggestion is checked over.
const suggestionStepCap = 50

// Suggest returns a verified built-in for each expression partition of the config's
// simulation (its expansion's, for a macros: config) that restates one, in partition
// order. It runs the config once, capped at a few steps, whatever its run: says, so it
// reads its inputs; it writes nothing.
func Suggest(config *ApiRunConfig) ([]Suggestion, error) {
	text := config.source
	if len(config.Macros) > 0 {
		expanded, err := ExpandToYAML(config)
		if err != nil {
			return nil, err
		}
		text = expanded
	}
	if len(text) == 0 {
		return nil, configError(fmt.Errorf("api: suggesting built-ins reads the config's " +
			"text; load it with LoadConfig"))
	}
	loaded, reference, err := runCapped(text, config.sourcePath, "", nil)
	if err != nil {
		return nil, err
	}
	candidates, replacements := suggestionCandidates(loaded)
	suggestions := []Suggestion{}
	for index, candidate := range candidates {
		_, replaced, err := runCapped(text, config.sourcePath, candidate.Partition, replacements[index])
		if err != nil {
			continue // a replacement that does not run is not one
		}
		if sameRows(reference.GetValues(candidate.Partition), replaced.GetValues(candidate.Partition)) {
			suggestions = append(suggestions, candidate)
		}
	}
	return suggestions, nil
}

// suggestionCandidates matches each expression partition of a loaded config's main:
// run against the templates, without running anything, returning each match and its
// replacement partitions.
func suggestionCandidates(config *ApiRunConfig) ([]Suggestion, [][]*yaml3.Node) {
	expressions := map[string]*general.ExpressionIteration{}
	for index := range config.Main.Expressions {
		expressions[config.Main.Expressions[index].Partition] = &config.Main.Expressions[index].ExpressionIteration
	}
	var candidates []Suggestion
	var replacements [][]*yaml3.Node
	for _, partition := range config.Main.Partitions {
		iteration, ok := partition.Iteration.(*general.ExpressionIteration)
		if !ok {
			iteration, ok = expressions[partition.Name]
		}
		if !ok {
			continue
		}
		if suggestion, nodes, found := matchPartition(partition, iteration); found {
			candidates = append(candidates, suggestion)
			replacements = append(replacements, nodes)
		}
	}
	return candidates, replacements
}

// sameRows reports whether two runs of a partition agree at every step, to rounding.
func sameRows(want, got [][]float64) bool {
	if len(want) == 0 || len(want) != len(got) {
		return false
	}
	for step := range want {
		if len(want[step]) != len(got[step]) {
			return false
		}
		for i, value := range want[step] {
			scale := 1.0
			if value > 1 || value < -1 {
				scale = value
				if scale < 0 {
					scale = -scale
				}
			}
			difference := got[step][i] - value
			if difference > 1e-9*scale || difference < -1e-9*scale || difference != difference {
				return false
			}
		}
	}
	return true
}

// matchPartition matches a single-field expression partition, its bindings inlined,
// against the templates, and writes the replacement for the first that fits.
func matchPartition(
	partition simulator.PartitionConfig,
	iteration *general.ExpressionIteration,
) (Suggestion, []*yaml3.Node, bool) {
	if len(iteration.Fields) != 1 { // one output, as the config loaded
		return Suggestion{}, nil, false
	}
	// The config loaded, so its expressions parse.
	output, _ := parser.ParseExpr(iteration.Outputs[0])
	for index := len(iteration.Bindings) - 1; index >= 0; index-- {
		binding, _ := parser.ParseExpr(iteration.Bindings[index].Expr)
		output = substitute(output, iteration.Bindings[index].Name, binding)
	}
	field := iteration.Fields[0].Name
	expression := canonical(output)
	for _, template := range suggestionTemplates() {
		slots := map[string]*exprNode{}
		if !unify(template.pattern, expression, field, slots) {
			continue
		}
		nodes, ok := replacement(partition, iteration, template, slots)
		if !ok {
			continue
		}
		var text strings.Builder
		for _, node := range nodes {
			written, _ := yaml3.Marshal([]*yaml3.Node{node})
			text.Write(written)
		}
		return Suggestion{Partition: partition.Name, Iteration: template.iteration,
			YAML: text.String()}, nodes, true
	}
	return Suggestion{}, nil, false
}

// exprNode is an expression in a form where order does not matter: sums and products
// are flattened into lists, a - b is a + (-b), and a > b is b < a.
type exprNode struct {
	kind string // "+", "*", "/", "neg", "<", "<=", "==", "!=", "&&", "||", "call", "name", "num"
	name string
	kids []*exprNode
}

// canonical writes an expression as an exprNode.
func canonical(expression ast.Expr) *exprNode {
	switch typed := expression.(type) {
	case *ast.ParenExpr:
		return canonical(typed.X)
	case *ast.Ident:
		return &exprNode{kind: "name", name: typed.Name}
	case *ast.BasicLit:
		value, err := strconv.ParseFloat(typed.Value, 64)
		if err != nil {
			return &exprNode{kind: "name", name: typed.Value}
		}
		return &exprNode{kind: "num", name: strconv.FormatFloat(value, 'g', -1, 64)}
	case *ast.UnaryExpr:
		if typed.Op == token.SUB {
			return &exprNode{kind: "neg", kids: []*exprNode{canonical(typed.X)}}
		}
		return canonical(typed.X)
	case *ast.CallExpr:
		// shared(d) and iid(n, d) are d drawn once or n times: which a template's
		// built-in does is left to the run that checks the match.
		switch name := exprString(typed.Fun); {
		case name == "shared" && len(typed.Args) == 1:
			return canonical(typed.Args[0])
		case name == "iid" && len(typed.Args) == 2:
			return canonical(typed.Args[1])
		case name == "normal" && len(typed.Args) == 2:
			// normal(m, s) draws m + s Z, from the one standard draw Z.
			mean, scale := canonical(typed.Args[0]), canonical(typed.Args[1])
			if !isNumber(mean, "0") || !isNumber(scale, "1") {
				scaled := flatten("*", scale, standardNormal())
				if isNumber(mean, "0") {
					return scaled
				}
				return flatten("+", mean, scaled)
			}
		}
		node := &exprNode{kind: "call", name: exprString(typed.Fun)}
		for _, argument := range typed.Args {
			node.kids = append(node.kids, canonical(argument))
		}
		return node
	case *ast.BinaryExpr:
		left, right := canonical(typed.X), canonical(typed.Y)
		switch typed.Op {
		case token.ADD:
			return flatten("+", left, right)
		case token.SUB:
			return flatten("+", left, &exprNode{kind: "neg", kids: []*exprNode{right}})
		case token.MUL:
			return flatten("*", left, right)
		case token.QUO:
			return &exprNode{kind: "/", kids: []*exprNode{left, right}}
		case token.LSS:
			return comparison("<", left, right)
		case token.LEQ:
			return comparison("<=", left, right)
		case token.GTR:
			return comparison("<", right, left)
		case token.GEQ:
			return comparison("<=", right, left)
		default:
			return &exprNode{kind: typed.Op.String(), kids: []*exprNode{left, right}}
		}
	}
	return &exprNode{kind: "name", name: exprString(expression)}
}

// comparison is left < right (or <=) with a quotient on either side multiplied out, as
// a < b / c is a * c < b: the match only proposes, and the run that checks it settles
// the sign of c.
func comparison(kind string, left, right *exprNode) *exprNode {
	if right.kind == "/" {
		left, right = flatten("*", left, right.kids[1]), right.kids[0]
	} else if left.kind == "/" {
		left, right = left.kids[0], flatten("*", right, left.kids[1])
	}
	return &exprNode{kind: kind, kids: []*exprNode{left, right}}
}

// standardNormal is the draw normal(0, 1).
func standardNormal() *exprNode {
	return &exprNode{kind: "call", name: "normal", kids: []*exprNode{
		{kind: "num", name: "0"}, {kind: "num", name: "1"}}}
}

// isNumber reports whether node is the number value.
func isNumber(node *exprNode, value string) bool {
	return node.kind == "num" && node.name == value
}

// flatten joins two operands of an associative operator into one list.
func flatten(kind string, operands ...*exprNode) *exprNode {
	node := &exprNode{kind: kind}
	for _, operand := range operands {
		if operand.kind == kind {
			node.kids = append(node.kids, operand.kids...)
		} else {
			node.kids = append(node.kids, operand)
		}
	}
	return node
}

// exprString is an expression's source text, for a name or a callee.
func exprString(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		return exprString(typed.X) + "." + typed.Sel.Name
	}
	return fmt.Sprintf("%T", expression)
}

// drawNames are the expression functions that draw.
var drawNames = map[string]bool{"normal": true, "uniform": true, "gamma": true, "beta": true,
	"binomial": true, "exponential": true, "poisson": true, "iid": true, "shared": true}

// unify matches pattern against node, filling slots: X matches the state field, Z a
// standard normal draw and U a standard uniform one, and any
// other one-letter capital a part that neither reads the state nor draws, the same part
// wherever the letter repeats.
func unify(pattern, node *exprNode, field string, slots map[string]*exprNode) bool {
	if pattern.kind == "name" && len(pattern.name) == 1 && pattern.name[0] >= 'A' && pattern.name[0] <= 'Z' {
		switch pattern.name {
		case "X":
			return node.kind == "name" && node.name == field
		case "Z":
			return isStandardDraw(node, "normal", "0", "1")
		case "U":
			return isStandardDraw(node, "uniform", "0", "1")
		}
		if mentions(node, field) || draws(node) {
			return false
		}
		if bound, ok := slots[pattern.name]; ok {
			return sameNode(bound, node)
		}
		slots[pattern.name] = node
		return true
	}
	if pattern.kind == "+" || pattern.kind == "*" {
		if node.kind != pattern.kind {
			return false
		}
		if len(node.kids) == len(pattern.kids) {
			if _, ok := unifyLeaving(pattern.kids, node.kids, field, slots); ok {
				return true
			}
		}
		return unifyGrouped(pattern, node, field, slots)
	}
	if pattern.kind != node.kind || pattern.name != node.name || len(pattern.kids) != len(node.kids) {
		return false
	}
	for index := range pattern.kids {
		if !unify(pattern.kids[index], node.kids[index], field, slots) {
			return false
		}
	}
	return true
}

// unifyGrouped matches a sum or product with more operands than its pattern by letting one
// slot of the pattern take the operands the others leave, as R takes 2 + sin(t) in
// 2 + sin(t) + 1 / dt against R + 1 / dt.
func unifyGrouped(pattern, node *exprNode, field string, slots map[string]*exprNode) bool {
	for index, slot := range pattern.kids {
		if !isSlot(slot) {
			continue
		}
		others := append(append([]*exprNode{}, pattern.kids[:index]...), pattern.kids[index+1:]...)
		trial := copySlots(slots)
		left, ok := unifyLeaving(others, node.kids, field, trial)
		if !ok || !unify(slot, &exprNode{kind: node.kind, kids: left}, field, trial) {
			continue
		}
		for name, bound := range trial {
			slots[name] = bound
		}
		return true
	}
	return false
}

// unifyLeaving matches each pattern operand to a different node operand, in any order,
// and returns the node operands left over.
func unifyLeaving(patterns, nodes []*exprNode, field string, slots map[string]*exprNode) ([]*exprNode, bool) {
	if len(patterns) == 0 {
		return nodes, true
	}
	for index, node := range nodes {
		trial := copySlots(slots)
		if !unify(patterns[0], node, field, trial) {
			continue
		}
		rest := append(append([]*exprNode{}, nodes[:index]...), nodes[index+1:]...)
		if left, ok := unifyLeaving(patterns[1:], rest, field, trial); ok {
			for name, bound := range trial {
				slots[name] = bound
			}
			return left, true
		}
	}
	return nil, false
}

// isSlot reports whether a pattern node is a slot: a one-letter capital other than X, Z
// and U.
func isSlot(node *exprNode) bool {
	return node.kind == "name" && len(node.name) == 1 && node.name[0] >= 'A' && node.name[0] <= 'Z' &&
		node.name != "X" && node.name != "Z" && node.name != "U"
}

func copySlots(slots map[string]*exprNode) map[string]*exprNode {
	copied := make(map[string]*exprNode, len(slots))
	for name, node := range slots {
		copied[name] = node
	}
	return copied
}

// isStandardDraw reports whether node is draw(a, b).
func isStandardDraw(node *exprNode, draw, a, b string) bool {
	return node.kind == "call" && node.name == draw && len(node.kids) == 2 &&
		isNumber(node.kids[0], a) && isNumber(node.kids[1], b)
}

// mentions reports whether node reads the name.
func mentions(node *exprNode, name string) bool {
	if node.kind == "name" && node.name == name {
		return true
	}
	for _, kid := range node.kids {
		if mentions(kid, name) {
			return true
		}
	}
	return false
}

// draws reports whether node draws.
func draws(node *exprNode) bool {
	if node.kind == "call" && drawNames[node.name] {
		return true
	}
	for _, kid := range node.kids {
		if draws(kid) {
			return true
		}
	}
	return false
}

// sameNode reports whether two nodes are the same expression, operands in any order.
func sameNode(a, b *exprNode) bool {
	if a.kind != b.kind || a.name != b.name || len(a.kids) != len(b.kids) {
		return false
	}
	if a.kind != "+" && a.kind != "*" {
		for index := range a.kids {
			if !sameNode(a.kids[index], b.kids[index]) {
				return false
			}
		}
		return true
	}
	// Sameness is an equivalence, so pairing each operand with the first unpaired one
	// like it pairs them all if any pairing does.
	paired := make([]bool, len(b.kids))
	for _, kid := range a.kids {
		found := false
		for index, other := range b.kids {
			if !paired[index] && sameNode(kid, other) {
				paired[index], found = true, true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// substitute replaces every reading of a name in expression with value.
func substitute(expression ast.Expr, name string, value ast.Expr) ast.Expr {
	return astutilApply(expression, func(node ast.Expr) ast.Expr {
		if ident, ok := node.(*ast.Ident); ok && ident.Name == name {
			return &ast.ParenExpr{X: value}
		}
		return nil
	})
}

// astutilApply rebuilds an expression, replacing each subexpression replace returns a
// value for.
func astutilApply(expression ast.Expr, replace func(ast.Expr) ast.Expr) ast.Expr {
	if replaced := replace(expression); replaced != nil {
		return replaced
	}
	switch typed := expression.(type) {
	case *ast.ParenExpr:
		return &ast.ParenExpr{X: astutilApply(typed.X, replace)}
	case *ast.UnaryExpr:
		return &ast.UnaryExpr{Op: typed.Op, X: astutilApply(typed.X, replace)}
	case *ast.BinaryExpr:
		return &ast.BinaryExpr{X: astutilApply(typed.X, replace), Op: typed.Op,
			Y: astutilApply(typed.Y, replace)}
	case *ast.CallExpr:
		call := &ast.CallExpr{Fun: typed.Fun}
		for _, argument := range typed.Args {
			call.Args = append(call.Args, astutilApply(argument, replace))
		}
		return call
	}
	return expression
}

// nodeSource writes an exprNode back as expression text.
func nodeSource(node *exprNode) string {
	switch node.kind {
	case "name", "num":
		return node.name
	case "neg":
		return "-(" + nodeSource(node.kids[0]) + ")"
	case "call":
		arguments := make([]string, len(node.kids))
		for index, kid := range node.kids {
			arguments[index] = nodeSource(kid)
		}
		return node.name + "(" + strings.Join(arguments, ", ") + ")"
	case "+", "*":
		operands := make([]string, len(node.kids))
		for index, kid := range node.kids {
			operands[index] = "(" + nodeSource(kid) + ")"
		}
		return strings.Join(operands, " "+node.kind+" ")
	}
	return "(" + nodeSource(node.kids[0]) + ") " + node.kind + " (" + nodeSource(node.kids[1]) + ")"
}

// replacement writes the partitions that replace an expression partition with a
// template's built-in: a factored-out partition for each slot that is neither a param
// nor a number, then the built-in, with the partition's name, initial state and seed.
func replacement(
	partition simulator.PartitionConfig,
	iteration *general.ExpressionIteration,
	template suggestionTemplate,
	slots map[string]*exprNode,
) ([]*yaml3.Node, bool) {
	width := len(partition.InitStateValues)
	builtin := expandedPartition{
		Name:               partition.Name,
		Iteration:          component(template.iteration, nil),
		Params:             map[string][]float64{},
		ParamsFromUpstream: map[string]expandedUpstream{},
		InitStateValues:    partition.InitStateValues,
		InitStateFromUp:    partition.InitStateFromUpstream,
		StateHistoryDepth:  partition.StateHistoryDepth,
		Seed:               partition.Seed,
	}
	var factored []expandedPartition
	for _, param := range sortedKeys(template.params) {
		slot := template.params[param]
		if slot == "" {
			builtin.Params[param] = repeated(0, width)
			continue
		}
		node := slots[slot]
		isParam := node.kind == "name" && (partition.ParamsFromUpstream[node.name].Upstream != "" ||
			partition.Params.Map[node.name] != nil)
		if template.paramsOnly && !isParam && node.kind != "num" {
			return nil, false
		}
		switch {
		case node.kind == "num":
			value, _ := strconv.ParseFloat(node.name, 64)
			builtin.Params[param] = repeated(value, width)
		case node.kind == "name" && partition.ParamsFromUpstream[node.name].Upstream != "":
			upstream := partition.ParamsFromUpstream[node.name]
			builtin.ParamsFromUpstream[param] = expandedUpstream{Upstream: upstream.Upstream,
				Indices: upstream.Indices}
		case node.kind == "name" && partition.Params.Map[node.name] != nil:
			builtin.Params[param] = partition.Params.Map[node.name]
		default:
			if node.kind == "name" && (node.name == "dt" || node.name == "t" || node.name == "step") {
				return nil, false
			}
			name := partition.Name + "_" + param
			// The field is never read, but must not take a name the expression reads.
			field := param
			for partition.Params.Map[field] != nil || iteration.Upstreams[field] != "" {
				field += "_"
			}
			fields := []map[string]interface{}{{"name": field, "width": width}}
			spec := map[string]interface{}{"fields": fields, "outputs": []string{nodeSource(node)}}
			if len(iteration.Upstreams) > 0 {
				spec["upstreams"] = iteration.Upstreams
			}
			piece := expandedPartition{
				Name:               name,
				Iteration:          component("expression", spec),
				Params:             partition.Params.Map,
				ParamsFromUpstream: map[string]expandedUpstream{},
				InitStateValues:    repeated(0, width),
				StateHistoryDepth:  1,
			}
			for key, upstream := range partition.ParamsFromUpstream {
				piece.ParamsFromUpstream[key] = expandedUpstream{Upstream: upstream.Upstream,
					Indices: upstream.Indices}
			}
			factored = append(factored, piece)
			builtin.ParamsFromUpstream[param] = expandedUpstream{Upstream: name}
		}
	}
	for key, upstream := range partition.ParamsFromUpstream {
		if key == partition.InitStateFromUpstream {
			builtin.ParamsFromUpstream[key] = expandedUpstream{Upstream: upstream.Upstream,
				Indices: upstream.Indices}
		}
	}
	readable(append(factored, builtin))
	var nodes []*yaml3.Node
	for _, piece := range append(factored, builtin) {
		node := &yaml3.Node{}
		encoded, _ := piece.MarshalYAML()
		*node = *encoded.(*yaml3.Node)
		nodes = append(nodes, node)
	}
	return nodes, true
}

// runCapped runs a config's text for at most suggestionStepCap steps, with the named
// partition's entry, if any, replaced by nodes and its expressions: entry removed, and
// returns the config it ran and what it recorded.
func runCapped(
	text []byte,
	path, partition string,
	nodes []*yaml3.Node,
) (*ApiRunConfig, *simulator.StateTimeStorage, error) {
	// The text loaded, so it reads and writes back as YAML.
	var document yaml3.Node
	_ = yaml3.Unmarshal(text, &document)
	root := document.Content[0]
	// The check is of the partitions' maths, so it is one batch run, recording only to
	// memory, however the config says to run and where to write.
	var kept []*yaml3.Node
	for index := 0; index < len(root.Content); index += 2 {
		if key := root.Content[index].Value; key != "run" && key != "outputs" {
			kept = append(kept, root.Content[index], root.Content[index+1])
		}
	}
	root.Content = kept
	if main := mappingValue(root, "main"); main != nil {
		if partitions := mappingValue(main, "partitions"); partitions != nil && partition != "" {
			var kept []*yaml3.Node
			for _, entry := range partitions.Content {
				if name := mappingValue(entry, "name"); name != nil && name.Value == partition {
					kept = append(kept, nodes...)
					continue
				}
				kept = append(kept, entry)
			}
			partitions.Content = kept
		}
		if expressions := mappingValue(main, "expressions"); expressions != nil && partition != "" {
			var kept []*yaml3.Node
			for _, entry := range expressions.Content {
				if name := mappingValue(entry, "partition"); name != nil && name.Value == partition {
					continue
				}
				kept = append(kept, entry)
			}
			expressions.Content = kept
		}
		if simulation := mappingValue(main, "simulation"); simulation != nil {
			if termination := mappingValue(simulation, "termination_condition"); termination != nil {
				if steps := mappingValue(termination, "max_steps"); steps != nil {
					if value, err := strconv.Atoi(steps.Value); err == nil && value > suggestionStepCap {
						steps.Value = strconv.Itoa(suggestionStepCap)
					}
				}
			}
		}
	}
	written, _ := yaml3.Marshal(&document)
	config, err := loadConfigData(written, path)
	var result *RunResult
	if err == nil {
		result, err = RunToStorage(config)
	}
	if err != nil {
		return nil, nil, err
	}
	return config, result.Storage, nil
}

// suggestionNotices are the one-line notices --check prints for a config: each
// expression partition whose update matches a built-in's, before any run confirms it.
func suggestionNotices(config *ApiRunConfig) []string {
	candidates, _ := suggestionCandidates(config)
	notices := make([]string, len(candidates))
	for index, candidate := range candidates {
		notices[index] = fmt.Sprintf("partition %q looks like the built-in %s, which runs the "+
			"same maths faster; stochadex inspect --suggest checks it and gives the replacement",
			candidate.Partition, candidate.Iteration)
	}
	return notices
}
