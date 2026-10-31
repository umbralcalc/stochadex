package api

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// posteriorSeeds are the members of the posterior ensemble.
var posteriorSeeds = []uint64{5, 6, 7, 8}

// posteriorEnsemble is cfg/example_posterior_macro_config.yaml run as an
// ensemble over posteriorSeeds, with the given outputs: views.
func posteriorEnsemble(t *testing.T, views string) string {
	t.Helper()
	seeds := make([]string, len(posteriorSeeds))
	for i, seed := range posteriorSeeds {
		seeds[i] = fmt.Sprint(seed)
	}
	return readFile(t, "../../cfg/example_posterior_macro_config.yaml") +
		"run: {mode: ensemble, seeds: [" + strings.Join(seeds, ", ") + "]}\n" + views
}

func TestMacrosConfigEnsemble(t *testing.T) {
	result, err := RunToStorage(writeConfig(t, posteriorEnsemble(t, "")))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Members) != len(posteriorSeeds) {
		t.Fatalf("got %d members, want %d", len(result.Members), len(posteriorSeeds))
	}
	final := func(member int) []float64 {
		means := result.Members[member].Storage.GetValues("test_post_mean")
		return means[len(means)-1]
	}

	t.Run("each member estimates the posterior under its own seed", func(t *testing.T) {
		seen := map[string]int{}
		for member, seed := range posteriorSeeds {
			if result.Members[member].Seed != seed {
				t.Errorf("member %d has seed %d, want %d", member, result.Members[member].Seed, seed)
			}
			key := fmt.Sprint(final(member))
			if other, ok := seen[key]; ok {
				t.Errorf("members %d and %d end on the same estimate %s", other, member, key)
			}
			seen[key] = member
		}
	})

	t.Run("the members' mean estimate is near the data's true mean", func(t *testing.T) {
		truth := []float64{1.8, 5.0}
		for i := range truth {
			sum := 0.0
			for member := range posteriorSeeds {
				sum += final(member)[i]
			}
			// The prior is [0, 0], so an unmoved posterior fails this.
			if mean := sum / float64(len(posteriorSeeds)); math.Abs(mean-truth[i]) > 0.5 {
				t.Errorf("coordinate %d: ensemble mean %v, want within 0.5 of %v", i, mean, truth[i])
			}
		}
	})

	t.Run("each member is the run of the written expansion under its seed", func(t *testing.T) {
		// Reference: the expansion written out as core-language YAML, run as an
		// ordinary ensemble.
		loaded, err := LoadConfig(writeConfigPath(t, posteriorEnsemble(t, "")))
		if err != nil {
			t.Fatal(err)
		}
		written, err := ExpandToYAML(loaded)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(written), "mode: ensemble") {
			t.Fatalf("the expansion does not carry the run: block:\n%s", written)
		}
		reference, err := RunToStorage(writeConfig(t, string(written)))
		if err != nil {
			t.Fatal(err)
		}
		for member := range posteriorSeeds {
			assertStoragesEqual(t, fmt.Sprintf("member %d", member),
				result.Members[member].Storage, reference.Members[member].Storage)
		}
	})

	t.Run("the CLI writes each member's own outputs", func(t *testing.T) {
		dir := t.TempDir()
		path := writeConfigPath(t, posteriorEnsemble(t, fmt.Sprintf(
			"outputs:\n- {name: log, condition: {type: only_given_partitions, partitions: [test_post_mean]}, function: {type: json_log, path: %q}}\n",
			filepath.Join(dir, "member-{member}.log"))))
		if err, _ := executeIn(t, t.TempDir(), "--config", path); err != nil {
			t.Fatal(err)
		}
		for member := range posteriorSeeds {
			logged := keyedEntries(t, filepath.Join(dir, fmt.Sprintf("member-%d.log", member)))
			storage := result.Members[member].Storage
			want := map[string][]float64{}
			times := storage.GetTimes()
			for step, row := range storage.GetValues("test_post_mean") {
				want[fmt.Sprintf("%v/test_post_mean", times[step])] = row
			}
			assertSameEntries(t, fmt.Sprintf("member %d log", member), logged, want)
		}
		if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("member-%d.log", len(posteriorSeeds)))); err == nil {
			t.Error("a log was written for a member the ensemble does not have")
		}
	})
}

func TestMacrosConfigServe(t *testing.T) {
	// A shorter run of the posterior example, so the stream stays small.
	batch := strings.Replace(readFile(t, "../../cfg/example_posterior_macro_config.yaml"),
		"steps: 2000", "steps: 300", 1)
	want := capturedRun(t, batch, nil)
	if len(want) == 0 {
		t.Fatal("the batch run recorded nothing")
	}
	served := batch + serveRun("127.0.0.1:0") + "outputs:\n- " + streamView + "\n"

	t.Run("a served macros config streams its batch run", func(t *testing.T) {
		server := servedHandler(t, served)
		assertMatchesReference(t, "served", readStream(t, wsURLOf(server)), want)
	})

	t.Run("each connection re-runs it from the start", func(t *testing.T) {
		server := servedHandler(t, served)
		readStream(t, wsURLOf(server))
		assertMatchesReference(t, "second connection", readStream(t, wsURLOf(server)), want)
	})

	t.Run("the CLI serves it", func(t *testing.T) {
		address := freeAddress(t)
		executeServing(t, address, "--config", writeConfigPath(t,
			strings.Replace(served, "127.0.0.1:0", address, 1)))
		assertMatchesReference(t, "served", readStream(t, "ws://"+address+"/handle"), want)
	})

	t.Run("one whose input cannot be read is not served", func(t *testing.T) {
		config := writeConfig(t, strings.Replace(readFile(t, "../../cfg/example_data_source_config.yaml"),
			"cfg/example_data.csv", "cfg/absent.csv", 1)+serveRun("127.0.0.1:0")+
			"outputs:\n- "+streamView+"\n")
		if _, err := serveHandler(config); KindOf(err) != ErrUnavailable {
			t.Errorf("expected ErrUnavailable, got %v", err)
		}
	})
}
