package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// shardedYAML is a six-member ensemble of two partitions, each member writing
// its own log named by its seed under dir.
func shardedYAML(dir, name string) string {
	return fmt.Sprintf(`main:
  partitions:
  - {name: a, iteration: {type: wiener_process}, params: {variances: [1.0, 2.0]}, init_state_values: [0.0, 1.0], state_history_depth: 1, seed: 3}
  - {name: b, iteration: {type: wiener_process}, params: {variances: [0.5]}, init_state_values: [2.0], state_history_depth: 1, seed: 4}
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 20}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
run: {mode: ensemble, seeds: [11, 12, 13, 14, 15, 16]}
outputs:
- {name: log, function: {type: json_log, path: %q}}
`, filepath.Join(dir, name))
}

func TestEnsembleShards(t *testing.T) {
	t.Run("two shards' outputs together are the single run's, member for member", func(t *testing.T) {
		whole, shards := t.TempDir(), t.TempDir()
		if err := Execute([]string{"stochadex", "--config",
			writeConfigPath(t, shardedYAML(whole, "run-{seed}.log"))}); err != nil {
			t.Fatal(err)
		}
		sharded := writeConfigPath(t, shardedYAML(shards, "run-{seed}.log"))
		for _, shard := range []string{"11:13", "14:16"} {
			if err := Execute([]string{"stochadex", "--config", sharded, "--seed-range", shard}); err != nil {
				t.Fatal(err)
			}
		}
		files := filesUnder(t, whole)
		if len(files) != 6 || !slices.Equal(filesUnder(t, shards), files) {
			t.Fatalf("the shards wrote %v; the single run wrote %v", filesUnder(t, shards), files)
		}
		for _, file := range files {
			assertSameEntries(t, file, keyedEntries(t, filepath.Join(shards, file)),
				keyedEntries(t, filepath.Join(whole, file)))
		}
	})

	t.Run("a shard's members are the single run's members with its seeds", func(t *testing.T) {
		dir := t.TempDir()
		whole, err := RunToStorage(writeConfig(t, shardedYAML(dir, "run-{seed}.log")))
		if err != nil {
			t.Fatal(err)
		}
		bySeed := map[uint64]int{}
		for member, run := range whole.Members {
			bySeed[run.Seed] = member
		}
		for _, shard := range []string{"11:12", "13:16", "15:15"} {
			option, err := seedRangeOption(shard)
			if err != nil {
				t.Fatal(err)
			}
			config, err := LoadConfig(writeConfigPath(t, shardedYAML(dir, "run-{seed}.log")), option)
			if err != nil {
				t.Fatal(err)
			}
			result, err := RunToStorage(config)
			if err != nil {
				t.Fatal(err)
			}
			from, to, _ := strings.Cut(shard, ":")
			if first, last := result.Members[0].Seed, result.Members[len(result.Members)-1].Seed; fmt.Sprint(first) != from || fmt.Sprint(last) != to {
				t.Fatalf("shard %s ran seeds %d to %d", shard, first, last)
			}
			for _, member := range result.Members {
				assertStoragesEqual(t, fmt.Sprintf("shard %s seed %d", shard, member.Seed),
					member.Storage, whole.Members[bySeed[member.Seed]].Storage)
			}
		}
	})

	t.Run("--seed-range is --set run.seeds with the same list", func(t *testing.T) {
		pinnedBuild(t, "pinned")
		config := writeConfigPath(t, shardedYAML(t.TempDir(), "run-{seed}.log"))
		key := func(args ...string) Provenance {
			err, printed := executeIn(t, t.TempDir(), append([]string{"inspect", "--provenance", "-c", config}, args...)...)
			if err != nil {
				t.Fatal(err)
			}
			var p Provenance
			if err := json.Unmarshal([]byte(printed), &p); err != nil {
				t.Fatal(err)
			}
			return p
		}
		ranged, listed := key("--seed-range", "12:14"), key("--set", "run.seeds=[12, 13, 14]")
		if ranged.Key == "" || ranged.Key != listed.Key || !slices.Equal(ranged.Seeds, []uint64{12, 13, 14}) {
			t.Errorf("--seed-range gave key %q and seeds %v; --set gave %q", ranged.Key, ranged.Seeds, listed.Key)
		}
		if other := key("--seed-range", "15:16"); other.Key == ranged.Key {
			t.Error("two shards have the same key, so one would be skipped as the other")
		}
	})

	t.Run("each shard writes its own provenance", func(t *testing.T) {
		pinnedBuild(t, "pinned")
		dir := t.TempDir()
		config := writeConfigPath(t, shardedYAML(dir, "run-{seed}.log"))
		for _, shard := range []string{"11:13", "14:16"} {
			if err := Execute([]string{"stochadex", "--config", config, "--seed-range", shard, "--provenance"}); err != nil {
				t.Fatal(err)
			}
		}
		first, second := readProvenance(t, filepath.Join(dir, "run-11.log")), readProvenance(t, filepath.Join(dir, "run-14.log"))
		if first.Key == second.Key || !slices.Equal(first.Seeds, []uint64{11, 12, 13}) ||
			!slices.Equal(second.Seeds, []uint64{14, 15, 16}) {
			t.Errorf("shard provenance: %v %v and %v %v", first.Key, first.Seeds, second.Key, second.Seeds)
		}
		if again := readProvenance(t, filepath.Join(dir, "run-13.log")); again.Key != first.Key {
			t.Error("a shard's members carry different keys")
		}
	})

	t.Run("names by member number alone are warned about, as shards would overwrite each other", func(t *testing.T) {
		dir := t.TempDir()
		for name, warned := range map[string]bool{"run-{member}.log": true, "run-{member}-{seed}.log": false} {
			config := writeConfigPath(t, shardedYAML(dir, name))
			for _, args := range [][]string{{"--seed-range", "11:12"}, {}} {
				stderr := captureStderr(t, func() {
					if err := Execute(append([]string{"stochadex", "--config", config}, args...)); err != nil {
						t.Fatal(err)
					}
				})
				want := warned && len(args) > 0
				if got := strings.Contains(stderr, "names members by {member}"); got != want {
					t.Errorf("%s with %v: warned %v, want %v", name, args, got, want)
				}
			}
		}
	})

	errors := map[string][]string{
		"backwards":     {"--seed-range", "16:11"},
		"not numbers":   {"--seed-range", "a:b"},
		"one seed only": {"--seed-range", "11"},
		"negative":      {"--seed-range", "-1:3"},
	}
	for name, args := range errors {
		t.Run("a range that is "+name+" is a usage error", func(t *testing.T) {
			err := Execute(append([]string{"stochadex", "--config",
				writeConfigPath(t, shardedYAML(t.TempDir(), "run-{seed}.log"))}, args...))
			if KindOf(err) != ErrUsage || !strings.Contains(err.Error(), "expected FROM:TO") {
				t.Errorf("expected ErrUsage, got %v", err)
			}
		})
	}

	t.Run("a config that is not an ensemble is a config error naming --seed-range", func(t *testing.T) {
		err := Execute([]string{"stochadex", "--config", writeConfigPath(t, checkYAML(walkPartition("w", ""), "")),
			"--seed-range", "1:3"})
		if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), "--seed-range 1:3: the config has no run") {
			t.Errorf("expected ErrConfig naming the flag, got %v", err)
		}
	})

	t.Run("inspect --io shows the shard's seeds and its members' files", func(t *testing.T) {
		dir := t.TempDir()
		err, printed := executeIn(t, t.TempDir(), "inspect", "--io", "-c",
			writeConfigPath(t, shardedYAML(dir, "run-{seed}.log")), "--seed-range", "14:15")
		if err != nil {
			t.Fatal(err)
		}
		var manifest IOManifest
		if err := json.Unmarshal([]byte(printed), &manifest); err != nil {
			t.Fatal(err)
		}
		want := []string{filepath.Join(dir, "run-14.log"), filepath.Join(dir, "run-15.log")}
		if !slices.Equal(manifest.Run.Seeds, []uint64{14, 15}) || !slices.Equal(manifest.Outputs[0].Locations, want) {
			t.Errorf("seeds %v and files %v", manifest.Run.Seeds, manifest.Outputs[0].Locations)
		}
	})
}

// captureStderr runs f and returns everything it wrote to os.Stderr.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	original := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	done := make(chan string)
	go func() {
		var buffer bytes.Buffer
		io.Copy(&buffer, reader)
		done <- buffer.String()
	}()
	defer func() {
		writer.Close()
		os.Stderr = original
	}()
	f()
	writer.Close()
	os.Stderr = original
	return <-done
}

func TestRunWithParsedArgsRunsAShard(t *testing.T) {
	dir := t.TempDir()
	captureStdout(t, func() {
		RunWithParsedArgs(ParsedArgs{ConfigFile: writeConfigPath(t, shardedYAML(dir, "run-{seed}.log")),
			SeedRange: "12:13"})
	})
	if files := filesUnder(t, dir); !slices.Equal(files, []string{"run-12.log", "run-13.log"}) {
		t.Errorf("the shard wrote %v", files)
	}
}
