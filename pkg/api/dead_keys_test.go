package api

import (
	"strings"
	"testing"
)

// A key yaml.v2 does not recognise is ignored in silence, which is how state_width came to
// sit in every config in this repo doing nothing. These tests pin the rule that catches it,
// and — just as importantly — pin that it stays quiet about the keys the two views
// legitimately split between them, since that is what makes it safe to apply to both.

func TestDeadKeysAreRejected(t *testing.T) {
	t.Run("a key no view reads is reported by name", func(t *testing.T) {
		err := validateNoDeadKeys([]byte(`
main:
  partitions:
  - name: walk
    params: {drift: [0.5]}
    init_state_values: [0.0]
    state_history_depth: 1
    state_widht: 1
`))
		if err == nil {
			t.Fatal("expected a typo'd key to be rejected")
		}
		if !strings.Contains(err.Error(), "state_widht") {
			t.Errorf("the error should name the offending key, got: %q", err)
		}
	})

	t.Run("a key from a superseded schema is reported", func(t *testing.T) {
		// The shape pkg/simulator's fixture carried for years: the wiring key was split into
		// params_from_upstream_partition and params_from_indices before becoming the single
		// params_from_upstream. Neither old key errored; both did nothing.
		err := validateNoDeadKeys([]byte(`
main:
  partitions:
  - name: walk
    init_state_values: [0.0]
    params_from_upstream_partition:
      other: source
`))
		if err == nil || !strings.Contains(err.Error(), "params_from_upstream_partition") {
			t.Fatalf("expected the superseded key to be reported, got: %v", err)
		}
	})

	t.Run("every dead key is reported at once, sorted", func(t *testing.T) {
		err := validateNoDeadKeys([]byte(`
main:
  partitions:
  - name: walk
    init_state_values: [0.0]
    zebra: 1
    apple: 2
`))
		if err == nil {
			t.Fatal("expected the dead keys to be rejected")
		}
		// Sorted, so the message is stable rather than map-order dependent.
		if !strings.Contains(err.Error(), "apple, zebra") {
			t.Errorf("expected both keys reported in sorted order, got: %q", err)
		}
	})
}

func TestLegitimateKeysAreNotMistakenForDead(t *testing.T) {
	// The point of intersecting the two views. Each of these is unknown to one view and owned
	// by the other, so a naive strict parse against either would reject the file.
	t.Run("keys split across the two views all pass", func(t *testing.T) {
		err := validateNoDeadKeys([]byte(`
main:
  partitions:
  # iteration, extra_packages and extra_vars belong to the code-generation view; params,
  # init_state_values, seed and state_history_depth belong to the concrete view.
  - name: walk
    iteration: "&continuous.WienerProcessIteration{}"
    extra_packages: ["github.com/umbralcalc/stochadex/pkg/continuous"]
    extra_vars: [{variance: "0.1"}]
    params: {variances: [0.1]}
    params_as_partitions: {other: [somewhere]}
    params_from_upstream:
      inflow: {upstream: source}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 3
  expressions:
  - partition: walk
    fields: [{name: x, width: 1}]
    upstreams: {drv: driver}
    bindings: [{name: b, expr: "x + 1"}]
    outputs: ["b"]
  simulation:
    output_condition: "&simulator.NilOutputCondition{}"
embedded:
# The embedded run is inlined, so its partitions sit directly here — there is no run key.
- name: inner
  partitions:
  - name: p
    init_state_values: [0.0]
`))
		if err != nil {
			t.Errorf("legitimate keys were reported dead: %v", err)
		}
	})

	t.Run("free-form params keys are never dead", func(t *testing.T) {
		// Params is an inline map, so any key under it is data, not schema.
		err := validateNoDeadKeys([]byte(`
main:
  partitions:
  - name: walk
    init_state_values: [0.0]
    params:
      anything_at_all: [1.0]
      state_width: [7.0]
`))
		if err != nil {
			t.Errorf("param names must not be checked against the schema: %v", err)
		}
	})

	t.Run("every config file in the repo is clean", func(t *testing.T) {
		// The regression guard: this is what state_width would have tripped.
		for _, path := range []string{
			"test_program_config.yaml",
			"test_program_embedded_config.yaml",
			"test_program_expression_config.yaml",
			"test_program_expression_coupled_config.yaml",
		} {
			t.Run(path, func(t *testing.T) {
				if didPanic(func() { LoadApiRunConfigFromYaml(path) }) {
					t.Errorf("%s has a key nothing reads", path)
				}
			})
		}
	})
}
