package api

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// modelReaderIteration stands in for an iteration that reads a model file
// named by model_path, as the ONNX partition does. It ignores the file.
type modelReaderIteration struct{}

func (*modelReaderIteration) Configure(int, *simulator.Settings) {}

func (*modelReaderIteration) Iterate(
	*simulator.Params, int, []*simulator.StateHistory, *simulator.CumulativeTimestepsHistory,
) []float64 {
	return []float64{1}
}

func init() {
	RegisterIteration("model_reader", func(spec simulator.ComponentSpec) (simulator.Iteration, error) {
		if _, ok := spec.Fields["model_path"].(string); !ok {
			return nil, fmt.Errorf("model_reader needs model_path")
		}
		return &modelReaderIteration{}, nil
	})
	RegisterDataSource("versioned_store", func(map[string]interface{}) (*simulator.StateTimeStorage, error) {
		return nil, fmt.Errorf("not read in these tests")
	})
	RegisterSourceFingerprint("versioned_store", func(fields map[string]interface{}) (string, error) {
		return "version:" + fields["version"].(string), nil
	})
	RegisterDataSource("path_store", func(map[string]interface{}) (*simulator.StateTimeStorage, error) {
		return nil, fmt.Errorf("not read in these tests")
	})
	// The CLI module registers the real arrow sink; this stand-in writes its
	// file at Finalize, as that does.
	simulator.RegisterComponent("output_function", "arrow", func(spec simulator.ComponentSpec) (interface{}, error) {
		return &fileAtFinalize{path: spec.Fields["path"].(string)}, nil
	})
	RegisterDataSource("opaque_store", func(map[string]interface{}) (*simulator.StateTimeStorage, error) {
		return nil, fmt.Errorf("not read in these tests")
	})
}

// fileAtFinalize writes a file when its run finalizes.
type fileAtFinalize struct {
	simulator.NilOutputFunction
	path string
}

func (f *fileAtFinalize) Finalize() { os.WriteFile(f.path, []byte("written"), 0o644) }

// pinnedBuild pins the build to a revision for the test, as a release binary
// is: a test binary carries no version control stamp, and a run is cacheable
// only from a build that can be pinned.
func pinnedBuild(t *testing.T, revision string, features ...string) {
	t.Helper()
	previous, previousFeatures := BuildRevision, BuildFeatures
	BuildRevision, BuildFeatures = revision, features
	t.Cleanup(func() { BuildRevision, BuildFeatures = previous, previousFeatures })
}

// scenario is everything that decides a run's outputs.
type scenario struct {
	seed     int
	input    string // the CSV input's contents
	override string // a --set on a param, or "" for none
	model    string // the model file's contents
	revision string
	features []string
	image    string
	reformat bool // write the same config with other comments, layout and key order
	// dir holds the input, the model file and the output. Their paths are part
	// of the config, so a trial keeps them fixed.
	dir string
}

func (s scenario) config() string {
	csv, model := filepath.Join(s.dir, "in.csv"), filepath.Join(s.dir, "model.onnx")
	if s.reformat {
		return fmt.Sprintf(`# The same run, written differently.
outputs:
  - function:
      path: %q
      type: json_log
    name: log
main:
  simulation:
    init_time_value: 0.0
    timestep_function: {type: from_input, input: prices}
    termination_condition: {type: input_exhausted, input: prices}
  partitions:
    - name: price
      state_history_depth: 1
      iteration: {input: prices, type: from_input}
    - {seed: %d, name: walk, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [0.0], state_history_depth: 1}
    - {name: brain, iteration: {model_path: %q, type: model_reader}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
inputs:
  prices:
    source:
      csv: {path: %q, time_column: 0, state_columns: {price: [1]}}
`, filepath.Join(s.dir, "run.log"), s.seed, model, csv)
	}
	return fmt.Sprintf(`inputs:
  prices: {source: {csv: {path: %q, time_column: 0, state_columns: {price: [1]}}}}
main:
  partitions:
  - {name: price, iteration: {type: from_input, input: prices}, state_history_depth: 1}
  - {name: walk, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [0.0], state_history_depth: 1, seed: %d}
  - {name: brain, iteration: {type: model_reader, model_path: %q}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
  simulation:
    timestep_function: {type: from_input, input: prices}
    termination_condition: {type: input_exhausted, input: prices}
    init_time_value: 0.0
outputs:
- {name: log, function: {type: json_log, path: %q}}
`, csv, s.seed, model, filepath.Join(s.dir, "run.log"))
}

