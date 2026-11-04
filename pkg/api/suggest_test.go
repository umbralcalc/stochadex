package api

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"path/filepath"
	"strings"
	"testing"

	"gonum.org/v1/gonum/floats"
)

// suggestConfig is a config whose last partition is out, given as its YAML entry, after a
// partition rate (2 + sin(t), width 2) it may read, stepping 0.37 for 120 steps.
func suggestConfig(out string) string {
	return `main:
  partitions:
  - name: rate
    iteration: {type: expression, fields: [{name: r, width: 2}], outputs: ["2 + sin(t) + 0 * r"]}
    init_state_values: [2, 2]
    state_history_depth: 1
    seed: 0
` + out + `  simulation:
    termination_condition: {type: number_of_steps, max_steps: 120}
    timestep_function: {type: constant, stepsize: 0.37}
    init_time_value: 0
`
}

// expressionPartition is out as an expression partition of width w.
func expressionPartition(w int, update, params, wiring string, initial float64) string {
	entry := fmt.Sprintf("  - name: out\n    iteration: {type: expression, fields: [{name: x, width: %d}], outputs: [%q]}\n",
		w, update)
	if params != "" {
		entry += "    params: {" + params + "}\n"
	}
	return entry + wiring + fmt.Sprintf("    init_state_values: %s\n    state_history_depth: 1\n    seed: 11\n",
		repeatedList(initial, w))
}

// suggestedPartitions indents a suggestion's YAML to sit in main.partitions.
func suggestedPartitions(yaml string) string {
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(yaml, "\n"), "\n") {
		lines = append(lines, "  "+line)
	}
	return strings.Join(lines, "\n") + "\n"
}

// twinsAgree reports whether two runs of a partition agree at every step to 1e-9, and
// whether the partition moved, without which agreeing shows little.
func twinsAgree(got, want [][]float64) (agree, moved bool) {
	if len(got) != len(want) {
		return false, false
	}
	for step := range want {
		if !floats.EqualApprox(got[step], want[step], 1e-9) {
			return false, false
		}
		moved = moved || !floats.Equal(want[step], want[0])
	}
	return true, moved
}

