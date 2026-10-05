package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"gonum.org/v1/gonum/floats"
)

// viewsMain is batchConfigYAML's partitions with the simulation block's output
// pair replaced by SIMOUTPUT, so a test can supply either the shorthand pair or
// nothing (when the config uses outputs:).
func viewsMain(simOutput string) string {
	base := fmt.Sprintf(batchConfigYAML, "{type: every_step}", "unused")
	return strings.Replace(base,
		"    output_condition: {type: every_step}\n    output_function: {type: json_log, path: \"unused\"}\n",
		simOutput, 1)
}

func shorthand(condition, path string) string {
	return fmt.Sprintf("    output_condition: %s\n    output_function: {type: json_log, path: %q}\n",
		condition, path)
}

// keyedEntries indexes a json_log's entries by (time, partition), failing on
// duplicates. Partitions output concurrently, so file order carries no meaning.
func keyedEntries(t *testing.T, path string) map[string][]float64 {
	t.Helper()
	byKey := map[string][]float64{}
	for _, entry := range readLogEntries(t, path) {
		key := fmt.Sprintf("%v/%s", entry.CumulativeTimesteps, entry.PartitionName)
		if _, dup := byKey[key]; dup {
			t.Fatalf("%s: duplicate entry %s", path, key)
		}
		byKey[key] = entry.State
	}
	return byKey
}

func assertSameEntries(t *testing.T, label string, got, want map[string][]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d entries, want %d", label, len(got), len(want))
	}
	for key, values := range want {
		if !floats.Equal(got[key], values) {
			t.Fatalf("%s: %s = %v, want %v", label, key, got[key], values)
		}
	}
}

func TestOutputViews(t *testing.T) {
	conditions := map[string]string{
		"all":    "{type: every_step}",
		"second": "{type: only_given_partitions, partitions: [second]}",
		"sparse": "{type: every_n_steps, n: 5}",
	}

	t.Run("each view receives exactly what a single-sink run with its condition writes", func(t *testing.T) {
		dir := t.TempDir()
		outputs := "outputs:\n"
		for name, condition := range conditions {
			outputs += fmt.Sprintf("- {name: %s, condition: %s, function: {type: json_log, path: %q}}\n",
				name, condition, filepath.Join(dir, name+".log"))
		}
		Run(writeConfig(t, viewsMain("")+outputs), &SocketConfig{})

		for name, condition := range conditions {
			// Reference: the same partitions and seeds, through the shorthand pair.
			referencePath := filepath.Join(dir, name+".reference.log")
			Run(writeConfig(t, viewsMain(shorthand(condition, referencePath))), &SocketConfig{})
			assertSameEntries(t, "view "+name,
				keyedEntries(t, filepath.Join(dir, name+".log")),
				keyedEntries(t, referencePath))
		}
		// The views really filter differently: all 62 rows, second's 31, and the
		// sparse view a strict subset.
		if n := len(keyedEntries(t, filepath.Join(dir, "all.log"))); n != 62 {
			t.Errorf("all view has %d entries, want 62", n)
		}
		if n := len(keyedEntries(t, filepath.Join(dir, "second.log"))); n != 31 {
			t.Errorf("second view has %d entries, want 31", n)
		}
		if n := len(keyedEntries(t, filepath.Join(dir, "sparse.log"))); n == 0 || n >= 62 {
			t.Errorf("sparse view has %d entries, want a strict non-empty subset", n)
		}
	})

	t.Run("RunToStorage writes none of the views", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "view.log")
		writeSentinel(t, path)
		config := writeConfig(t, viewsMain("")+fmt.Sprintf(
			"outputs:\n- {name: log, function: {type: json_log, path: %q}}\n", path))
		result, err := RunToStorage(config)
		if err != nil {
			t.Fatal(err)
		}
		assertSentinelIntact(t, path, "RunToStorage")
		if n := len(result.Storage.GetValues("first")); n != 31 {
			t.Errorf("RunToStorage should still record every step (31 rows), got %d", n)
		}
	})

	t.Run("a view's sink is finalized, and only gets its condition's rows", func(t *testing.T) {
		receiver, server := newPushReceiver(t)
		defer server.Close()
		url := "ws" + strings.TrimPrefix(server.URL, "http")
		dir := t.TempDir()
		config := writeConfig(t, viewsMain("")+fmt.Sprintf(`outputs:
- {name: dash, condition: {type: only_given_partitions, partitions: [first]}, function: {type: websocket, url: %q}}
- {name: log, function: {type: json_log, path: %q}}
`, url, filepath.Join(dir, "log.log")))
		Run(config, &SocketConfig{})
		receiver.waitForConnections(t, 1)
		stream := receiver.connections[0]
		if len(stream) != 31 {
			t.Fatalf("dash view got %d messages, want first's 31", len(stream))
		}
		for _, state := range stream {
			if state.PartitionName != "first" {
				t.Fatalf("dash view received %s, which its condition excludes", state.PartitionName)
			}
		}
		if receiver.closeCodes[0] != websocket.CloseNormalClosure {
			t.Errorf("the view's websocket should be closed normally by Finalize, got code %d",
				receiver.closeCodes[0])
		}
	})

	invalid := []struct {
		name, yaml, wantInError string
	}{
		{"outputs: alongside the shorthand pair",
			viewsMain(shorthand("{type: every_step}", "x.log")) +
				"outputs:\n- {name: a, function: {type: nil}}\n",
			"both outputs:"},
		{"a view without a name",
			viewsMain("") + "outputs:\n- {function: {type: nil}}\n", "needs a name"},
		{"two views with one name",
			viewsMain("") + "outputs:\n- {name: a, function: {type: nil}}\n- {name: a, function: {type: nil}}\n",
			`view "a" twice`},
		{"a view without a function",
			viewsMain("") + "outputs:\n- {name: a}\n", "needs a function"},
		{"a view with an unknown sink type",
			viewsMain("") + "outputs:\n- {name: a, function: {type: carrier_pigeon}}\n", `"a"`},
		{"outputs: with macros:",
			macroConfigYAML + "outputs:\n- {name: a, function: {type: nil}}\n", "macros:"},
	}
	for _, c := range invalid {
		t.Run(c.name+" is rejected at load", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(c.yaml), 0o644); err != nil {
				t.Fatal(err)
			}
			message := func() (message string) {
				defer func() { message = stringify(recover()) }()
				LoadApiRunConfigFromYaml(path)
				return ""
			}()
			if !strings.Contains(message, c.wantInError) {
				t.Errorf("expected a load error mentioning %q, got %q", c.wantInError, message)
			}
		})
	}
}
