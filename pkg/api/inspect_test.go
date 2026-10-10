package api

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

var updateManifests = flag.Bool("update-manifests", false,
	"rewrite testdata/manifests from the current I/O manifests")

// repoDir is the absolute repository root, where the shipped configs' relative
// paths start, as when the CLI is run from a checkout. It is read before any
// test changes directory.
var repoDir = func() string {
	root, err := filepath.Abs("../..")
	if err != nil {
		panic(err)
	}
	return root
}()

func repoRootPath(*testing.T) string { return repoDir }

// writeFileAt writes contents to path, making its directory.
func writeFileAt(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, contents)
}

// shippedConfigs are cfg/example_*.yaml, relative to the repository root.
func shippedConfigs(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(repoRootPath(t), "cfg", "example_*.yaml"))
	if err != nil || len(paths) < 15 {
		t.Fatalf("found %d shipped configs (%v)", len(paths), err)
	}
	for i, path := range paths {
		paths[i] = filepath.Join("cfg", filepath.Base(path))
	}
	return paths
}

// fileInputs are the files a manifest says a run reads.
func fileInputs(manifest *IOManifest) []string {
	files := []string{}
	for _, input := range manifest.Inputs {
		if input.Kind == "csv" || input.Kind == "json_log" {
			files = append(files, filepath.Clean(input.Location))
		}
	}
	return files
}

// fileOutputs are the files a manifest says a run writes, debug logs
// included: each member's for an ensemble.
func fileOutputs(manifest *IOManifest) []string {
	files := []string{}
	for _, output := range append(append([]ManifestOutput(nil), manifest.Outputs...), manifest.Debug...) {
		switch output.Sink {
		case "stdout", "nil", connectionSink:
			continue
		}
		locations := output.Locations
		if len(locations) == 0 {
			locations = []string{output.Location}
		}
		for _, location := range locations {
			files = append(files, filepath.Clean(location))
		}
	}
	slices.Sort(files)
	return files
}

// filesUnder lists the files under dir, relative to it.
func filesUnder(t *testing.T, dir string) []string {
	t.Helper()
	files := []string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(dir, path)
		files = append(files, relative)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(files)
	return files
}