// keyOf writes a scenario's files and returns its key.
func keyOf(t *testing.T, s scenario) string {
	t.Helper()
	writeFileAt(t, filepath.Join(s.dir, "in.csv"), s.input)
	writeFileAt(t, filepath.Join(s.dir, "model.onnx"), s.model)
	pinnedBuild(t, s.revision, s.features...)
	t.Setenv(imageDigestEnv, s.image)
	options := []LoadOption{}
	if s.override != "" {
		options = append(options, WithSet("main.partitions[name=walk].params.variances", s.override))
	}
	config, err := LoadConfig(writeConfigPath(t, s.config()), options...)
	if err != nil {
		t.Fatal(err)
	}
	provenance, err := ComputeProvenance(config)
	if err != nil {
		t.Fatal(err)
	}
	if provenance.Key == "" {
		t.Fatalf("the scenario should be cacheable: %v", provenance.Uncacheable)
	}
	return provenance.Key
}

func TestProvenanceKeyProperty(t *testing.T) {
	random := rand.New(rand.NewSource(20261010))
	csvRows := func() string {
		rows := ""
		for step := range 3 + random.Intn(4) {
			rows += fmt.Sprintf("%d,%.3f\n", step, random.Float64())
		}
		return rows
	}
	for trial := range 25 {
		base := scenario{
			seed:     random.Intn(1 << 20),
			input:    csvRows(),
			model:    fmt.Sprintf("weights-%d", random.Int()),
			revision: fmt.Sprintf("rev%d", random.Intn(1000)),
			features: []string{"arrow", "s3"},
			image:    fmt.Sprintf("sha256:%x", random.Int63()),
			dir:      t.TempDir(),
		}
		if random.Intn(2) == 0 {
			base.override = "[2.0]"
		}
		key := keyOf(t, base)
		if again := keyOf(t, base); again != key {
			t.Fatalf("trial %d: the same run gave keys %s and %s", trial, key, again)
		}
		reformatted := base
		reformatted.reformat = true
		if got := keyOf(t, reformatted); got != key {
			t.Errorf("trial %d: rewriting the config's comments, layout and key order changed the key", trial)
		}
		changes := map[string]func(*scenario){
			"the seed":         func(s *scenario) { s.seed++ },
			"the input":        func(s *scenario) { s.input += "99,1.0\n" },
			"one input byte":   func(s *scenario) { s.input = strings.Replace(s.input, "0", "1", 1) },
			"an override":      func(s *scenario) { s.override = map[bool]string{true: "", false: "[3.0]"}[s.override != ""] },
			"the model file":   func(s *scenario) { s.model += "x" },
			"the revision":     func(s *scenario) { s.revision += "x" },
			"the features":     func(s *scenario) { s.features = []string{"arrow", "cblas", "s3"} },
			"the image digest": func(s *scenario) { s.image += "0" },
		}
		for name, change := range changes {
			changed := base
			change(&changed)
			if got := keyOf(t, changed); got == key {
				t.Errorf("trial %d: changing %s left the key %s", trial, name, key)
			}
		}
	}
}

