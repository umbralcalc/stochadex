package api

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// updateGoldens rewrites this architecture's expansion goldens from the run.
var updateGoldens = flag.Bool("update-goldens", false,
	"rewrite pkg/api/testdata/golden/expansion_<GOARCH>.json from this run")

// expansionGoldens are the expanded runs' outputs, for the Go version that
// recorded them (a compiler can change floating-point results, so a mismatch
// after a Go upgrade may be that), keyed by config path.
type expansionGoldens struct {
	Go      string                   `json:"go"`
	Configs map[string]storageDigest `json:"configs"`
}

// storageDigest pins a storage's every bit: its times, and each partition's
// rows, as "<rows>x<width> <sha256 of the float64 bits>".
type storageDigest struct {
	Times      string            `json:"times"`
	Partitions map[string]string `json:"partitions"`
}

// digestOf is a storage's digest.
func digestOf(storage *simulator.StateTimeStorage) storageDigest {
	digest := storageDigest{Times: bitsDigest([][]float64{storage.GetTimes()}),
		Partitions: map[string]string{}}
	for _, name := range storage.GetNames() {
		rows := storage.GetValues(name)
		width := 0
		if len(rows) > 0 {
			width = len(rows[0])
		}
		digest.Partitions[name] = fmt.Sprintf("%dx%d %s", len(rows), width, bitsDigest(rows))
	}
	return digest
}

// bitsDigest hashes rows' float64 bits, in order.
func bitsDigest(rows [][]float64) string {
	hash := sha256.New()
	var bits [8]byte
	for _, row := range rows {
		for _, value := range row {
			binary.LittleEndian.PutUint64(bits[:], math.Float64bits(value))
			hash.Write(bits[:])
		}
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// oracleConfigs are every shipped macros: config: the examples and the
// stochadex-model skill's recipes, relative to the repository root.
func oracleConfigs(t *testing.T) []string {
	t.Helper()
	var configs []string
	for _, pattern := range []string{"cfg/example_*.yaml", ".claude/skills/stochadex-model/recipes/*.yaml"} {
		paths, _ := filepath.Glob(pattern)
		for _, path := range paths {
			text, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(string(text), "macros:") || strings.Contains(string(text), "\nmacros:") {
				configs = append(configs, path)
			}
		}
	}
	sort.Strings(configs)
	return configs
}

// TestExpansionOracle is PLAN 2.6's exact oracle: every shipped macros: config
// runs, expanded into the core language, to storage bit-for-bit identical to
// the macro tier's, with the same seeds; and the expanded run matches the
// golden digests recorded for this architecture, which keep that output pinned
// once the macro tier is gone. Floating-point results differ between
// architectures (arm64 fuses multiply-adds), so each has its own goldens; a
// run on one with none prints the goldens it would record.
func TestExpansionOracle(t *testing.T) {
	t.Chdir("../..")
	goldenPath := filepath.Join("pkg/api/testdata/golden", "expansion_"+runtime.GOARCH+".json")
	var goldens expansionGoldens
	text, err := os.ReadFile(goldenPath)
	recorded := err == nil
	if recorded {
		if err := json.Unmarshal(text, &goldens); err != nil {
			t.Fatal(err)
		}
	}
	configs := oracleConfigs(t)
	if len(configs) < 13 {
		t.Fatalf("found %d macros: configs, want every shipped one (13 when written): %v", len(configs), configs)
	}
	run := expansionGoldens{Go: runtime.Version(), Configs: map[string]storageDigest{}}
	for _, path := range configs {
		t.Run(path, func(t *testing.T) {
			config, err := LoadConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			macro, err := RunToStorage(config)
			if err != nil {
				t.Fatal(err)
			}
			fresh, err := LoadConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			expanded, err := Expand(fresh)
			if err != nil {
				t.Fatal(err)
			}
			single, err := RunToStorage(expanded)
			if err != nil {
				t.Fatal(err)
			}
			digest := digestOf(single.Storage)
			run.Configs[path] = digest
			if want := digestOf(macro.Storage); fmt.Sprint(digest) != fmt.Sprint(want) {
				t.Errorf("the expanded run differs from the macro tier:\n%s", digestDiff(digest, want))
			}
			if want, pinned := goldens.Configs[path]; recorded && !*updateGoldens {
				if !pinned {
					t.Errorf("%s has no golden; record it with -update-goldens", goldenPath)
				} else if fmt.Sprint(digest) != fmt.Sprint(want) {
					t.Errorf("the expanded run differs from its golden (recorded with %s, run with %s):\n%s",
						goldens.Go, runtime.Version(), digestDiff(digest, want))
				}
			}
		})
	}
	written, _ := json.MarshalIndent(run, "", "  ")
	switch {
	case *updateGoldens:
		if err := os.WriteFile(goldenPath, append(written, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	case !recorded:
		t.Fatalf("no goldens for %s at %s; run with -update-goldens to record these:\n%s",
			runtime.GOARCH, goldenPath, written)
	default:
		for path := range goldens.Configs {
			if _, shipped := run.Configs[path]; !shipped {
				t.Errorf("%s pins %s, which is no longer a shipped macros: config; "+
					"rerun with -update-goldens", goldenPath, path)
			}
		}
	}
}

// digestDiff names what differs between two digests.
func digestDiff(got, want storageDigest) string {
	var lines []string
	if got.Times != want.Times {
		lines = append(lines, "times")
	}
	names := map[string]bool{}
	for name := range got.Partitions {
		names[name] = true
	}
	for name := range want.Partitions {
		names[name] = true
	}
	for _, name := range sortedKeys(names) {
		if got.Partitions[name] != want.Partitions[name] {
			lines = append(lines, fmt.Sprintf("%s: %q, want %q", name, got.Partitions[name], want.Partitions[name]))
		}
	}
	return strings.Join(lines, "\n")
}
