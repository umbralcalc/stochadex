package api

import (
	"fmt"
	"io"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// sortedLines splits printed output into its lines, sorted, so output written
// by concurrent partitions or members compares as a set of rows.
func sortedLines(printed string) []string {
	lines := strings.Split(strings.TrimSpace(printed), "\n")
	sort.Strings(lines)
	return lines
}

func assertSameLines(t *testing.T, label string, got, want []string) {
	t.Helper()
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%s: printed %d lines, want %d:\n%v\nvs\n%v", label, len(got), len(want), got, want)
	}
}

// noOutputYAML is serveConfigYAML (two Wiener processes, 40 steps) with no
// output declared at all.
var noOutputYAML = strings.Replace(serveConfigYAML, shorthandPair, "", 1)

func TestShorthandOutputIsOneView(t *testing.T) {
	t.Run("a config declaring no output prints every step to stdout", func(t *testing.T) {
		// This used to crash the CLI with a nil pointer.
		var err error
		printed := captureStdout(t, func() {
			err = Execute([]string{"stochadex", "--config", writeConfigPath(t, noOutputYAML)})
		})
		if err != nil {
			t.Fatalf("expected a clean run, got %v", err)
		}
		reference, err := RunToStorage(writeConfig(t, noOutputYAML))
		if err != nil {
			t.Fatal(err)
		}
		want := rowLines(reference.Storage, "")
		if len(want) != 82 {
			t.Fatalf("reference has %d rows, want 82", len(want))
		}
		assertSameLines(t, "default stdout", sortedLines(printed), want)
	})

	t.Run("the shorthand pair and the same outputs: view write the same", func(t *testing.T) {
		condition := "{type: only_given_partitions, partitions: [second_wiener_process]}"
		shorthand := replaceOnce(t, serveConfigYAML, shorthandPair,
			"    output_condition: "+condition+"\n    output_function: {type: stdout}\n")
		declared := noOutputYAML + "outputs:\n- {name: output, condition: " + condition +
			", function: {type: stdout}}\n"
		var fromShorthand, fromDeclared string
		fromShorthand = captureStdout(t, func() { Run(writeConfig(t, shorthand), &SocketConfig{}) })
		fromDeclared = captureStdout(t, func() { Run(writeConfig(t, declared), &SocketConfig{}) })
		if len(sortedLines(fromShorthand)) != 41 {
			t.Fatalf("expected 41 rows of second_wiener_process, got %d", len(sortedLines(fromShorthand)))
		}
		assertSameLines(t, "shorthand vs outputs:", sortedLines(fromShorthand), sortedLines(fromDeclared))
	})

	t.Run("an output_condition with no output_function prints what it selects", func(t *testing.T) {
		yaml := replaceOnce(t, serveConfigYAML, shorthandPair,
			"    output_condition: {type: only_given_partitions, partitions: [first_wiener_process]}\n")
		printed := captureStdout(t, func() { Run(writeConfig(t, yaml), &SocketConfig{}) })
		reference, err := RunToStorage(writeConfig(t, noOutputYAML))
		if err != nil {
			t.Fatal(err)
		}
		want := []string{}
		for _, line := range rowLines(reference.Storage, "") {
			if strings.Contains(line, " first_wiener_process ") {
				want = append(want, line)
			}
		}
		assertSameLines(t, "condition-only shorthand", sortedLines(printed), want)
	})

	t.Run("RunToStorage still records what the shorthand's condition selects", func(t *testing.T) {
		yaml := replaceOnce(t, serveConfigYAML, "output_condition: {type: every_step}",
			"output_condition: {type: only_given_partitions, partitions: [first_wiener_process]}")
		result, err := RunToStorage(writeConfig(t, yaml))
		if err != nil {
			t.Fatal(err)
		}
		if first, second := len(result.Storage.GetValues("first_wiener_process")),
			len(result.Storage.GetValues("second_wiener_process")); first != 41 || second != 0 {
			t.Errorf("recorded %d / %d rows, want 41 of the selected partition and none of the other",
				first, second)
		}
	})

	t.Run("an ensemble's shorthand json_log is written per member", func(t *testing.T) {
		dir := t.TempDir()
		yaml := fmt.Sprintf(batchConfigYAML, "{type: every_step}", filepath.Join(dir, "run-{member}.log")) +
			"run: {mode: ensemble, seeds: [5, 6]}\n"
		result, err := RunWith(writeConfig(t, yaml), WithConfigOutputs())
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Members) != 2 {
			t.Fatalf("expected 2 members, got %d", len(result.Members))
		}
		for member, run := range result.Members {
			assertSameEntries(t, fmt.Sprintf("member %d's log", member),
				keyedEntries(t, filepath.Join(dir, fmt.Sprintf("run-%d.log", member))),
				storageEntries(run.Storage))
		}
	})

	t.Run("an ensemble's stdout prefixes each row with its member", func(t *testing.T) {
		yaml := noOutputYAML + "run: {mode: ensemble, seeds: [5, 6]}\n"
		printed := captureStdout(t, func() { Run(writeConfig(t, yaml), &SocketConfig{}) })
		result, err := RunToStorage(writeConfig(t, yaml))
		if err != nil {
			t.Fatal(err)
		}
		want := []string{}
		for member, run := range result.Members {
			want = append(want, rowLines(run.Storage, fmt.Sprintf("member=%d seed=%d", member, run.Seed))...)
		}
		assertSameLines(t, "ensemble stdout", sortedLines(printed), want)
	})

	errorCases := []struct{ name, yaml, want string }{
		{"a shared shorthand sink in an ensemble",
			fmt.Sprintf(batchConfigYAML, "{type: every_step}", filepath.Join(t.TempDir(), "run.log")) +
				"run: {mode: ensemble, seeds: [5, 6]}\n",
			"main.simulation's output_function would have every ensemble member write to the same destination"},
		{"{member} in a batch run's shorthand",
			fmt.Sprintf(batchConfigYAML, "{type: every_step}", filepath.Join(t.TempDir(), "run-{member}.log")),
			"main.simulation's output_function uses {member}"},
		{"a misspelled shorthand output_condition",
			fmt.Sprintf(batchConfigYAML, "{type: every_stpe}", filepath.Join(t.TempDir(), "run.log")),
			"main.simulation's output_condition"},
		{"a shared shorthand sink in serve mode",
			fmt.Sprintf(batchConfigYAML, "{type: every_step}", filepath.Join(t.TempDir(), "run.log")) +
				serveRun(":2112"),
			"main.simulation's output_function would have every served connection write"},
	}
	for _, c := range errorCases {
		t.Run(c.name+" is a config error naming the shorthand", func(t *testing.T) {
			_, err := LoadConfig(writeConfigPath(t, c.yaml))
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}
}

func TestShorthandOutputWhenServing(t *testing.T) {
	t.Run("a shorthand connection sink serves its condition's rows", func(t *testing.T) {
		yaml := replaceOnce(t, serveConfigYAML, shorthandPair,
			"    output_condition: {type: only_given_partitions, partitions: [first_wiener_process]}\n"+
				"    output_function: {type: connection}\n") + serveRun("127.0.0.1:0")
		want := capturedRun(t, serveConfigYAML, &simulator.OnlyGivenPartitionsOutputCondition{
			Partitions: map[string]bool{"first_wiener_process": true}})
		server := servedHandler(t, yaml)
		assertMatchesReference(t, "stream", readStream(t, wsURLOf(server)), want)
	})

	t.Run("each connection's stdout view prefixes its rows with the connection", func(t *testing.T) {
		yaml := servedYAML(serveConfigYAML, "127.0.0.1:0", streamView, "{name: log, function: {type: stdout}}")
		server := servedHandler(t, yaml)
		var stream []*simulator.PartitionState
		printed := captureStdout(t, func() { stream = readStream(t, wsURLOf(server)) })
		if len(stream) != 82 {
			t.Fatalf("streamed %d rows, want 82", len(stream))
		}
		reference, err := RunToStorage(writeConfig(t, serveConfigYAML))
		if err != nil {
			t.Fatal(err)
		}
		assertSameLines(t, "connection stdout", sortedLines(printed), rowLines(reference.Storage, "connection=0"))
	})

	t.Run("--socket on a config declaring no output serves every step", func(t *testing.T) {
		aliased, err := withSocketAlias(writeConfig(t, noOutputYAML),
			&SocketConfig{Address: "127.0.0.1:0", Handle: "/"}, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		handler, err := serveHandler(aliased)
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(handler)
		defer server.Close()
		var stream []*simulator.PartitionState
		printed := captureStdout(t, func() { stream = readStream(t, wsURLOf(server)) })
		assertMatchesReference(t, "stream", stream, capturedRun(t, serveConfigYAML, nil))
		if strings.TrimSpace(printed) != "" {
			t.Errorf("the default stdout view should go to the client instead, printed %q", printed)
		}
	})
}