func TestSuggest(t *testing.T) {
	for _, c := range []struct {
		name, out, want string
	}{
		{"a Wiener step", expressionPartition(3, "x + sqrt(variances * dt) * iid(3, normal(0, 1))",
			"variances: [1, 2, 3]", "", 0), "wiener_process"},
		{"a Wiener step at a stated variance", expressionPartition(2, "x + sqrt(2 * dt) * iid(2, normal(0, 1))",
			"", "", 0), "wiener_process"},
		{"a Wiener step started from its upstream's first row", expressionPartition(2,
			"x + sqrt(variances * dt) * iid(2, normal(0, 1))", "variances: [1, 2], v: [0, 0]",
			"    params_from_upstream: {v: {upstream: rate}}\n    init_state_from_upstream: v\n", 0), "wiener_process"},
		{"a Wiener step drawn at its scale", expressionPartition(3, "x + normal(0, sqrt(variances * dt))",
			"variances: [1, 2, 3]", "", 0), "wiener_process"},
		{"a geometric Brownian motion", expressionPartition(2, "x * (1 + sqrt(v * dt) * iid(2, normal(0, 1)))",
			"v: [0.01, 0.04]", "", 1), "geometric_brownian_motion"},
		{"a geometric Brownian motion drawn at its scale", expressionPartition(2, "x * (1 + normal(0, sqrt(v * dt)))",
			"v: [0.01, 0.04]", "", 1), "geometric_brownian_motion"},
		{"a drift-diffusion with its own param names, in brackets", expressionPartition(1,
			"x + (mu * dt + sd * sqrt(dt) * shared(normal(0, 1)))", "mu: [0.2], sd: [0.3]", "", 1), "drift_diffusion"},
		{"a drift-diffusion drawn at its scale", expressionPartition(2, "x + mu * dt + normal(0, sd * sqrt(dt))",
			"mu: [0.2, 0.1], sd: [0.3, 0.5]", "", 1), "drift_diffusion"},
		{"a driftless diffusion", expressionPartition(2, "x + sd * sqrt(dt) * iid(2, normal(0, 1))",
			"sd: [0.3, 0.5]", "", 0), "drift_diffusion"},
		{"an Ornstein-Uhlenbeck step with its terms in another order", expressionPartition(2,
			"sigmas * sqrt(dt) * iid(2, normal(0, 1)) + dt * (mus - x) * thetas + x",
			"thetas: [1, 2], mus: [3, 4], sigmas: [0.5, 0.2]", "", 0), "ornstein_uhlenbeck"},
		{"an Ornstein-Uhlenbeck step drawn at its scale", expressionPartition(1,
			"x + thetas * (mus - x) * dt + shared(normal(0, sigmas * sqrt(dt)))",
			"thetas: [0.3], mus: [0], sigmas: [0.5]", "", 0), "ornstein_uhlenbeck"},
		{"an Ornstein-Uhlenbeck step stated through bindings", `  - name: out
    iteration:
      type: expression
      fields: [{name: x}]
      bindings: [{name: pull, expr: "thetas * (mus - x) * dt"}, {name: kick, expr: "sigmas * sqrt(dt) * shared(normal(0, 1))"}]
      outputs: ["x + pull + kick"]
    params: {thetas: [1], mus: [2], sigmas: [0.5]}
    init_state_values: [0]
    state_history_depth: 1
    seed: 11
`, "ornstein_uhlenbeck"},
		{"a Cox process whose rate comes from upstream", expressionPartition(2,
			"x + where(rates > (rates + 1 / dt) * iid(2, uniform(0, 1)), 1, 0)", "rates: [0, 0]",
			"    params_from_upstream: {rates: {upstream: rate}}\n", 0), "poisson_process"},
		{"a Bernoulli draw", expressionPartition(2, "where(iid(2, uniform(0, 1)) < p, 1, 0)", "p: [0.3, 0.8]", "", 0),
			"bernoulli_process"},
		{"a clock", expressionPartition(1, "t + dt", "", "", 0), "cumulative_time"},
		{"a Poisson step with its probability written as a quotient", expressionPartition(2,
			"x + where(rates / (rates + 1 / dt) > iid(2, uniform(0, 1)), 1, 0)", "rates: [0.5, 2]", "", 0),
			"poisson_process"},
		{"an Ornstein-Uhlenbeck step with its draw first", expressionPartition(1,
			"x + thetas * (mus - x) * dt + shared(normal(0, 1)) * sigmas * sqrt(dt)", "thetas: [0.3], mus: [1], sigmas: [0.5]", "", 0),
			"ornstein_uhlenbeck"},
		{"a param copied out", expressionPartition(2, "level", "level: [4, 5]", "", 0), "param_values"},
		{"a constant", expressionPartition(2, "x", "", "", 3), "constant_values"},
	} {
		t.Run(c.name, func(t *testing.T) {
			original := suggestConfig(c.out)
			suggestions, err := Suggest(writeConfig(t, original))
			if err != nil {
				t.Fatal(err)
			}
			if len(suggestions) != 1 || suggestions[0].Partition != "out" || suggestions[0].Iteration != c.want {
				t.Fatalf("got %+v, want out as %s", suggestions, c.want)
			}
			if !strings.Contains(suggestions[0].YAML, "type: "+c.want) {
				t.Errorf("the replacement does not name %s:\n%s", c.want, suggestions[0].YAML)
			}
			// Reference: the config as written, and the config with the replacement pasted
			// in, run in full.
			want := runToStorageT(t, original).GetValues("out")
			got := runToStorageT(t, suggestConfig(suggestedPartitions(suggestions[0].YAML))).GetValues("out")
			agree, moved := twinsAgree(got, want)
			if len(got) != 121 || !agree || (!moved && c.want != "constant_values") {
				t.Errorf("the replacement gives %v, the expression %v", got, want)
			}
		})
	}

	for _, c := range []struct {
		name, out string
		want      []string
	}{
		{"a rate written into the draw", expressionPartition(2,
			"x + where(iid(2, uniform(0, 1)) * (2 + sin(t) + 1 / dt) < 2 + sin(t), 1, 0)", "", "", 0), nil},
		{"a rate read from an upstream's history and params", `  - name: out
    iteration:
      type: expression
      fields: [{name: x, width: 2}]
      upstreams: {r: rate}
      outputs: ["x + where(iid(2, uniform(0, 1)) * (0.5 * b + r / 2 + 1 / dt) < 0.5 * b + r / 2, 1, 0)"]
    params: {b: [0, 0]}
    params_from_upstream: {b: {upstream: rate}}
    init_state_values: [0, 0]
    state_history_depth: 1
    seed: 11
`, []string{"upstreams: {r: rate}", "b: {upstream: rate}"}},
		{"a rate scaling a param of the built-in's param's name", expressionPartition(2,
			"x + where(iid(2, uniform(0, 1)) * (2 * rates + 1 / dt) < 2 * rates, 1, 0)", "rates: [1, 0.5]", "", 0),
			[]string{"name: rates_"}},
	} {
		t.Run(c.name+" is factored out into a partition of its own", func(t *testing.T) {
			original := suggestConfig(c.out)
			suggestions, err := Suggest(writeConfig(t, original))
			if err != nil {
				t.Fatal(err)
			}
			if len(suggestions) != 1 || suggestions[0].Iteration != "poisson_process" {
				t.Fatalf("got %+v", suggestions)
			}
			yaml := suggestions[0].YAML
			for _, want := range append([]string{"- name: out_rates\n", "type: expression", "- name: out\n",
				"type: poisson_process", "upstream: out_rates"}, c.want...) {
				if !strings.Contains(yaml, want) {
					t.Errorf("want %q in\n%s", want, yaml)
				}
			}
			want := runToStorageT(t, original).GetValues("out")
			got := runToStorageT(t, suggestConfig(suggestedPartitions(yaml))).GetValues("out")
			if agree, moved := twinsAgree(got, want); !agree || !moved {
				t.Errorf("the replacement gives %v, the expression %v", got, want)
			}
		})
	}

	t.Run("a replacement whose factored partition's name is taken is not suggested", func(t *testing.T) {
		config := suggestConfig(expressionPartition(2,
			"x + where(iid(2, uniform(0, 1)) * (2 + sin(t) + 1 / dt) < 2 + sin(t), 1, 0)", "", "", 0) +
			"  - {name: out_rates, iteration: {type: constant_values}, init_state_values: [1], " +
			"state_history_depth: 1, seed: 0}\n")
		suggestions, err := Suggest(writeConfig(t, config))
		if err != nil {
			t.Fatal(err)
		}
		if len(suggestions) != 0 {
			t.Errorf("got %+v", suggestions)
		}
	})

	for _, c := range []struct {
		name, out string
		noticed   bool
	}{
		{"a draw shared across the field, which the built-in draws for each element",
			expressionPartition(3, "x + sqrt(variances * dt) * shared(normal(0, 1))", "variances: [1, 2, 3]", "", 0),
			true},
		{"a pull away from the mean", expressionPartition(1,
			"x + thetas * (mus + x) * dt + sigmas * sqrt(dt) * shared(normal(0, 1))",
			"thetas: [1], mus: [2], sigmas: [0.5]", "", 0), false},
		{"a uniform draw for a normal one", expressionPartition(1, "x + sqrt(v * dt) * shared(uniform(0, 1))",
			"v: [1]", "", 0), false},
		{"a draw in a variance", expressionPartition(1, "x + sqrt(shared(uniform(1, 2)) * dt) * shared(normal(0, 1))",
			"", "", 0), false},
		{"a rate that is not the threshold", expressionPartition(1,
			"x + where(shared(uniform(0, 1)) * (rates + 1 / dt) < 2 * rates, 1, 0)", "rates: [1]", "", 0), false},
		{"a rate named like a slot that is not the threshold", expressionPartition(1,
			"x + where(shared(uniform(0, 1)) * (K + 1 / dt) < 2 * K, 1, 0)", "K: [1]", "", 0), false},
		{"a draw off zero", expressionPartition(1, "x + sqrt(v * dt) * shared(normal(1, 1))", "v: [1]", "", 0), false},
		{"a term the built-in has not", expressionPartition(1, "x + sqrt(v * dt) * shared(normal(0, 1)) + 0.1",
			"v: [1]", "", 0), false},
		{"a scale that reads the state", expressionPartition(1, "x + sqrt(x * dt) * shared(normal(0, 1))", "", "", 1), false},
		{"a variance that is the timestep", expressionPartition(1, "x + sqrt(dt * dt) * shared(normal(0, 1))", "", "", 0), false},
		{"a param scaled, which param_values cannot state", expressionPartition(1, "2 * level", "level: [4]", "", 0), false},
		{"a non-standard draw", expressionPartition(1, "x + sqrt(v * dt) * shared(normal(0, 2))", "v: [1]", "", 0), false},
		{"two fields", `  - name: out
    iteration: {type: expression, fields: [{name: a}, {name: b}], outputs: ["a", "b"]}
    init_state_values: [1, 2]
    state_history_depth: 1
    seed: 11
`, false},
	} {
		t.Run(c.name+" is not suggested", func(t *testing.T) {
			config := writeConfig(t, suggestConfig(c.out))
			if notices := suggestionNotices(config); (len(notices) > 0) != c.noticed {
				t.Errorf("notices %q", notices)
			}
			suggestions, err := Suggest(config)
			if err != nil {
				t.Fatal(err)
			}
			for _, suggestion := range suggestions {
				if suggestion.Partition == "out" {
					t.Errorf("got %+v", suggestion)
				}
			}
		})
	}

	t.Run("the expressions: block form is replaced, its entry removed", func(t *testing.T) {
		config := `main:
  partitions:
  - name: walk
    params: {variances: [2]}
    init_state_values: [0]
    state_history_depth: 1
    seed: 3
  - name: level
    init_state_values: [4]
    state_history_depth: 1
    seed: 0
  expressions:
  - partition: walk
    fields: [{name: w}]
    outputs: ["w + sqrt(variances * dt) * shared(normal(0, 1))"]
  - partition: level
    fields: [{name: l}]
    outputs: ["l"]
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 200}
    timestep_function: {type: constant, stepsize: 1}
    init_time_value: 0
`
		suggestions, err := Suggest(writeConfig(t, config))
		if err != nil {
			t.Fatal(err)
		}
		if len(suggestions) != 2 || suggestions[0].Iteration != "wiener_process" ||
			suggestions[1].Iteration != "constant_values" {
			t.Fatalf("got %+v", suggestions)
		}
	})

	t.Run("a macros config's main: partitions are checked through its expansion", func(t *testing.T) {
		config := strings.Replace(walkMain("", "- {type: vector_mean, name: m, data: {partition_name: walk}, window: 5}\n"),
			"{name: walk, iteration: {type: wiener_process}, params: {variances: [1.0]}",
			`{name: walk, iteration: {type: expression, fields: [{name: w}], outputs: ["w + sqrt(variances * dt) * shared(normal(0, 1))"]}, params: {variances: [1.0]}`, 1)
		suggestions, err := Suggest(writeConfig(t, config))
		if err != nil {
			t.Fatal(err)
		}
		if len(suggestions) != 1 || suggestions[0].Partition != "walk" {
			t.Fatalf("got %+v", suggestions)
		}
	})

	t.Run("a served config is checked as one run", func(t *testing.T) {
		config := suggestConfig(expressionPartition(1, "x + sqrt(v * dt) * shared(normal(0, 1))", "v: [2]", "", 0)) +
			"outputs:\n  - {name: stream, function: {type: connection}}\n" +
			"run:\n  mode: serve\n  websocket: {address: \":2112\", handle: /handle}\n"
		suggestions, err := Suggest(writeConfig(t, config))
		if err != nil {
			t.Fatal(err)
		}
		if len(suggestions) != 1 || suggestions[0].Iteration != "wiener_process" {
			t.Fatalf("got %+v", suggestions)
		}
	})

	t.Run("a config not loaded from text is refused", func(t *testing.T) {
		config := writeConfig(t, suggestConfig(expressionPartition(1, "x", "", "", 1)))
		config.source = nil
		if _, err := Suggest(config); KindOf(err) != ErrConfig {
			t.Errorf("got %v", err)
		}
	})

	t.Run("a config that does not run gives no suggestion, and says why", func(t *testing.T) {
		config := "inputs:\n  obs: {source: {csv: {path: absent.csv, time_column: 0, state_columns: {flow: [1]}}}}\n" +
			suggestConfig(expressionPartition(1, "x", "", "", 1)+
				"  - {name: flow, iteration: {type: from_input, input: obs}, init_state_values: [0], "+
				"state_history_depth: 1, seed: 0}\n")
		suggestions, err := Suggest(writeConfig(t, config))
		if KindOf(err) != ErrUnavailable || suggestions != nil {
			t.Errorf("got %v, %v", suggestions, err)
		}
	})
}

