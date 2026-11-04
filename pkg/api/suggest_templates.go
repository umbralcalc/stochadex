package api

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"sync"
)

// The templates Suggest matches are generated from the catalogue's update rules, so every
// built-in whose rule is a formula of its own state is matched, and a new one is matched
// as soon as its doc comment states its rule. A rule translates when it is one line,
// x[i] <- ... (or x <- ...), over the partition's state, its params by key, dt, t, step,
// numbers and functions, the standard draws it declares (Z[i] ~ N(0, 1), U[i] ~ U(0, 1))
// and Bernoulli(p). Every other built-in is an alias of one that translates, or is
// excluded with its reason; a test holds the three lists to the catalogue.

// suggestionTemplate is one built-in's rule as a pattern with slots, and which of the
// built-in's params each slot fills. A param mapped to "" is set to 0.
type suggestionTemplate struct {
	iteration string
	pattern   *exprNode
	params    map[string]string
	// paramsOnly marks a template whose slots must be params or numbers: factoring
	// its slot out would only move the expression, not replace it.
	paramsOnly bool
}

// suggestionAliases are built-ins that compute exactly another's update, by the
// name of the one suggested in their place.
var suggestionAliases = map[string]string{
	"cox_process":    "poisson_process",
	"hawkes_process": "poisson_process",
}

// suggestionExclusions are the built-ins whose rule no template states, and why.
var suggestionExclusions = map[string]string{
	"binomial_observation_process":        "it reads its trials by index into another param",
	"categorical_state_transition":        "it chooses among a variable number of transitions by its state",
	"compound_poisson_process":            "it takes a jump distribution",
	"copy_values":                         "it reads other partitions' state by index",
	"cumulative":                          "it wraps a nested iteration",
	"data_comparison":                     "it takes a likelihood",
	"data_comparison_gradient":            "it takes a likelihood over a batch of history",
	"data_generation":                     "it takes a likelihood",
	"discounted_cumulative":               "it wraps a nested iteration",
	"drift_jump_diffusion":                "it takes a jump distribution",
	"embedded_simulation_run":             "it runs a nested simulation",
	"ensemble_kalman_filter":              "it solves a linear system over an ensemble",
	"expression":                          "it is the expression iteration itself",
	"from_history":                        "it replays a history",
	"from_input":                          "it replays an input",
	"from_storage":                        "it replays stored data",
	"gradient_descent":                    "its rule has a case, for ascent",
	"hawkes_process_intensity":            "it takes a kernel over another partition's history",
	"mcts_apply":                          "it runs an environment's decision rules",
	"mcts_rollout":                        "it runs an environment's decision rules",
	"mcts_tree":                           "it runs an environment's decision rules",
	"ornstein_uhlenbeck_exact_gaussian":   "its rule has a case, for thetas = 0",
	"posterior_covariance":                "it combines other partitions' log-likelihoods",
	"posterior_log_normalisation":         "it combines other partitions' log-likelihoods over their history",
	"posterior_mean":                      "it combines other partitions' log-likelihoods",
	"replicated_simulation_run":           "it runs nested simulations",
	"scalar_regression_stats":             "it keeps running sums, with cases",
	"smc_posterior":                       "it combines particles' log-likelihoods",
	"smc_proposal":                        "it draws from priors and a fitted normal",
	"values_changing_events":              "it switches between nested iterations",
	"values_collection":                   "it keeps a collection of slots",
	"values_function":                     "it takes a function",
	"values_function_cumulative_mean":     "it takes a function",
	"values_function_vector_covariance":   "it takes a function and a kernel over a window",
	"values_function_vector_mean":         "it takes a function and a kernel over a window",
	"values_grouped_aggregation":          "it aggregates grouped values with a kernel",
	"values_sorted_collection_covariance": "it reads a sorted collection",
	"values_sorted_collection_mean":       "it reads a sorted collection",
	"values_sorting_collection":           "it keeps a sorted collection",
	"values_weighted_resampling":          "it resamples other partitions' history",
}

// suggestionTemplates are the templates of every built-in whose rule translates, in name
// order, generated once.
var suggestionTemplates = sync.OnceValue(func() []suggestionTemplate {
	described := catalogueFields()["iterations"]
	var templates []suggestionTemplate
	for _, name := range sortedKeys(described) {
		if _, aliased := suggestionAliases[name]; aliased {
			continue
		}
		generated, _ := templatesFromRule(name, described[name])
		templates = append(templates, generated...)
	}
	return templates
})