// sandboxRun runs a config through the CLI in an empty directory holding only
// the files its manifest says it reads (but omit), which provide writes, each
// to the path the manifest gives. It returns the run's error and the files it
// left besides those.
func sandboxRun(
	t *testing.T,
	config string,
	manifest *IOManifest,
	provide func(path string) []byte,
	omit string,
	args ...string,
) (error, []string) {
	t.Helper()
	dir := t.TempDir()
	inputs := fileInputs(manifest)
	for _, path := range inputs {
		if path != omit {
			writeFileAt(t, filepath.Join(dir, path), string(provide(path)))
		}
	}
	for _, path := range fileOutputs(manifest) {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(path)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	var err error
	captureStdout(t, func() {
		err = Execute(append([]string{"stochadex", "--config", config}, args...))
	})
	written := []string{}
	for _, path := range filesUnder(t, dir) {
		if !slices.Contains(inputs, path) {
			written = append(written, path)
		}
	}
	return err, written
}

// loadManifest loads a config from the repository root, as the CLI would be
// run from a checkout, and returns its manifest.
func loadManifest(t *testing.T, path string, options ...LoadOption) *IOManifest {
	t.Helper()
	t.Chdir(repoRootPath(t))
	config, err := LoadConfig(path, options...)
	if err != nil {
		t.Fatal(err)
	}
	return Manifest(config)
}

func TestManifestGolden(t *testing.T) {
	for _, path := range shippedConfigs(t) {
		t.Run(path, func(t *testing.T) {
			manifest := loadManifest(t, path)
			got, err := json.MarshalIndent(manifest, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			golden := filepath.Join(repoRootPath(t), "pkg", "api", "testdata", "manifests",
				strings.TrimSuffix(filepath.Base(path), ".yaml")+".json")
			if *updateManifests {
				writeFileAt(t, golden, string(got)+"\n")
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run with -update-manifests to create it)", err)
			}
			if string(got)+"\n" != string(want) {
				t.Errorf("manifest differs from %s:\n%s", golden, got)
			}
		})
	}
}

// ioYAML exercises every way a main run reads and writes: one column of a CSV
// input read by two from_input partitions and the clock, a json_log input read by
// params_from_input, outputs: views to a file and to stdout, and an embedded
// run's own file sink.
const ioYAML = `inputs:
  prices: {source: {csv: {path: data/prices.csv, time_column: 0, state_columns: {close: [1], volume: [2]}}}}
  rates: {source: {json_log: {path: data/rates.log}}}
main:
  partitions:
  - name: price
    iteration: {type: from_input, input: prices, partition: close}
    state_history_depth: 1
  - name: close_again
    iteration: {type: from_input, input: prices, partition: close}
    state_history_depth: 1
  - name: walk
    iteration: {type: wiener_process}
    params: {variances: [1.0]}
    params_from_input: {variances: {input: rates, partition: rate}}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 3
  - name: nested
    params: {burn_in_steps: [0]}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 0
  simulation:
    timestep_function: {type: from_input, input: prices}
    termination_condition: {type: input_exhausted, input: prices}
    init_time_value: 0.0
outputs:
- {name: log, function: {type: json_log, path: out/run.log}}
- {name: screen, condition: {type: only_given_partitions, partitions: [walk]}, function: {type: stdout}}
embedded:
- name: nested
  partitions:
  - {name: inner, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [5.0], state_history_depth: 1, seed: 11}
  simulation:
    output_condition: {type: every_step}
    output_function: {type: json_log, path: out/nested.log}
    termination_condition: {type: number_of_steps, max_steps: 3}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`

// ensembleIOYAML writes one json_log per member.
const ensembleIOYAML = `main:
  partitions:
  - {name: walk, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [0.0], state_history_depth: 1, seed: 3}
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 5}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
run: {mode: ensemble, seeds: [4, 9]}
outputs:
- {name: log, function: {type: json_log, path: "out/run-{member}-{seed}.log"}}
`

// ioInputs are the files ioYAML reads.
func ioInputs(path string) []byte {
	switch path {
	case filepath.Join("data", "prices.csv"):
		return []byte("0,1.0,2.0\n1,1.5,2.5\n2,2.0,3.0\n3,2.5,3.5\n4,3.0,4.0\n")
	case filepath.Join("data", "rates.log"):
		lines := ""
		for step := 0; step < 5; step++ {
			lines += fmt.Sprintf(`{"partition_name":"rate","state":[%d.5],"time":%d}`+"\n", step+1, step)
		}
		return []byte(lines)
	}
	panic("ioYAML reads no " + path)
}

func TestManifestMatchesWhatARunReadsAndWrites(t *testing.T) {
	// The oracle is the run itself, in an empty directory: given only the files
	// the manifest lists as inputs it succeeds, without any one of them it
	// fails, and the files it leaves are exactly the manifest's outputs.
	root := repoRootPath(t)
	fromRepo := func(path string) []byte {
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			panic(err)
		}
		return data
	}
	type runCase struct {
		name, config string
		provide      func(string) []byte
	}
	cases := []runCase{}
	for _, path := range shippedConfigs(t) {
		cases = append(cases, runCase{path, filepath.Join(root, path), fromRepo})
	}
	cases = append(cases,
		runCase{"every main-run read and write", writeConfigPath(t, ioYAML), ioInputs},
		runCase{"an ensemble's per-member files", writeConfigPath(t, ensembleIOYAML), ioInputs})
	ran := 0
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			manifest := loadManifest(t, c.config)
			if manifest.Run.Mode == "serve" {
				t.Skip("a served run starts per connection; see the serve test")
			}
			ran++
			err, written := sandboxRun(t, c.config, manifest, c.provide, "")
			if err != nil {
				t.Fatalf("the run failed given only the manifest's inputs: %v", err)
			}
			if want := fileOutputs(manifest); !slices.Equal(written, want) {
				t.Errorf("the run wrote %v; the manifest lists %v", written, want)
			}
			for _, missing := range fileInputs(manifest) {
				err, _ := sandboxRun(t, c.config, manifest, c.provide, missing)
				if KindOf(err) != ErrUnavailable || !strings.Contains(err.Error(), missing) {
					t.Errorf("without %s the run should fail as unavailable, got %v", missing, err)
				}
			}
		})
	}
	if ran < 17 {
		t.Errorf("ran %d configs, want every non-serve config", ran)
	}
}