func TestSuggestCommand(t *testing.T) {
	path := writeConfigPath(t, suggestConfig(expressionPartition(3,
		"x + sqrt(variances * dt) * iid(3, normal(0, 1))", "variances: [1, 2, 3]", "", 0)))

	t.Run("inspect --suggest prints the suggestions as JSON", func(t *testing.T) {
		err, printed := executeIn(t, t.TempDir(), "inspect", "--suggest", "-c", path)
		if err != nil {
			t.Fatal(err)
		}
		var suggestions []Suggestion
		if err := json.Unmarshal([]byte(printed), &suggestions); err != nil {
			t.Fatalf("%v in %q", err, printed)
		}
		want, err := Suggest(writeConfig(t, readFile(t, path)))
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(suggestions) != fmt.Sprint(want) || len(want) != 1 {
			t.Errorf("printed %+v, want %+v", suggestions, want)
		}
	})

	t.Run("--check notes the match, without running", func(t *testing.T) {
		var err error
		stderr := captureStderr(t, func() {
			err, _ = executeIn(t, t.TempDir(), "--config", path, "--check")
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stderr, `partition "out" looks like the built-in wiener_process`) ||
			!strings.Contains(stderr, "is valid") {
			t.Errorf("stderr: %s", stderr)
		}
	})

	t.Run("inspect takes exactly one of its modes", func(t *testing.T) {
		for _, args := range [][]string{{"inspect", "-c", path}, {"inspect", "--io", "--suggest", "-c", path}} {
			if err, _ := executeIn(t, t.TempDir(), args...); KindOf(err) != ErrUsage {
				t.Errorf("%v: got %v", args, err)
			}
		}
	})

	t.Run("a config that cannot expand is not suggested on", func(t *testing.T) {
		missing := writeConfigPath(t, strings.Replace(readFile(t, "../../cfg/example_data_source_config.yaml"),
			"cfg/example_data.csv", "cfg/absent.csv", 1))
		if err, _ := executeIn(t, t.TempDir(), "inspect", "--suggest", "-c", missing); KindOf(err) != ErrUnavailable {
			t.Errorf("got %v", err)
		}
	})
}

func TestSuggestionMatching(t *testing.T) {
	for _, c := range []struct{ expression, want string }{
		{"-a + +b", "(-(a)) + (b)"},
		{"a >= b", "(b) <= (a)"},
		{"a > b", "(b) < (a)"},
		{"a == b / c", "(a) == ((b) / (c))"},
		{"math.f(a, 'q')", "math.f(a, 'q')"},
		{"normal(m, s)", "(m) + ((s) * (normal(0, 1)))"},
		{"normal(0, s)", "(s) * (normal(0, 1))"},
		{"a < b / c", "((a) * (c)) < (b)"},
		{"b / c <= a", "(b) <= ((a) * (c))"},
	} {
		t.Run(c.expression+" is read in order-free form", func(t *testing.T) {
			parsed, err := parser.ParseExpr(c.expression)
			if err != nil {
				t.Fatal(err)
			}
			if got := nodeSource(canonical(parsed)); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}

	for _, c := range []struct {
		a, b string
		same bool
	}{
		{"a + b * c", "c * b + a", true},
		{"a + a", "a + b", false},
		{"f(a)", "g(a)", false},
		{"f(a, b)", "f(b, a)", false},
		{"K", "2 * K", false},
	} {
		t.Run(c.a+" and "+c.b+" are compared as expressions", func(t *testing.T) {
			a, _ := parser.ParseExpr(c.a)
			b, _ := parser.ParseExpr(c.b)
			if sameNode(canonical(a), canonical(b)) != c.same || sameNode(canonical(b), canonical(a)) != c.same {
				t.Errorf("want same %v", c.same)
			}
		})
	}

	t.Run("a binding read under a sign is inlined there", func(t *testing.T) {
		output, _ := parser.ParseExpr("x + -k")
		binding, _ := parser.ParseExpr("s * z")
		if got := nodeSource(canonical(substitute(output, "k", binding))); got != "(x) + (-((s) * (z)))" {
			t.Errorf("got %q", got)
		}
	})

	for _, c := range []struct {
		name      string
		want, got [][]float64
	}{
		{"no rows", nil, nil},
		{"fewer rows", [][]float64{{1}, {2}}, [][]float64{{1}}},
		{"another width", [][]float64{{1}, {2}}, [][]float64{{1}, {2, 3}}},
		{"a value apart", [][]float64{{1}, {200}}, [][]float64{{1}, {200.001}}},
	} {
		t.Run("runs with "+c.name+" do not agree", func(t *testing.T) {
			if sameRows(c.want, c.got) {
				t.Error("they agree")
			}
		})
	}
	t.Run("runs apart by rounding agree", func(t *testing.T) {
		if !sameRows([][]float64{{1}, {200}}, [][]float64{{1}, {200 * (1 + 1e-12)}}) {
			t.Error("they do not agree")
		}
	})
}

// TestSuggestOnCatalogueTwins runs Suggest over every catalogue model's twins. A
// declarative twin is the model with every partition an expression, and a building-block
// twin the same model with built-ins wherever one fits, chosen by hand, so the suggestions
// for the first should be the built-ins of the second, and the second should have none.
// (trywizard's building-block twin uses cox_process where poisson_process is suggested: the
// two compute the same update.)
func TestSuggestOnCatalogueTwins(t *testing.T) {
	want := map[string][]string{
		"homark/declarative.yaml": {"bank_rate ornstein_uhlenbeck", "log_earnings drift_diffusion",
			"log_price drift_diffusion"},
		"trywizard/declarative.yaml": {"baseline_rates param_values", "score_events poisson_process",
			"card_events poisson_process"},
		"bathing-water-forecaster/declarative.yaml": {"anomaly ornstein_uhlenbeck"},
	}
	files, err := filepath.Glob(filepath.Join(repoDir, "models", "*", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	twins := 0
	for _, file := range files {
		name := filepath.Base(filepath.Dir(file)) + "/" + filepath.Base(file)
		if !strings.HasSuffix(name, "/declarative.yaml") && !strings.HasSuffix(name, "/building_blocks.yaml") {
			continue
		}
		twins++
		t.Run(name, func(t *testing.T) {
			text := readFile(t, file)
			// A twin's caller sets its clock; it is given one here.
			if !strings.Contains(text, "\n  simulation:") {
				text += "  simulation:\n    termination_condition: {type: number_of_steps, max_steps: 50}\n" +
					"    timestep_function: {type: constant, stepsize: 0.37}\n    init_time_value: 0\n"
			}
			suggestions, err := Suggest(writeConfig(t, text))
			if err != nil {
				t.Fatal(err)
			}
			got := []string{}
			for _, suggestion := range suggestions {
				got = append(got, suggestion.Partition+" "+suggestion.Iteration)
			}
			if fmt.Sprint(got) != fmt.Sprint(want[name]) {
				t.Errorf("got %q, want %q", got, want[name])
			}
		})
	}
	if twins < 14 {
		t.Errorf("found %d twins, want at least 14", twins)
	}
}

// TestSuggestionTemplateCoverage holds the templates to the catalogue: every built-in
// iteration's rule translates into a template, or the built-in is an alias of one whose
// rule is the same, or it is excluded with a reason and its rule does not translate.
func TestSuggestionTemplateCoverage(t *testing.T) {
	described := catalogueFields()["iterations"]
	patterns := map[string]*exprNode{}
	for _, name := range sortedKeys(described) {
		templates, err := templatesFromRule(name, described[name])
		_, aliased := suggestionAliases[name]
		reason, excluded := suggestionExclusions[name]
		switch {
		case aliased && excluded:
			t.Errorf("%s is both an alias and excluded", name)
		case excluded && err == nil:
			t.Errorf("%s is excluded (%s), but its rule translates: suggest it", name, reason)
		case !excluded && err != nil:
			t.Errorf("%s: %v; state its rule as a template, or exclude it with a reason", name, err)
		case err == nil:
			patterns[name] = templates[0].pattern
		}
	}
	for alias, name := range suggestionAliases {
		if patterns[alias] == nil || patterns[name] == nil || !sameNode(patterns[alias], patterns[name]) {
			t.Errorf("%s's rule is not %s's", alias, name)
		}
	}
	for name := range suggestionExclusions {
		if _, ok := described[name]; !ok {
			t.Errorf("%s is excluded, but is not in the catalogue", name)
		}
	}
	suggested := map[string]bool{}
	for _, template := range suggestionTemplates() {
		suggested[template.iteration] = true
	}
	if len(suggested)+len(suggestionAliases)+len(suggestionExclusions) != len(described) || len(suggested) < 9 {
		t.Errorf("%d suggested, %d aliased and %d excluded of %d", len(suggested), len(suggestionAliases),
			len(suggestionExclusions), len(described))
	}
}

func TestTemplatesFromRule(t *testing.T) {
	entry := func(update string, params ...string) CatalogueEntry {
		described := CatalogueEntry{Update: update}
		for _, param := range params {
			described.Params = append(described.Params, CatalogueField{Name: param})
		}
		return described
	}
	for _, c := range []struct {
		name  string
		entry CatalogueEntry
		want  string
	}{
		{"a rule of two lines", entry("x[i] <- a[i]\nwhen b"), "more than one line"},
		{"a rule for something other than x", entry("y <- 1"), "does not give x"},
		{"a draw other than a standard one", entry("x[i] <- G[i],  G[i] ~ Gamma(1, 2)"), "it draws"},
		{"a read of history", entry("x[i] <- x@2[i]"), "elements or history"},
		{"a read of another element", entry("x[i] <- x[i+1]"), "elements or history"},
		{"text", entry("x <- the next row"), "not an expression"},
		{"a name that is not a param", entry("x[i] <- x[i] + k[i]", "a"), "reads k"},
		{"more params than slots", CatalogueEntry{}, ""},
	} {
		t.Run(c.name+" does not translate", func(t *testing.T) {
			if c.name == "more params than slots" {
				// 24 distinct names, one more than there are slot letters.
				var names []string
				for index := 0; index < 24; index++ {
					names = append(names, fmt.Sprintf("p%d", index))
				}
				c.entry = entry("x[i] <- "+strings.Join(names, " + "), names...)
				c.want = "more params than a template has slots"
			}
			if _, err := templatesFromRule("test", c.entry); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %v, want %q", err, c.want)
			}
		})
	}

	for _, c := range []struct {
		name, update string
		params       []string
		want         []string
	}{
		{"a drift term is dropped, its param set to 0", "x[i] <- x[i] + a[i] * dt + b[i] * Z[i],  Z[i] ~ N(0, 1)",
			[]string{"a", "b"}, []string{"(X) + ((A) * (dt)) + ((B) * (Z)) map[a:A b:B]", "(X) + ((B) * (Z)) map[a: b:B]"}},
		{"a term whose param the rest reads stays", "x[i] <- x[i] + a[i] * dt + a[i] * Z[i],  Z[i] ~ N(0, 1)",
			[]string{"a"}, []string{"(X) + ((A) * (dt)) + ((A) * (Z)) map[a:A]"}},
		{"a term leaving nothing drawn stays", "x[i] <- x[i] + a[i] * U[i],  U[i] ~ U(0, 1)",
			[]string{"a"}, []string{"(X) + ((A) * (U)) map[a:A]"}},
		{"a term reading the state stays", "x[i] <- x[i] * a[i] + b[i] * Z[i],  Z[i] ~ N(0, 1)",
			[]string{"a", "b"}, []string{"((X) * (A)) + ((B) * (Z)) map[a:A b:B]"}},
		{"a term with no params stays", "x[i] <- x[i] + dt + Z[i],  Z[i] ~ N(0, 1)",
			nil, []string{"(X) + (dt) + (Z) map[]"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			templates, err := templatesFromRule("test", entry(c.update, c.params...))
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, template := range templates {
				got = append(got, nodeSource(template.pattern)+" "+fmt.Sprint(template.params))
			}
			if fmt.Sprint(got) != fmt.Sprint(c.want) {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
