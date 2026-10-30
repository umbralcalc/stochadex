package api

import (
	"strings"
	"testing"
)

// twoPartWindow is comparisonMacro with a second window partition, drift,
// which sim reads within the step: listed drift first the window is in order,
// sim first it is not.
func twoPartWindow(inOrder bool) string {
	drift := `    - partition:
        name: drift
        iteration: {type: wiener_process}
        params: {variances: [0.05]}
        init_state_values: [0.0]
        state_history_depth: 1
        seed: 3
`
	sim := `    - partition:
        name: sim
        iteration: {type: ornstein_uhlenbeck}
        params: {thetas: [0.5], mus: [0.0], sigmas: [0.2]}
        params_from_upstream: {mus: {upstream: drift}}
        init_state_values: [1.0]
        state_history_depth: 1
        seed: 8
`
	window := sim + drift
	if inOrder {
		window = drift + sim
	}
	start := strings.Index(comparisonMacro, "    - partition:\n")
	end := strings.Index(comparisonMacro, "    data: [{partition_name: stream}]")
	return inferenceInputs + comparisonMacro[:start] + window + comparisonMacro[end:]
}

// inlineCount is how many nested runs a config's expansion states inline.
func inlineCount(t *testing.T, yaml string) int {
	t.Helper()
	config, err := LoadConfig(writeConfigPath(t, yaml))
	if err != nil {
		t.Fatal(err)
	}
	written, err := ExpandToYAML(config)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(written), "execution_strategy: {type: inline}")
}

func TestExpandedWindowsRunAsTheMacroTierRunsThem(t *testing.T) {
	t.Run("a comparison's window in order runs inline", func(t *testing.T) {
		if n := inlineCount(t, inferenceInputs+comparisonMacro); n != 1 {
			t.Errorf("%d nested runs stated inline, want 1", n)
		}
	})

	t.Run("a fit's gradient window runs inline", func(t *testing.T) {
		if n := inlineCount(t, readFile(t, "testdata/gradient/fit_macro.yaml")); n != 1 {
			t.Errorf("%d nested runs stated inline, want 1", n)
		}
	})

	t.Run("a window listed out of order keeps the default, and scores as in order", func(t *testing.T) {
		if n := inlineCount(t, twoPartWindow(false)); n != 0 {
			t.Errorf("%d nested runs stated inline, want 0", n)
		}
		if n := inlineCount(t, twoPartWindow(true)); n != 1 {
			t.Errorf("%d nested runs stated inline in order, want 1", n)
		}
		outOfOrder, err := RunToStorage(expandedAlike(t, twoPartWindow(false)))
		if err != nil {
			t.Fatal(err)
		}
		inOrder, err := RunToStorage(expandedAlike(t, twoPartWindow(true)))
		if err != nil {
			t.Fatal(err)
		}
		// The window's row lists its partitions in their own order, so compare
		// the score, its last value.
		a, b := outOfOrder.Storage.GetValues("fit"), inOrder.Storage.GetValues("fit")
		for i := range a {
			if a[i][len(a[i])-1] != b[i][len(b[i])-1] {
				t.Fatalf("step %d: out of order scores %v, in order %v", i, a[i][len(a[i])-1], b[i][len(b[i])-1])
			}
		}
	})

	t.Run("a nested run stated inline runs inline: out of order, it is refused", func(t *testing.T) {
		config, err := LoadConfig(writeConfigPath(t, twoPartWindow(false)))
		if err != nil {
			t.Fatal(err)
		}
		written, err := ExpandToYAML(config)
		if err != nil {
			t.Fatal(err)
		}
		forced := strings.Replace(string(written), "init_time_value: 0,",
			"init_time_value: 0, execution_strategy: {type: inline},", 1)
		if forced == string(written) {
			t.Fatalf("found no nested run to force inline in:\n%s", written)
		}
		_, err = RunToStorage(writeConfig(t, forced))
		if err == nil || !strings.Contains(err.Error(), "InlineExecution requires upstreams to be ordered before their consumers") {
			t.Errorf("expected the inline strategy's ordering refusal, got %v", err)
		}
	})
}
