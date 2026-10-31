package api

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// alikeGoldens are the expansion tests' expanded runs (see expandedAlike), for
// the Go version that recorded them, keyed by test name.
type alikeGoldens struct {
	Go    string                   `json:"go"`
	Tests map[string]storageDigest `json:"tests"`
}

// alike holds this architecture's recorded alikeGoldens, and the digests this
// run made, as expandedAlike calls it from tests that may run in parallel.
var alike = struct {
	sync.Mutex
	path     string
	recorded bool
	goldens  alikeGoldens
	run      map[string]storageDigest
	calls    map[string]int
}{run: map[string]storageDigest{}, calls: map[string]int{}}

func init() {
	// The tests that change directory do so after this, so the path is the
	// package's own.
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	alike.path = filepath.Join(dir, "testdata", "golden", "alike_"+runtime.GOARCH+".json")
	text, err := os.ReadFile(alike.path)
	if err != nil {
		return
	}
	if err := json.Unmarshal(text, &alike.goldens); err != nil {
		panic(fmt.Sprintf("%s: %v", alike.path, err))
	}
	alike.recorded = true
}

// checkAlikeGolden checks an expanded run's storage against the golden this
// test recorded for this architecture. A test that expands more than one
// config numbers each after the first.
func checkAlikeGolden(t *testing.T, storage *simulator.StateTimeStorage) {
	t.Helper()
	alike.Lock()
	defer alike.Unlock()
	key := t.Name()
	if alike.calls[t.Name()]++; alike.calls[t.Name()] > 1 {
		key = fmt.Sprintf("%s#%d", t.Name(), alike.calls[t.Name()])
	}
	digest := digestOf(storage)
	alike.run[key] = digest
	if *updateGoldens {
		return
	}
	want, pinned := alike.goldens.Tests[key]
	switch {
	case !alike.recorded:
		t.Errorf("no expansion goldens for %s at %s; run with -update-goldens to record them",
			runtime.GOARCH, alike.path)
	case !pinned:
		t.Errorf("%s has no golden for %s; record it with -update-goldens", alike.path, key)
	case fmt.Sprint(digest) != fmt.Sprint(want):
		t.Errorf("the expanded run differs from its golden (recorded with %s, run with %s):\n%s",
			alike.goldens.Go, runtime.Version(), digestDiff(digest, want))
	}
}

// TestMain records the expansion tests' goldens under -update-goldens: all of
// them on an unfiltered run, or, on a run filtered by -run, those it made
// added to the rest already recorded. Without the flag, it prints the
// goldens a run would record when any were missing, so an architecture with
// none (as CI's may be) can be recorded from its log; and, on an unfiltered
// run, it fails on a golden no test made.
func TestMain(m *testing.M) {
	code := m.Run()
	alike.Lock()
	defer alike.Unlock()
	run := flag.Lookup("test.run")
	unfiltered := run != nil && run.Value.String() == ""
	if *updateGoldens {
		recorded := alikeGoldens{Go: runtime.Version(), Tests: map[string]storageDigest{}}
		if !unfiltered {
			for key, digest := range alike.goldens.Tests {
				recorded.Tests[key] = digest
			}
		}
		for key, digest := range alike.run {
			recorded.Tests[key] = digest
		}
		written, _ := json.MarshalIndent(recorded, "", "  ")
		if err := os.WriteFile(alike.path, append(written, '\n'), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
		os.Exit(code)
	}
	missing := !alike.recorded
	for key := range alike.run {
		if _, pinned := alike.goldens.Tests[key]; !pinned {
			missing = true
		}
	}
	if missing {
		written, _ := json.MarshalIndent(alikeGoldens{Go: runtime.Version(), Tests: alike.run}, "", "  ")
		fmt.Printf("expansion goldens this run would record at %s:\n%s\n", alike.path, written)
	}
	if alike.recorded && unfiltered {
		var stale []string
		for key := range alike.goldens.Tests {
			if _, made := alike.run[key]; !made {
				stale = append(stale, key)
			}
		}
		if len(stale) > 0 {
			sort.Strings(stale)
			fmt.Printf("%s pins tests that no longer run, %v; rerun with -update-goldens\n",
				alike.path, stale)
			code = 1
		}
	}
	os.Exit(code)
}
