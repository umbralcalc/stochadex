package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func didPanic(f func()) (panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
		}
	}()
	f()
	return panicked
}

// Item 7: validateApiRunConfigStrings guards the code-generation contract that
// every partition either declares an Iteration or is backed by an embedded run
// of the same name.
func TestValidateApiRunConfigStrings(t *testing.T) {
	t.Run(
		"a partition with no iteration and no matching embedded run panics",
		func(t *testing.T) {
			config := &ApiRunConfigStrings{
				Main: RunConfigStrings{
					Partitions: []PartitionConfigStrings{
						{Name: "has_iteration", Iteration: simulator.ComponentSpec{GoExpr: "&continuous.WienerProcessIteration{}"}},
						{Name: "orphan"}, // no iteration, no embedded run
					},
				},
			}
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("expected a panic for a partition with no iteration")
				}
				got := stringify(r)
				if !strings.Contains(got, "orphan") ||
					!strings.Contains(got, "no embedded simulation runs") {
					t.Errorf("unhelpful panic message: %q", got)
				}
			}()
			validateApiRunConfigStrings(config)
		},
	)
	t.Run(
		"a partition with no iteration is valid when an embedded run matches",
		func(t *testing.T) {
			config := &ApiRunConfigStrings{
				Main: RunConfigStrings{
					Partitions: []PartitionConfigStrings{
						{Name: "embedded_sim"}, // no iteration, but matched below
					},
				},
				Embedded: []EmbeddedRunConfigStrings{
					{Name: "embedded_sim"},
				},
			}
			if didPanic(func() { validateApiRunConfigStrings(config) }) {
				t.Error("validation panicked despite a matching embedded run")
			}
		},
	)
	t.Run(
		"all partitions declaring an iteration is valid",
		func(t *testing.T) {
			config := &ApiRunConfigStrings{
				Main: RunConfigStrings{
					Partitions: []PartitionConfigStrings{
						{Name: "a", Iteration: simulator.ComponentSpec{GoExpr: "&continuous.WienerProcessIteration{}"}},
						{Name: "b", Iteration: simulator.ComponentSpec{GoExpr: "&general.ConstantValuesIteration{}"}},
					},
				},
			}
			if didPanic(func() { validateApiRunConfigStrings(config) }) {
				t.Error("validation panicked on a fully-specified config")
			}
		},
	)
}

// Item 8: the YAML loaders parse configuration off disk, initialise partition
// defaults, and enforce the validation contract.
func TestLoadApiRunConfigFromYaml(t *testing.T) {
	t.Run(
		"loads partitions and initialises their defaults",
		func(t *testing.T) {
			config := LoadApiRunConfigFromYaml("test_program_config.yaml")

			if len(config.Main.Partitions) != 3 {
				t.Fatalf("got %d partitions, want 3", len(config.Main.Partitions))
			}
			names := []string{
				config.Main.Partitions[0].Name,
				config.Main.Partitions[1].Name,
				config.Main.Partitions[2].Name,
			}
			want := []string{"first_wiener_process", "second_wiener_process", "other_thing"}
			for i := range want {
				if names[i] != want[i] {
					t.Errorf("partition %d: got %q, want %q", i, names[i], want[i])
				}
			}
			if got := len(config.Main.Partitions[0].InitStateValues); got != 5 {
				t.Errorf("first partition init state width: got %d, want 5", got)
			}
			// Init() must have replaced the nil upstream map from YAML so that
			// wiring code can index it safely.
			if config.Main.Partitions[0].ParamsFromUpstream == nil {
				t.Error("LoadApiRunConfigFromYaml did not Init() partition defaults")
			}
		},
	)
	t.Run(
		"panics on a missing file",
		func(t *testing.T) {
			if !didPanic(func() { LoadApiRunConfigFromYaml("does_not_exist.yaml") }) {
				t.Error("expected a panic loading a nonexistent file")
			}
		},
	)
}