var (
	// ruleSeparator parts a rule from its declarations: x[i] <- ...,  Z[i] ~ N(0, 1).
	ruleSeparator = regexp.MustCompile(`,\s{2,}`)
	// ruleElement is a name read at element i, or at 0 for a param of one value.
	ruleElement = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\[(?:i|0)\]`)
)

// slotLetters name a template's slots: one-letter capitals other than X, Z and U.
const slotLetters = "ABCDEFGHIJKLMNOPQRSTVWY"

// templatesFromRule writes a built-in's update rule as templates, or says why it can't:
// the rule itself, and the rule with each term dropped whose params it alone reads,
// those params set to 0, so long as the rest still draws.
func templatesFromRule(name string, entry CatalogueEntry) ([]suggestionTemplate, error) {
	rule := strings.TrimSpace(entry.Update)
	if strings.Contains(rule, "\n") {
		return nil, errors.New("its rule has more than one line")
	}
	parts := ruleSeparator.Split(rule, -1)
	head, found := strings.CutPrefix(parts[0], "x[i] <- ")
	if !found {
		head, found = strings.CutPrefix(parts[0], "x <- ")
	}
	if !found {
		return nil, fmt.Errorf("its rule %q does not give x", parts[0])
	}
	draws := map[string]string{}
	for _, declaration := range parts[1:] {
		drawn, distribution, _ := strings.Cut(declaration, " ~ ")
		drawn = ruleElement.ReplaceAllString(drawn, "$1")
		switch distribution {
		case "N(0, 1)":
			draws[drawn] = "Z"
		case "U(0, 1)":
			draws[drawn] = "U"
		default:
			return nil, fmt.Errorf("it draws %q", declaration)
		}
	}
	head = strings.ReplaceAll(head, "x@1", "x")
	if strings.HasPrefix(head, "[") && strings.HasSuffix(head, "]") {
		head = head[1 : len(head)-1]
	}
	head = ruleElement.ReplaceAllString(head, "$1")
	if strings.ContainsAny(head, "[@") {
		return nil, errors.New("it reads elements or history")
	}
	expression, err := parser.ParseExpr(head)
	if err != nil {
		return nil, fmt.Errorf("its rule is not an expression: %w", err)
	}
	params := map[string]bool{}
	for _, param := range entry.Params {
		params[param.Name] = true
	}
	slots := map[string]string{}
	var unread error
	var replace func(ast.Expr) ast.Expr
	replace = func(node ast.Expr) ast.Expr {
		switch typed := node.(type) {
		case *ast.CallExpr:
			if exprString(typed.Fun) == "Bernoulli" && len(typed.Args) == 1 {
				// U < p is 1 with probability p, as the built-ins draw it.
				return &ast.CallExpr{Fun: ast.NewIdent("where"), Args: []ast.Expr{
					&ast.BinaryExpr{X: ast.NewIdent("U"), Op: token.LSS, Y: astutilApply(typed.Args[0], replace)},
					&ast.BasicLit{Kind: token.INT, Value: "1"}, &ast.BasicLit{Kind: token.INT, Value: "0"}}}
			}
		case *ast.Ident:
			switch {
			case typed.Name == "x":
				return ast.NewIdent("X")
			case draws[typed.Name] != "":
				return ast.NewIdent(draws[typed.Name])
			case typed.Name == "dt" || typed.Name == "t" || typed.Name == "step":
			case params[typed.Name]:
				if slots[typed.Name] == "" {
					if len(slots) == len(slotLetters) {
						unread = errors.New("it reads more params than a template has slots")
						return nil
					}
					slots[typed.Name] = slotLetters[len(slots) : len(slots)+1]
				}
				return ast.NewIdent(slots[typed.Name])
			default:
				unread = fmt.Errorf("it reads %s, which is not one of its params", typed.Name)
			}
		}
		return nil
	}
	pattern := canonical(astutilApply(expression, replace))
	if unread != nil {
		return nil, unread
	}
	templates := []suggestionTemplate{{iteration: name, pattern: pattern, params: slots,
		paramsOnly: isSlot(pattern)}}
	if pattern.kind != "+" {
		return templates, nil
	}
	for index, term := range pattern.kids {
		rest := append(append([]*exprNode{}, pattern.kids[:index]...), pattern.kids[index+1:]...)
		remaining := flatten("+", rest...)
		if len(rest) == 1 {
			remaining = rest[0]
		}
		dropped, any := map[string]string{}, false
		for param, slot := range slots {
			if mentions(term, slot) {
				dropped[param], any = "", true
			} else {
				dropped[param] = slot
			}
		}
		if !any || mentions(term, "X") || !(mentions(remaining, "Z") || mentions(remaining, "U")) {
			continue
		}
		alone := true
		for param, slot := range dropped {
			alone = alone && (slot != "" || !mentions(remaining, slots[param]))
		}
		if !alone {
			continue
		}
		templates = append(templates, suggestionTemplate{iteration: name, pattern: remaining,
			params: dropped, paramsOnly: isSlot(remaining)})
	}
	return templates, nil
}
