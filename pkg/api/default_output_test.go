package api

import (
	"fmt"
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
var noOutputYAML = strings.Replace(serveConfigYAML, quietOutput, "", 1)

func TestDefaultOutput(t *testing.T) {
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

	t.Run("an ensemble's stdout prefixes each row with its member", func(t *testing.T) {
		yaml := noOutputYAML + "run: {mode: ensemble, seeds: [5, 6]}\n"
		printed := captureStdout(t, func() { Run(writeConfig(t, yaml)) })
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
}