// checkYAML is a five-step main run of the given partitions, plus extra.
func checkYAML(partitions, extra string) string {
	return "main:\n  partitions:\n" + partitions + `  simulation:
    termination_condition: {type: number_of_steps, max_steps: 5}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
` + extra
}

// walkPartition is a width-1 Wiener partition named name, with more fields.
func walkPartition(name, more string) string {
	return "  - {name: " + name + ", iteration: {type: wiener_process}, params: {variances: [1.0]}, " +
		"init_state_values: [0.0], state_history_depth: 1, seed: 1" + more + "}\n"
}

const hostedRun = `embedded:
- name: nested
  partitions:
  - {name: inner, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [5.0], state_history_depth: 1, seed: 11}
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 3}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`

const hostPartition = "  - {name: nested, params: {burn_in_steps: [0]}, init_state_values: [0.0], state_history_depth: 1, seed: 0}\n"

// executeIn runs the CLI in dir, returning its error and what it printed.
func executeIn(t *testing.T, dir string, args ...string) (error, string) {
	t.Helper()
	t.Chdir(dir)
	var err error
	printed := captureStdout(t, func() { err = Execute(append([]string{"stochadex"}, args...)) })
	return err, printed
}

func TestCheck(t *testing.T) {
	t.Run("every shipped config passes, with nothing read, written or printed", func(t *testing.T) {
		// The directory holds none of the configs' inputs, so a check that read
		// one would fail, and one that opened an output would leave a file.
		root := repoRootPath(t)
		for _, path := range shippedConfigs(t) {
			dir := t.TempDir()
			err, printed := executeIn(t, dir, "--config", filepath.Join(root, path), "--check")
			if err != nil {
				t.Errorf("%s: %v", path, err)
			}
			if files := filesUnder(t, dir); len(files) > 0 || printed != "" {
				t.Errorf("%s: --check wrote %v and printed %q", path, files, printed)
			}
		}
	})

	t.Run("a config whose inputs do not exist yet passes; the run then cannot read them", func(t *testing.T) {
		config := writeConfigPath(t, ioYAML)
		dir := t.TempDir()
		if err, _ := executeIn(t, dir, "--config", config, "--check"); err != nil {
			t.Fatalf("--check: %v", err)
		}
		if err, _ := executeIn(t, dir, "--config", config); KindOf(err) != ErrUnavailable {
			t.Errorf("the run should find its inputs missing, got %v", err)
		}
	})

	macroConfig, err := os.ReadFile("../../cfg/example_macro_config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	// The oracle for each bad config is the run: --check must fail with the
	// same message the run fails with, before it steps.
	cases := []struct{ name, yaml, want string }{
		{"a within-step cycle",
			checkYAML(walkPartition("a", ", params_from_upstream: {variances: {upstream: b}}")+
				walkPartition("b", ", params_from_upstream: {variances: {upstream: a}}"), ""),
			"simulation wiring will deadlock"},
		{"an upstream index past the upstream's width",
			checkYAML(walkPartition("a", "")+
				walkPartition("b", ", params_from_upstream: {variances: {upstream: a, indices: [3]}}"), ""),
			`params_from_upstream "variances" -> upstream "a": index 3 out of range (state width 1)`},
		{"an unknown upstream",
			checkYAML(walkPartition("a", ", params_from_upstream: {variances: {upstream: nope}}"), ""),
			"no partition by that name"},
		{"an unknown partition in params_as_partitions",
			checkYAML(walkPartition("a", ", params_as_partitions: {others: [nope]}"), ""),
			"error converting params name: nope"},
		{"an ensemble with no seeds",
			checkYAML(walkPartition("a", ""), "run: {mode: ensemble}\n"),
			"requires a non-empty run.seeds"},
		{"an ensemble with an embedded run",
			checkYAML(walkPartition("a", "")+hostPartition, hostedRun+"run: {mode: ensemble, seeds: [1, 2]}\n"),
			"ensemble run mode does not yet support embedded runs"},
		{"a partition with no state_history_depth",
			checkYAML(strings.Replace(walkPartition("a", ""), "state_history_depth: 1, ", "", 1), ""),
			`api: main partition "a" needs state_history_depth of at least 1, got 0`},
		{"an embedded partition with no state_history_depth",
			checkYAML(walkPartition("a", "")+hostPartition,
				strings.Replace(hostedRun, "state_history_depth: 1, ", "", 1)),
			`api: embedded run nested partition "inner" needs state_history_depth of at least 1`},
		{"data: without macros",
			checkYAML(walkPartition("a", ""), "data: {steps: 3, timestep: 1.0, partitions: []}\n"),
			"sets data: but no macros:"},
		{"macros alongside main partitions",
			string(macroConfig) + "main:\n  partitions:\n" + walkPartition("a", ""),
			"sets both main.partitions and macros:"},
	}
	for _, c := range cases {
		t.Run(c.name+" fails --check as the run fails", func(t *testing.T) {
			config := writeConfigPath(t, c.yaml)
			checked, _ := executeIn(t, t.TempDir(), "--config", config, "--check")
			ran, _ := executeIn(t, t.TempDir(), "--config", config)
			if KindOf(checked) != ErrConfig || !strings.Contains(checked.Error(), c.want) {
				t.Fatalf("expected ErrConfig containing %q, got %v", c.want, checked)
			}
			if ran == nil || ran.Error() != checked.Error() {
				t.Errorf("--check said %q but the run said %v", checked, ran)
			}
		})
	}

	t.Run("an input's undeclared width is left to the run; a declared one is checked", func(t *testing.T) {
		reader := walkPartition("b", ", params_from_upstream: {variances: {upstream: price, indices: [1]}}")
		undeclared := strings.Replace(ioYAML, "  - name: walk\n", reader+"  - name: walk\n", 1)
		if err, _ := executeIn(t, t.TempDir(), "--config", writeConfigPath(t, undeclared), "--check"); err != nil {
			t.Errorf("an undeclared width should be left to the run: %v", err)
		}
		declared := replaceOnce(t, undeclared, "iteration: {type: from_input, input: prices, partition: close}",
			"iteration: {type: from_input, input: prices, partition: close}\n    init_state_values: [0.0]")
		err, _ := executeIn(t, t.TempDir(), "--config", writeConfigPath(t, declared), "--check")
		if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), "index 1 out of range (state width 1)") {
			t.Errorf("a declared width should be checked, got %v", err)
		}
	})

	t.Run("checking leaves the config to run exactly as loaded", func(t *testing.T) {
		dir := t.TempDir()
		for _, path := range []string{"data/prices.csv", "data/rates.log"} {
			writeFileAt(t, filepath.Join(dir, path), string(ioInputs(filepath.FromSlash(path))))
		}
		t.Chdir(dir)
		config := strings.NewReplacer("out/run.log", "run.log", "out/nested.log", "nested.log").Replace(ioYAML)
		reference, err := RunToStorage(writeConfig(t, config))
		if err != nil {
			t.Fatal(err)
		}
		for _, inMemory := range []bool{false, true} {
			loaded := writeConfig(t, config)
			if inMemory {
				loaded.source = nil // as a config built in Go has no document
			}
			if err := Check(loaded); err != nil {
				t.Fatal(err)
			}
			checkedThenRun, err := RunToStorage(loaded)
			if err != nil {
				t.Fatalf("in memory %v: %v", inMemory, err)
			}
			assertStoragesEqual(t, fmt.Sprintf("in memory %v", inMemory), checkedThenRun.Storage, reference.Storage)
		}
	})
}

