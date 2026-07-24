package api

import (
	"strings"
	"testing"
)

// A key yaml.v2 does not recognise is ignored in silence, which is how state_width came to
// sit in every config in this repo doing nothing. These tests pin the rule that catches it,
// and pin that it stays quiet about every legitimate key of the (single, data-only) schema.

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

	t.Run("the removed extra_packages / extra_vars keys are dead", func(t *testing.T) {
		// These belonged to the deleted Go-expression code-generation path; a config that
		// still carries them must be told they read nothing rather than silently ignored.
		err := validateNoDeadKeys([]byte(`
main:
  partitions:
  - name: walk
    iteration: {type: wiener_process}
    params: {variances: [0.1]}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 1
    extra_packages: ["github.com/umbralcalc/stochadex/pkg/continuous"]
    extra_vars: [{variance: "0.1"}]
`))
		if err == nil {
			t.Fatal("expected extra_packages/extra_vars to be reported as dead keys")
		}
		if !strings.Contains(err.Error(), "extra_packages") ||
			!strings.Contains(err.Error(), "extra_vars") {
			t.Errorf("the error should name both removed keys, got: %q", err)
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
	// Every legitimate key of the (single, data-only) config schema must pass strict parsing.
	t.Run("the full data-config key surface passes", func(t *testing.T) {
		err := validateNoDeadKeys([]byte(`
main:
  partitions:
  - name: walk
    iteration: {type: wiener_process}
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
    output_condition: {type: nil}
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