func TestLoadApiRunConfigStringsFromYaml(t *testing.T) {
	t.Run(
		"loads and validates a well-formed templated config",
		func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			yaml := `main:
  partitions:
  - name: wiener
    iteration: "&continuous.WienerProcessIteration{}"
  simulation:
    output_condition: "&simulator.NilOutputCondition{}"
`
			if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
				t.Fatal(err)
			}
			config := LoadApiRunConfigStringsFromYaml(path)
			if len(config.Main.Partitions) != 1 ||
				config.Main.Partitions[0].Name != "wiener" {
				t.Fatalf("unexpected parse result: %+v", config.Main.Partitions)
			}
			if config.Main.Partitions[0].Iteration.GoExpr !=
				"&continuous.WienerProcessIteration{}" {
				t.Errorf("iteration string not parsed: %q",
					config.Main.Partitions[0].Iteration.GoExpr)
			}
		},
	)
	t.Run(
		"propagates the validation panic for an orphan partition",
		func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			// No iteration and no embedded run of the same name.
			yaml := `main:
  partitions:
  - name: orphan
`
			if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
				t.Fatal(err)
			}
			if !didPanic(func() { LoadApiRunConfigStringsFromYaml(path) }) {
				t.Error("expected validation to panic for an orphan partition")
			}
		},
	)
}

// Item 9: ApiRunConfig.GetConfigGenerator swaps any partition whose name matches
// an embedded run for an embedded-simulation iteration wired to that run.
func TestApiRunConfigGetConfigGeneratorEmbeddedSwap(t *testing.T) {
	t.Run(
		"a named partition is replaced by an embedded simulation iteration",
		func(t *testing.T) {
			simulation := func() simulator.SimulationConfig {
				return simulator.SimulationConfig{
					OutputCondition: &simulator.NilOutputCondition{},
					OutputFunction:  &simulator.NilOutputFunction{},
					TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
						MaxNumberOfSteps: 2,
					},
					TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
					InitTimeValue:    0.0,
				}
			}
			newPartition := func(name string, iteration simulator.Iteration) simulator.PartitionConfig {
				p := simulator.PartitionConfig{
					Name:              name,
					Iteration:         iteration,
					Params:            simulator.NewParams(make(map[string][]float64)),
					InitStateValues:   []float64{0.0},
					StateHistoryDepth: 1,
				}
				p.Init()
				return p
			}

			// The host partition for an embedded run needs the params the
			// embedded-simulation iteration reads in Configure. Its own
			// iteration is discarded by the swap, so any type will do.
			host := newPartition("embedded_sim", &continuous.WienerProcessIteration{})
			host.Params.Set("burn_in_steps", []float64{0})

			// The partitions that actually run use a param-free iteration.
			config := &ApiRunConfig{
				Main: RunConfig{
					Partitions: []simulator.PartitionConfig{
						newPartition("plain", &general.ConstantValuesIteration{}),
						host,
					},
					Simulation: simulation(),
				},
				Embedded: []EmbeddedRunConfig{
					{
						Name: "embedded_sim",
						Run: RunConfig{
							Partitions: []simulator.PartitionConfig{
								newPartition("inner", &general.ConstantValuesIteration{}),
							},
							Simulation: simulation(),
						},
					},
				},
			}

			generator := config.GetConfigGenerator()

			// The matched partition must now carry an embedded-simulation
			// iteration; the unmatched one must be untouched.
			swapped := generator.GetPartition("embedded_sim").Iteration
			if _, ok := swapped.(*general.EmbeddedSimulationRunIteration); !ok {
				t.Errorf("embedded_sim iteration was not swapped: got %T", swapped)
			}
			plain := generator.GetPartition("plain").Iteration
			if _, ok := plain.(*general.ConstantValuesIteration); !ok {
				t.Errorf("plain partition was unexpectedly swapped: got %T", plain)
			}

			// The generated config must still run end to end.
			simulator.NewPartitionCoordinator(generator.GenerateConfigs()).Run()
		},
	)
}

// stringify renders a recovered panic value for substring assertions.
func stringify(v any) string {
	if err, ok := v.(error); ok {
		return err.Error()
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