func TestManifestReportsNoSecrets(t *testing.T) {
	yaml := `inputs:
  db: {source: {postgres: {user: trader, password: s3cret, dbname: market, table: prices, partition_names: [p], start_time: 0, end_time: 5}}}
  feed: {stream: {websocket: {url: "ws://trader:s3cret@feeds.example.org/live?token=s3cret"}}}
main:
  partitions:
  - {name: p, iteration: {type: from_input, input: db}, init_state_values: [0.0], state_history_depth: 1}
` + walkPartition("w", ", params_from_input: {variances: {input: feed}}") + `  simulation:
    termination_condition: {type: number_of_steps, max_steps: 5}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
outputs:
- {name: db, function: {type: postgres, dsn: "postgres://trader:s3cret@db.example.org/market", table: runs}}
- {name: push, function: {type: websocket, url: "ws://trader:s3cret@sink.example.org:9000/in?token=s3cret"}}
- {name: log, function: {type: json_log, path: "out/${RUN_ID}.log"}}
`
	config, err := LoadConfig(writeConfigPath(t, yaml), WithSet("main.partitions[name=w].seed", "4"),
		envOf(map[string]string{"RUN_ID": "run-77"}))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(Manifest(config))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "s3cret") || strings.Contains(string(data), "trader") {
		t.Errorf("the manifest leaks a credential: %s", data)
	}
	manifest := Manifest(config)
	locations := map[string]string{}
	for _, input := range manifest.Inputs {
		locations["input "+input.Name] = input.Kind + " " + input.Location
	}
	for _, output := range manifest.Outputs {
		locations["output "+output.Name] = output.Sink + " " + output.Location
	}
	want := map[string]string{
		"input db":    "postgres postgres:market.prices",
		"input feed":  "stream ws://feeds.example.org/live",
		"output db":   "postgres postgres:runs",
		"output push": "websocket ws://sink.example.org:9000/in",
		"output log":  "json_log out/run-77.log",
	}
	for key, location := range want {
		if locations[key] != location {
			t.Errorf("%s at %q, want %q", key, locations[key], location)
		}
	}
	if !slices.Equal(manifest.Overrides, []string{"--set main.partitions[name=w].seed=4"}) ||
		!slices.Equal(manifest.Variables, []string{"RUN_ID"}) {
		t.Errorf("overrides %v and variables %v", manifest.Overrides, manifest.Variables)
	}
}