func TestProvenanceSaysWhyARunIsNotCacheable(t *testing.T) {
	cacheable := func(t *testing.T) {
		pinnedBuild(t, "pinned")
		t.Setenv(imageDigestEnv, "")
	}
	walk := walkPartition("w", "")
	cases := []struct{ name, yaml, want string }{
		{"a Postgres input",
			"inputs:\n  db: {source: {postgres: {user: u, password: p, dbname: d, table: t, partition_names: [p], start_time: 0, end_time: 5}}}\n" +
				checkYAML("  - {name: p, iteration: {type: from_input, input: db}, init_state_values: [0.0], state_history_depth: 1}\n", ""),
			`input "db" reads a Postgres table`},
		{"a live stream",
			"inputs:\n  feed: {stream: {websocket: {url: \"ws://localhost:1/x\"}}}\n" +
				checkYAML(walkPartition("w", ", params_from_input: {variances: {input: feed}}"), ""),
			`input "feed" is a live stream`},
		{"a served run",
			strings.Replace(servedYAML(serveConfigYAML, "127.0.0.1:0"), "", "", 1),
			"a served run is driven by its clients"},
		{"a registered source with no fingerprint",
			"inputs:\n  o: {source: {opaque_store: {bucket: b}}}\n" +
				checkYAML("  - {name: p, iteration: {type: from_input, input: o}, init_state_values: [0.0], state_history_depth: 1}\n", ""),
			`input "o" is a opaque_store source, which has no fingerprint`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cacheable(t)
			config, err := LoadConfig(writeConfigPath(t, c.yaml))
			if err != nil {
				t.Fatal(err)
			}
			provenance, err := ComputeProvenance(config)
			if err != nil {
				t.Fatal(err)
			}
			if provenance.Key != "" || !slices.ContainsFunc(provenance.Uncacheable,
				func(reason string) bool { return strings.Contains(reason, c.want) }) {
				t.Errorf("want no key and a reason containing %q, got key %q and %v",
					c.want, provenance.Key, provenance.Uncacheable)
			}
		})
	}
	t.Run("a local build, stamped only by version control, clean or not", func(t *testing.T) {
		pinnedBuild(t, "")
		config, err := LoadConfig(writeConfigPath(t, checkYAML(walk, "")))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { buildVCS = vcsRevision })
		for _, dirty := range []bool{false, true} {
			buildVCS = func() (string, bool, bool) { return "abc123", dirty, true }
			provenance, _ := ComputeProvenance(config)
			if provenance.Key != "" || provenance.Build.VCSRevision != "abc123" ||
				provenance.Build.VCSDirty != dirty || provenance.Build.Revision != "" {
				t.Errorf("dirty %v: got key %q and build %+v", dirty, provenance.Key, provenance.Build)
			}
		}
	})

	t.Run("a module installed at a version is pinned by it", func(t *testing.T) {
		pinnedBuild(t, "")
		original := buildModuleVersion
		buildModuleVersion = func() string { return "v0.21.0" }
		t.Cleanup(func() { buildModuleVersion = original })
		config, err := LoadConfig(writeConfigPath(t, checkYAML(walk, "")))
		if err != nil {
			t.Fatal(err)
		}
		provenance, _ := ComputeProvenance(config)
		if provenance.Key == "" || provenance.Build.Version != "v0.21.0" {
			t.Errorf("got key %q and build %+v", provenance.Key, provenance.Build)
		}
		previous := BuildVersion
		BuildVersion = "v0.20.0"
		t.Cleanup(func() { BuildVersion = previous })
		if stamped, _ := ComputeProvenance(config); stamped.Build.Version != "v0.20.0" {
			t.Errorf("a stamped version should win over the module's, got %q", stamped.Build.Version)
		}
	})

	t.Run("an unpinned build", func(t *testing.T) {
		pinnedBuild(t, "")
		config, err := LoadConfig(writeConfigPath(t, checkYAML(walk, "")))
		if err != nil {
			t.Fatal(err)
		}
		provenance, _ := ComputeProvenance(config)
		if provenance.Key != "" || !slices.Contains(provenance.Uncacheable,
			"the build is not a release or a stamped image, so it cannot be pinned (Go's own version-control stamp is not trusted)") {
			t.Errorf("got key %q and %v", provenance.Key, provenance.Uncacheable)
		}
	})
	t.Run("a config built in memory", func(t *testing.T) {
		cacheable(t)
		config, err := LoadConfig(writeConfigPath(t, checkYAML(walk, "")))
		if err != nil {
			t.Fatal(err)
		}
		config.source = nil
		provenance, _ := ComputeProvenance(config)
		if provenance.Key != "" {
			t.Errorf("a config with no document should have no key, got %q", provenance.Key)
		}
	})
}

