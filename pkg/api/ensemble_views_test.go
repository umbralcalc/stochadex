package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gonum.org/v1/gonum/floats"
)

var ensembleSeeds = []uint64{11, 22, 33, 44}

// ensembleViewsYAML is cfg/example_ensemble_config.yaml with its stdout view
// replaced by two per-member views: every step to run-{member}.log, and every
// 5th step to sparse-{seed}.log.
func ensembleViewsYAML(t *testing.T, dir string) string {
	t.Helper()
	yaml := replaceOnce(t, readFile(t, "../../cfg/example_ensemble_config.yaml"),
		"outputs:\n- {name: output, condition: {type: every_step}, function: {type: stdout}}\n", "")
	return yaml + fmt.Sprintf(`outputs:
- {name: log, function: {type: json_log, path: %q}}
- {name: sparse, condition: {type: every_n_steps, n: 5}, function: {type: json_log, path: %q}}
`, filepath.Join(dir, "run-{member}.log"), filepath.Join(dir, "sparse-{seed}.log"))
}

func TestEnsembleMemberViews(t *testing.T) {
	// Reference: the members as RunToStorage returns them — through the original
	// RunSeededEnsemble path, with every output suppressed.
	reference, err := RunToStorage(writeConfig(t, ensembleViewsYAML(t, t.TempDir())))
	if err != nil {
		t.Fatal(err)
	}
	if len(reference.Members) != len(ensembleSeeds) {
		t.Fatalf("expected %d reference members, got %d", len(ensembleSeeds), len(reference.Members))
	}

	t.Run("each member writes its own files, holding exactly its own run", func(t *testing.T) {
		dir := t.TempDir()
		var err error
		printed := captureStdout(t, func() {
			err = runChecked(writeConfig(t, ensembleViewsYAML(t, dir)), &SocketConfig{})
		})
		if err != nil {
			t.Fatal(err)
		}
		if printed != "" {
			t.Errorf("with outputs: declared, members go to their views, not stdout; printed %d bytes",
				len(printed))
		}
		for member, seed := range ensembleSeeds {
			storage := reference.Members[member].Storage
			assertSameEntries(t, fmt.Sprintf("member %d log", member),
				keyedEntries(t, filepath.Join(dir, fmt.Sprintf("run-%d.log", member))),
				storageEntries(storage))
			// every_n_steps(5) keeps steps 0, 5, 10, 15 and 20 of the member's run.
			want := map[string][]float64{}
			times := storage.GetTimes()
			for step, row := range storage.GetValues("growth") {
				if step%5 == 0 {
					want[fmt.Sprintf("%v/growth", times[step])] = row
				}
			}
			assertSameEntries(t, fmt.Sprintf("member %d sparse", member),
				keyedEntries(t, filepath.Join(dir, fmt.Sprintf("sparse-%d.log", seed))), want)
		}
		// The members genuinely differ, so a shared or misrouted file would show.
		first := readFile(t, filepath.Join(dir, "run-0.log"))
		if first == readFile(t, filepath.Join(dir, "run-1.log")) {
			t.Error("members 0 and 1 wrote identical logs")
		}
	})

	t.Run("RunToStorage writes no member files", func(t *testing.T) {
		dir := t.TempDir()
		for member := range ensembleSeeds {
			writeSentinel(t, filepath.Join(dir, fmt.Sprintf("run-%d.log", member)))
		}
		if _, err := RunToStorage(writeConfig(t, ensembleViewsYAML(t, dir))); err != nil {
			t.Fatal(err)
		}
		for member := range ensembleSeeds {
			assertSentinelIntact(t, filepath.Join(dir, fmt.Sprintf("run-%d.log", member)), "RunToStorage")
		}
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "sparse-") {
				t.Errorf("RunToStorage created %s", entry.Name())
			}
		}
	})

	t.Run("RunWith(WithConfigOutputs) writes the files and returns the same members", func(t *testing.T) {
		dir := t.TempDir()
		result, err := RunWith(writeConfig(t, ensembleViewsYAML(t, dir)), WithConfigOutputs())
		if err != nil {
			t.Fatal(err)
		}
		for member := range ensembleSeeds {
			if result.Members[member].Seed != ensembleSeeds[member] {
				t.Errorf("member %d has seed %d, want %d", member, result.Members[member].Seed, ensembleSeeds[member])
			}
			got, want := result.Members[member].Storage.GetValues("growth"),
				reference.Members[member].Storage.GetValues("growth")
			if len(got) != len(want) {
				t.Fatalf("member %d: %d rows, want %d", member, len(got), len(want))
			}
			for i := range want {
				if !floats.Equal(got[i], want[i]) {
					t.Fatalf("member %d row %d = %v, want %v", member, i, got[i], want[i])
				}
			}
			if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("run-%d.log", member))); err != nil {
				t.Errorf("member %d's log was not written: %v", member, err)
			}
		}
	})
}

func TestMemberPlaceholders(t *testing.T) {
	fields := map[string]interface{}{"path": "run-{member}-{seed}.log", "buffer": 64, "table": "t"}
	if !hasPlaceholder(fields, memberPlaceholders) {
		t.Error("a placeholder in a string field should be found")
	}
	if hasPlaceholder(map[string]interface{}{"path": "run.log", "buffer": 64}, memberPlaceholders) {
		t.Error("no placeholder should be found when none is present")
	}
	got := substituteFields(fields, strings.NewReplacer("{member}", "3", "{seed}", "44"))
	if got["path"] != "run-3-44.log" || got["buffer"] != 64 || got["table"] != "t" {
		t.Errorf("substitution should replace placeholders in strings and keep other fields, got %v", got)
	}
	if fields["path"] != "run-{member}-{seed}.log" {
		t.Error("substitution must not modify the config's own fields")
	}
}