func TestSinkAndSourceLocations(t *testing.T) {
	// The CLI module registers these; the engine only sees their fields.
	spec := func(kind string, fields map[string]interface{}) simulator.ComponentSpec {
		return simulator.ComponentSpec{Type: kind, Fields: fields}
	}
	sinks := map[string]simulator.ComponentSpec{
		"s3://runs/2026/a.arrow": spec("s3", map[string]interface{}{"bucket": "runs", "key": "2026/a.arrow", "format": "arrow"}),
		"out/a.duckdb#runs":      spec("duckdb", map[string]interface{}{"path": "out/a.duckdb", "table": "runs"}),
		"out/a.arrow":            spec("arrow", map[string]interface{}{"path": "out/a.arrow"}),
		"stdout":                 spec("stdout", nil),
		"":                       spec("nil", nil),
	}
	for want, sink := range sinks {
		if got := sinkLocation(sink); got != want {
			t.Errorf("%s sink at %q, want %q", sink.Type, got, want)
		}
	}
	sources := map[string]*DataSource{
		"s3 s3://data/x.csv": {Extra: map[string]map[string]interface{}{"s3": {"bucket": "data", "key": "x.csv"}}},
		"arrow in/x.arrow":   {Extra: map[string]map[string]interface{}{"arrow": {"path": "in/x.arrow"}}},
		"json_log in/x.log":  {JsonLog: &jsonLogSource{Path: "in/x.log"}},
		" ":                  {},
	}
	for want, source := range sources {
		if kind, location := sourceLocation(source); kind+" "+location != want {
			t.Errorf("source at %q, want %q", kind+" "+location, want)
		}
	}
	if got := redactURL("::not a url"); got != "" {
		t.Errorf("an unparseable URL should give no location, got %q", got)
	}
}

