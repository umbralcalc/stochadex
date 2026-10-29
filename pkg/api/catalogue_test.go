package api

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// catalogueNames returns the names of a catalogue list.
func catalogueNames(list []CatalogueEntry) []string {
	names := make([]string, len(list))
	for i, entry := range list {
		names[i] = entry.Name
	}
	return names
}

// catalogueLists returns every list in a catalogue, by its JSON key.
func catalogueLists(t *testing.T, catalogue Catalogue) map[string][]CatalogueEntry {
	t.Helper()
	lists := map[string][]CatalogueEntry{}
	value := reflect.ValueOf(catalogue)
	for i := 0; i < value.NumField(); i++ {
		if list, ok := value.Field(i).Interface().([]CatalogueEntry); ok {
			key, _, _ := strings.Cut(value.Type().Field(i).Tag.Get("json"), ",")
			lists[key] = list
		}
	}
	return lists
}

func TestCatalogue(t *testing.T) {
	t.Run("the CLI prints the catalogue, identically every time", func(t *testing.T) {
		run := func() string {
			var err error
			out := captureStdout(t, func() {
				err = Execute([]string{"stochadex", "catalogue"})
			})
			if err != nil {
				t.Fatal(err)
			}
			return out
		}
		first := run()
		for range 5 {
			if again := run(); again != first {
				t.Fatalf("catalogue output changed between runs:\n%s", firstDifference(
					[]byte(first), []byte(again)))
			}
		}
		var decoded Catalogue
		if err := json.Unmarshal([]byte(first), &decoded); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(decoded, BuildCatalogue()) {
			t.Error("the CLI's catalogue is not BuildCatalogue's")
		}
		if decoded.CatalogueVersion != CatalogueVersion || decoded.Version != BuildVersion {
			t.Errorf("got catalogue_version %d, version %q", decoded.CatalogueVersion, decoded.Version)
		}
	})

	t.Run("it reports the build's version and features, sorted", func(t *testing.T) {
		version, features := BuildVersion, BuildFeatures
		defer func() { BuildVersion, BuildFeatures = version, features }()
		BuildVersion, BuildFeatures = "v9.9.9", []string{"s3", "arrow"}
		catalogue := BuildCatalogue()
		if catalogue.Version != "v9.9.9" || !slices.Equal(catalogue.Features, []string{"arrow", "s3"}) {
			t.Errorf("got %q %v", catalogue.Version, catalogue.Features)
		}
		BuildFeatures = nil
		data, _ := json.Marshal(BuildCatalogue())
		if !strings.Contains(string(data), `"features":[]`) {
			t.Errorf("no features must be an empty list, not null: %s", data)
		}
	})

	t.Run("every list is sorted, distinct and non-empty", func(t *testing.T) {
		for key, list := range catalogueLists(t, BuildCatalogue()) {
			names := catalogueNames(list)
			if len(names) == 0 {
				t.Errorf("%s is empty", key)
			}
			if !slices.IsSorted(names) || len(slices.Compact(slices.Clone(names))) != len(names) {
				t.Errorf("%s is not sorted and distinct: %v", key, names)
			}
		}
	})

	t.Run("every listed name resolves", func(t *testing.T) {
		catalogue := BuildCatalogue()
		unknown := func(err error) bool {
			return err != nil && strings.Contains(err.Error(), "unknown")
		}
		check := func(kind string, list []CatalogueEntry, resolve func(simulator.ComponentSpec) error) {
			for _, name := range catalogueNames(list) {
				// Its fields are missing, but its type must be known.
				if err := resolve(simulator.ComponentSpec{Type: name}); unknown(err) &&
					strings.Contains(err.Error(), `"`+name+`"`) {
					t.Errorf("%s %s is listed but unknown: %v", kind, name, err)
				}
			}
			if err := resolve(simulator.ComponentSpec{Type: "no_such_type"}); !unknown(err) {
				t.Errorf("%s: an unknown type gave %v", kind, err)
			}
		}
		check("iteration", catalogue.Iterations, func(spec simulator.ComponentSpec) error {
			_, err := resolveIterationIn(spec, newRunScope())
			return err
		})
		check("kernel", catalogue.Kernels, func(spec simulator.ComponentSpec) error {
			_, err := resolveKernel(spec)
			return err
		})
		check("likelihood", catalogue.Likelihoods, func(spec simulator.ComponentSpec) error {
			_, err := resolveLikelihood(spec)
			return err
		})
		check("prior", catalogue.Priors, func(spec simulator.ComponentSpec) error {
			_, err := resolvePrior(spec)
			return err
		})
		check("jump", catalogue.Jumps, func(spec simulator.ComponentSpec) error {
			_, err := resolveJump(spec)
			return err
		})
		check("environment", catalogue.Environments, func(spec simulator.ComponentSpec) error {
			_, err := resolveMCTSEnvironment(spec, newRunScope())
			return err
		})
		for _, name := range catalogueNames(catalogue.Macros) {
			if macroSpecFactories[name] == nil {
				t.Errorf("macro %s is listed but has no spec", name)
			}
		}
	})

	t.Run("the simulation component families are the simulator's", func(t *testing.T) {
		keys := map[string]string{
			"execution_strategy":    "execution_strategies",
			"output_condition":      "output_conditions",
			"output_function":       "output_functions",
			"termination_condition": "termination_conditions",
			"timestep_function":     "timestep_functions",
		}
		lists := catalogueLists(t, BuildCatalogue())
		for _, family := range simulator.ComponentFamilies {
			key, ok := keys[family]
			if !ok {
				t.Errorf("family %s is not in the catalogue", family)
				continue
			}
			if got := catalogueNames(lists[key]); !slices.Equal(got, simulator.ComponentTypes(family)) {
				t.Errorf("%s: got %v, want %v", key, got, simulator.ComponentTypes(family))
			}
		}
		// The engine's own registrations from this package are among them.
		for key, name := range map[string]string{
			"timestep_functions":     "from_input",
			"termination_conditions": "input_exhausted",
			"output_functions":       "postgres",
		} {
			if !slices.Contains(catalogueNames(lists[key]), name) {
				t.Errorf("%s has no %s", key, name)
			}
		}
	})

	t.Run("sources and transports are the config's own spellings plus registered ones", func(t *testing.T) {
		catalogue := BuildCatalogue()
		// Other tests in this package register sources of their own.
		for _, name := range []string{"csv", "inline", "json_log", "postgres"} {
			if got := catalogueNames(catalogue.DataSources); !slices.Contains(got, name) {
				t.Errorf("data_sources has no %s: %v", name, got)
			}
		}
		if got := catalogueNames(catalogue.StreamTransports); !slices.Contains(got, "websocket") ||
			!slices.Contains(got, "connection") {
			t.Errorf("stream_transports: %v", got)
		}
	})

	t.Run("downstream registrations are listed", func(t *testing.T) {
		RegisterIteration("test_catalogue_iteration",
			func(simulator.ComponentSpec) (simulator.Iteration, error) { return nil, nil })
		defer delete(extraIterationBuilders, "test_catalogue_iteration")
		RegisterDataSource("test_catalogue_source",
			func(map[string]interface{}) (*simulator.StateTimeStorage, error) { return nil, nil })
		defer delete(extraDataSources, "test_catalogue_source")
		RegisterEnvironment("test_catalogue_environment",
			func(simulator.ComponentSpec) (MCTSEnvironment, error) { return nil, nil })
		defer delete(environmentBuilders, "test_catalogue_environment")
		RegisterStreamTransport("test_catalogue_transport", StreamTransport{})
		defer func() {
			streamTransportsMu.Lock()
			delete(streamTransports, "test_catalogue_transport")
			streamTransportsMu.Unlock()
		}()

		catalogue := BuildCatalogue()
		for list, name := range map[string]string{
			"iterations":        "test_catalogue_iteration",
			"data_sources":      "test_catalogue_source",
			"environments":      "test_catalogue_environment",
			"stream_transports": "test_catalogue_transport",
		} {
			if !slices.Contains(catalogueNames(catalogueLists(t, catalogue)[list]), name) {
				t.Errorf("%s does not list %s", list, name)
			}
		}
	})

	t.Run("every registry in this package is read", func(t *testing.T) {
		// Drift guard: a package-level registry map — a *Builders or
		// *Factories var, or an extra* map filled by a Register function —
		// must be read by BuildCatalogue, or a new registry would be left out.
		source, err := os.ReadFile("catalogue.go")
		if err != nil {
			t.Fatal(err)
		}
		files, err := filepath.Glob("*.go")
		if err != nil {
			t.Fatal(err)
		}
		found := 0
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, decl := range parsed.Decls {
				general, ok := decl.(*ast.GenDecl)
				if !ok || general.Tok != token.VAR {
					continue
				}
				for _, spec := range general.Specs {
					for _, name := range spec.(*ast.ValueSpec).Names {
						registry := strings.HasSuffix(name.Name, "Builders") ||
							strings.HasSuffix(name.Name, "Factories") ||
							(strings.HasPrefix(name.Name, "extra") && file != "catalogue.go") ||
							name.Name == "streamTransports"
						if !registry {
							continue
						}
						found++
						if !strings.Contains(string(source), name.Name) {
							t.Errorf("registry %s (%s) is not read by the catalogue", name.Name, file)
						}
					}
				}
			}
		}
		if found < 10 {
			t.Fatalf("found only %d registries; the scan is broken", found)
		}
	})
}