func TestProvenanceFingerprintsRegisteredSources(t *testing.T) {
	pinnedBuild(t, "pinned")
	dir := t.TempDir()
	stored := filepath.Join(dir, "stored.bin")
	writeFileAt(t, stored, "contents")
	key := func(version string) (string, []ProvenanceInput) {
		yaml := fmt.Sprintf("inputs:\n  v: {source: {versioned_store: {version: %q}}}\n"+
			"  f: {source: {path_store: {path: %q}}}\n", version, stored) +
			checkYAML("  - {name: a, iteration: {type: from_input, input: v}, init_state_values: [0.0], state_history_depth: 1}\n"+
				"  - {name: b, iteration: {type: from_input, input: f}, init_state_values: [0.0], state_history_depth: 1}\n", "")
		config, err := LoadConfig(writeConfigPath(t, yaml))
		if err != nil {
			t.Fatal(err)
		}
		provenance, err := ComputeProvenance(config)
		if err != nil {
			t.Fatal(err)
		}
		return provenance.Key, provenance.Inputs
	}
	first, inputs := key("v1")
	digest, _ := fileDigest(stored)
	if inputs[0].Fingerprint != "sha256:"+digest || inputs[1].Fingerprint != "version:v1" {
		t.Errorf("fingerprints %v; want the file's digest and the registered version", inputs)
	}
	if again, _ := key("v1"); again != first {
		t.Error("the same versions gave different keys")
	}
	if moved, _ := key("v2"); moved == first {
		t.Error("a new object version left the key unchanged")
	}
	writeFileAt(t, stored, "rewritten")
	if rewritten, _ := key("v1"); rewritten == first {
		t.Error("rewriting a path source left the key unchanged")
	}
}

// provenanceYAML is a run with file outputs in main, in an embedded run and,
// optionally, per ensemble member.
func provenanceYAML(dir string, ensemble bool) string {
	yaml := strings.Replace(stagedYAML(0, 0, filepath.Join(dir, "run.log"), filepath.Join(dir, "nested.log")),
		"embedded:\n", fmt.Sprintf("- {name: columns, function: {type: arrow, path: %q}}\nembedded:\n",
			filepath.Join(dir, "run.arrow")), 1)
	if ensemble {
		yaml = strings.NewReplacer("out/run-{member}-{seed}.log", filepath.Join(dir, "member-{member}.log")).
			Replace(ensembleIOYAML)
	}
	return yaml
}