func TestInspectCLI(t *testing.T) {
	config := writeConfigPath(t, ensembleIOYAML)
	t.Run("inspect --io prints the manifest as JSON", func(t *testing.T) {
		err, printed := executeIn(t, t.TempDir(), "inspect", "--io", "-c", config, "--set", "run.seeds=[1, 2, 3]")
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := LoadConfig(config, WithSet("run.seeds", "[1, 2, 3]"))
		if err != nil {
			t.Fatal(err)
		}
		want, _ := json.MarshalIndent(Manifest(loaded), "", "  ")
		if printed != string(want)+"\n" {
			t.Errorf("printed\n%s\nwant\n%s", printed, want)
		}
		var manifest IOManifest
		if err := json.Unmarshal([]byte(printed), &manifest); err != nil {
			t.Fatal(err)
		}
		if got := manifest.Outputs[0].Locations; !slices.Equal(got,
			[]string{"out/run-0-1.log", "out/run-1-2.log", "out/run-2-3.log"}) {
			t.Errorf("per-member locations %v", got)
		}
	})
	for _, args := range [][]string{{"inspect", "-c", config}, {"inspect", "--io"}, {"inspect", "--io", "-c", config, "--bogus"}} {
		t.Run(strings.Join(args, " ")+" is a usage error", func(t *testing.T) {
			if err, _ := executeIn(t, t.TempDir(), args...); KindOf(err) != ErrUsage {
				t.Errorf("expected ErrUsage, got %v", err)
			}
		})
	}
	t.Run("inspect --io on a bad config is a config error", func(t *testing.T) {
		err, printed := executeIn(t, t.TempDir(), "inspect", "--io", "-c", writeConfigPath(t, "main: {partitons: []}\n"))
		if KindOf(err) != ErrConfig || printed != "" {
			t.Errorf("expected ErrConfig and nothing printed, got %v and %q", err, printed)
		}
	})
}

func TestManifestOfEveryMainRunReadAndWrite(t *testing.T) {
	config, err := LoadConfig(writeConfigPath(t, ioYAML))
	if err != nil {
		t.Fatal(err)
	}
	got, want := Manifest(config), &IOManifest{
		Config: config.sourcePath,
		Run:    ManifestRun{Mode: "batch", Runtime: "main"},
		Clock: &ManifestClock{Timestep: "from_input", TimestepInput: "prices",
			Termination: "input_exhausted", TerminationInput: "prices"},
		Inputs: []ManifestInput{
			{Name: "prices", Kind: "csv", Location: "data/prices.csv", Partitions: []string{"close"},
				ReadBy: []string{"main.partitions[name=price]", "main.partitions[name=close_again]",
					"main.simulation.timestep_function",
					"main.simulation.termination_condition"}},
			{Name: "rates", Kind: "json_log", Location: "data/rates.log", Partitions: []string{"rate"},
				ReadBy: []string{"main.partitions[name=walk].params_from_input.variances"}},
		},
		Outputs: []ManifestOutput{
			{Name: "log", DeclaredIn: "outputs", Condition: "every_step", Sink: "json_log",
				Location: "out/run.log"},
			{Name: "screen", DeclaredIn: "outputs", Condition: "only_given_partitions", Sink: "stdout",
				Location: "stdout"},
		},
		Embedded: []ManifestEmbedded{{Partition: "nested",
			Columns: []ManifestColumns{{Partition: "inner", Offset: 0, Width: 1}}}},
		Debug: []ManifestOutput{
			{Name: "nested", DeclaredIn: "embedded[name=nested].simulation", Condition: "every_step",
				Sink: "json_log", Location: "out/nested.log"},
		},
	}
	gotJSON, _ := json.MarshalIndent(got, "", "  ")
	wantJSON, _ := json.MarshalIndent(want, "", "  ")
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("manifest\n%s\nwant\n%s", gotJSON, wantJSON)
	}
}

func TestCheckWithTheSocketAlias(t *testing.T) {
	// --socket is rejected alongside an ensemble; --check says so as the run does.
	socket := writeConfigPath(t, "address: 127.0.0.1:0\nhandle: /\n")
	config := writeConfigPath(t, ensembleIOYAML)
	checked, _ := executeIn(t, t.TempDir(), "--config", config, "--socket", socket, "--check")
	ran, _ := executeIn(t, t.TempDir(), "--config", config, "--socket", socket)
	if KindOf(checked) != ErrUsage || ran == nil || ran.Error() != checked.Error() {
		t.Errorf("--check said %v, the run said %v", checked, ran)
	}
}

func TestMemberLocationsOnlyForAnEnsemble(t *testing.T) {
	batch := &ApiRunConfig{Run: RunModeConfig{Seeds: []uint64{1}}}
	if got := memberLocations(batch, "run-{member}.log"); got != nil {
		t.Errorf("a batch run has no members, got %v", got)
	}
}