func readProvenance(t *testing.T, path string) Provenance {
	t.Helper()
	var p Provenance
	if err := json.Unmarshal([]byte(readAll(t, path+sidecarSuffix)), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProvenanceSidecars(t *testing.T) {
	pinnedBuild(t, "pinned")
	for _, ensemble := range []bool{false, true} {
		t.Run(fmt.Sprintf("ensemble %v: every file output gets the run's provenance", ensemble), func(t *testing.T) {
			dir := t.TempDir()
			config := writeConfigPath(t, provenanceYAML(dir, ensemble))
			err, printed := executeIn(t, t.TempDir(), "inspect", "--provenance", "-c", config)
			if err != nil {
				t.Fatal(err)
			}
			var inspected Provenance
			if err := json.Unmarshal([]byte(printed), &inspected); err != nil {
				t.Fatal(err)
			}
			if err := Execute([]string{"stochadex", "--config", config, "--provenance"}); err != nil {
				t.Fatal(err)
			}
			targets := []string{"run.log", "nested.log", "run.arrow"}
			if ensemble {
				targets = []string{"member-0.log", "member-1.log"}
			}
			want := []string{}
			for _, target := range targets {
				want = append(want, target, target+sidecarSuffix)
				if got := readProvenance(t, filepath.Join(dir, target)); got.Key != inspected.Key || got.Key == "" {
					t.Errorf("%s's sidecar has key %q; inspect --provenance said %q", target, got.Key, inspected.Key)
				}
			}
			slices.Sort(want)
			if files := filesUnder(t, dir); !slices.Equal(files, want) {
				t.Errorf("found %v, want %v", files, want)
			}
		})
	}

	t.Run("a run that fails writes no provenance and leaves the earlier sidecar", func(t *testing.T) {
		dir := t.TempDir()
		log, nested := filepath.Join(dir, "run.log"), filepath.Join(dir, "nested.log")
		writeFileAt(t, log+sidecarSuffix, earlierLog)
		err := Execute([]string{"stochadex", "--config",
			writeConfigPath(t, stagedYAML(20, 0, log, nested)), "--provenance"})
		if KindOf(err) != ErrRuntime {
			t.Fatalf("expected the run to fail, got %v", err)
		}
		if got := readAll(t, log+sidecarSuffix); got != earlierLog {
			t.Errorf("the earlier sidecar changed: %q", got)
		}
		if _, err := os.Stat(nested + sidecarSuffix); err == nil {
			t.Error("a failed run wrote provenance")
		}
	})

	t.Run("a sidecar that cannot be written fails the run as unavailable", func(t *testing.T) {
		dir := t.TempDir()
		writeFileAt(t, filepath.Join(dir, "run.log"+sidecarSuffix, "inside"), "x")
		err := Execute([]string{"stochadex", "--config", writeConfigPath(t, provenanceYAML(dir, false)), "--provenance"})
		if KindOf(err) != ErrUnavailable || !strings.Contains(err.Error(), "writing provenance") {
			t.Errorf("expected ErrUnavailable, got %v", err)
		}
	})

	t.Run("without --provenance, no sidecar is written", func(t *testing.T) {
		dir := t.TempDir()
		if err := Execute([]string{"stochadex", "--config", writeConfigPath(t, provenanceYAML(dir, false))}); err != nil {
			t.Fatal(err)
		}
		if files := filesUnder(t, dir); !slices.Equal(files, []string{"nested.log", "run.arrow", "run.log"}) {
			t.Errorf("found %v", files)
		}
	})
}

func TestSkipIfUnchanged(t *testing.T) {
	pinnedBuild(t, "pinned")
	dir := t.TempDir()
	csv := filepath.Join(dir, "in.csv")
	writeFileAt(t, csv, "0,1.0\n1,2.0\n2,3.0\n")
	log := filepath.Join(dir, "run.log")
	yaml := fmt.Sprintf(`inputs:
  prices: {source: {csv: {path: %q, time_column: 0, state_columns: {price: [1]}}}}
main:
  partitions:
  - {name: price, iteration: {type: from_input, input: prices}, state_history_depth: 1}
  - {name: walk, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [0.0], state_history_depth: 1, seed: 3}
  simulation:
    timestep_function: {type: from_input, input: prices}
    termination_condition: {type: input_exhausted, input: prices}
    init_time_value: 0.0
outputs:
- {name: log, function: {type: json_log, path: %q}}
`, csv, log)
	config := writeConfigPath(t, yaml)
	const marker = "left by the test, so a run that rewrites the log is seen\n"
	// run runs with --skip-if-unchanged after marking the log, reporting
	// whether the run happened (the log was rewritten).
	run := func(t *testing.T, args ...string) bool {
		t.Helper()
		writeFileAt(t, log, marker)
		if err := Execute(append([]string{"stochadex", "--config", config, "--skip-if-unchanged"}, args...)); err != nil {
			t.Fatal(err)
		}
		return readAll(t, log) != marker
	}
	first := func(t *testing.T) {
		t.Helper()
		os.Remove(log + sidecarSuffix)
		if !run(t) {
			t.Fatal("a first run, with no provenance yet, should run")
		}
	}

	t.Run("an unchanged run is skipped", func(t *testing.T) {
		first(t)
		if run(t) {
			t.Error("an unchanged run ran again")
		}
	})
	t.Run("a changed input runs", func(t *testing.T) {
		first(t)
		writeFileAt(t, csv, "0,1.0\n1,2.0\n2,3.5\n")
		if !run(t) {
			t.Error("a run with a changed input was skipped")
		}
	})
	t.Run("a changed override runs", func(t *testing.T) {
		first(t)
		if !run(t, "--set", "main.partitions[name=walk].seed=4") {
			t.Error("a run with a new override was skipped")
		}
		if run(t, "--set", "main.partitions[name=walk].seed=4") {
			t.Error("the same override again should be skipped")
		}
	})
	t.Run("a changed build runs", func(t *testing.T) {
		first(t)
		pinnedBuild(t, "another")
		if !run(t) {
			t.Error("a run from another build was skipped")
		}
	})
	t.Run("a missing output runs", func(t *testing.T) {
		first(t)
		os.Remove(log)
		if err := Execute([]string{"stochadex", "--config", config, "--skip-if-unchanged"}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(log); err != nil {
			t.Error("a missing output was not made again")
		}
	})
	t.Run("a missing or foreign sidecar runs", func(t *testing.T) {
		first(t)
		os.Remove(log + sidecarSuffix)
		if !run(t) {
			t.Error("an output with no sidecar was skipped")
		}
		writeFileAt(t, log+sidecarSuffix, `{"key": "another run's"}`)
		if !run(t) {
			t.Error("an output from another run was skipped")
		}
	})
	t.Run("an uncacheable run always runs, however often", func(t *testing.T) {
		first(t)
		pinnedBuild(t, "")
		for attempt := range 2 {
			if !run(t) {
				t.Errorf("attempt %d: a run from an unpinned build was skipped", attempt)
			}
		}
	})
}

func TestInspectProvenanceUsage(t *testing.T) {
	config := writeConfigPath(t, checkYAML(walkPartition("w", ""), ""))
	for _, args := range [][]string{{"inspect", "-c", config}, {"inspect", "--io", "--provenance", "-c", config}} {
		if err, _ := executeIn(t, t.TempDir(), args...); KindOf(err) != ErrUsage {
			t.Errorf("%v: expected ErrUsage, got %v", args, err)
		}
	}
	err, _ := executeIn(t, t.TempDir(), "inspect", "--provenance", "-c", writeConfigPath(t,
		"inputs:\n  x: {source: {csv: {path: /no/such/file.csv, time_column: 0, state_columns: {x: [1]}}}}\n"+
			checkYAML("  - {name: x, iteration: {type: from_input, input: x}, state_history_depth: 1}\n", "")))
	if KindOf(err) != ErrUnavailable {
		t.Errorf("a missing input should be unavailable, got %v", err)
	}
}

func TestProvenanceFindsModelFilesAnywhereInASpec(t *testing.T) {
	pinnedBuild(t, "pinned")
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "top.onnx"), filepath.Join(dir, "nested.onnx"), filepath.Join(dir, "listed.onnx")}
	for _, path := range paths {
		writeFileAt(t, path, "weights of "+filepath.Base(path))
	}
	yaml := checkYAML(fmt.Sprintf("  - {name: brain, iteration: {type: model_reader, model_path: %q, "+
		"options: {model_path: %q}, stages: [{model_path: %q}]}, init_state_values: [0.0], state_history_depth: 1, seed: 0}\n",
		paths[0], paths[1], paths[2]), "")
	compute := func() *Provenance {
		config, err := LoadConfig(writeConfigPath(t, yaml))
		if err != nil {
			t.Fatal(err)
		}
		provenance, err := ComputeProvenance(config)
		if err != nil {
			t.Fatal(err)
		}
		return provenance
	}
	first := compute()
	found := []string{}
	for _, file := range first.Files {
		found = append(found, file.Path)
	}
	want := append([]string(nil), paths...)
	slices.Sort(want)
	if !slices.Equal(found, want) {
		t.Fatalf("found model files %v, want %v", found, want)
	}
	for _, path := range paths {
		writeFileAt(t, path, "retrained")
		if compute().Key == first.Key {
			t.Errorf("retraining %s left the key unchanged", filepath.Base(path))
		}
		writeFileAt(t, path, "weights of "+filepath.Base(path))
	}
	os.Remove(paths[1])
	config, _ := LoadConfig(writeConfigPath(t, yaml))
	if _, err := ComputeProvenance(config); KindOf(err) != ErrUnavailable {
		t.Errorf("a missing model file should be unavailable, got %v", err)
	}
}

func TestModuleVersionIsTrustedOnlyWithoutAVersionControlStamp(t *testing.T) {
	installed := &debug.BuildInfo{Main: debug.Module{Version: "v0.21.0"}}
	local := &debug.BuildInfo{Main: debug.Module{Version: "v0.0.0-20261004071117-e8919bf3fa43"},
		Settings: []debug.BuildSetting{{Key: "vcs", Value: "git"}, {Key: "vcs.revision", Value: "e8919bf"}}}
	cases := map[string]struct {
		info *debug.BuildInfo
		want string
	}{
		"go install at a version": {installed, "v0.21.0"},
		"a build in a checkout":   {local, ""},
		"a development build":     {&debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, ""},
		"no version":              {&debug.BuildInfo{}, ""},
		"no build information":    {nil, ""},
	}
	for name, c := range cases {
		if got := moduleVersion(c.info); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}
