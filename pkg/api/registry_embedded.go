package api

import (
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gopkg.in/yaml.v2"
)

// embedded_simulation_run runs a whole run inside a partition, to termination
// at every outer step (general.EmbeddedSimulationRunIteration):
//
//	iteration:
//	  type: embedded_simulation_run
//	  run:
//	    partitions: [...]
//	    expressions: [...]
//	    simulation: {...}
//	    embedded: [...]
//
// It is the top-level embedded: block stated in place, so its run can host
// runs of its own, to any depth: an embedded: entry, or a partition whose
// iteration is another embedded_simulation_run. As for embedded:, the host
// forwards "<inner partition>/<param>" params into the run, feeds memory with
// "<inner partition>/update_from_partition_history", and its row is the inner
// partitions' final states, concatenated in order. The run is a black box: it
// writes no output of its own.

// nestedRun is a run stated inside a partition: an ordinary run with the
// embedded runs its partitions host.
type nestedRun struct {
	RunConfig `yaml:",inline"`
	Embedded  []EmbeddedRunConfig `yaml:"embedded,omitempty"`
}

type embeddedRunSpec struct {
	Run nestedRun `yaml:"run"`
}

func init() {
	scopedIterationBuilders["embedded_simulation_run"] = buildEmbeddedSimulationRun
}

// buildEmbeddedSimulationRun builds a nested run within its host's run scope:
// the nested run steps inside its host's step, so it is part of the same run.
func buildEmbeddedSimulationRun(
	fields map[string]interface{},
	scope *runScope,
) (simulator.Iteration, error) {
	config, err := decodeNestedRun(fields, scope)
	if err == nil {
		err = validateNestedRun(config)
	}
	if err == nil {
		err = checkRunWiring(config.configGenerator(false))
	}
	if err != nil {
		return nil, fmt.Errorf("embedded_simulation_run: %w", err)
	}
	return general.NewEmbeddedSimulationRunIteration(
		config.configGenerator(false).GenerateConfigs()), nil
}

// decodeNestedRun decodes and resolves an embedded_simulation_run's run, as a
// config with no inputs or outputs of its own.
func decodeNestedRun(fields map[string]interface{}, scope *runScope) (*ApiRunConfig, error) {
	// Fields decoded from YAML always marshal back (yaml.v2 panics, rather
	// than erring, on a value that cannot).
	if err := refuseNestedExpressionBooleans(fields); err != nil {
		return nil, err
	}
	raw, _ := yaml.Marshal(fields)
	var spec embeddedRunSpec
	if err := yaml.UnmarshalStrict(raw, &spec); err != nil {
		return nil, err
	}
	config := &ApiRunConfig{Main: spec.Run.RunConfig, Embedded: spec.Run.Embedded, scope: scope}
	for index := range config.Main.Partitions {
		config.Main.Partitions[index].Init()
	}
	for index := range config.Embedded {
		for pIndex := range config.Embedded[index].Run.Partitions {
			config.Embedded[index].Run.Partitions[pIndex].Init()
		}
		if err := config.Embedded[index].Run.resolveIn(true, scope); err != nil {
			return nil, err
		}
		config.Embedded[index].Run.recordInputReaders(scope)
	}
	if err := config.Main.resolveIn(true, scope); err != nil {
		return nil, err
	}
	config.Main.recordInputReaders(scope)
	return config, nil
}

// refuseNestedExpressionBooleans applies refuseBooleanValues to each expressions:
// entry of a nested run and of the embedded runs it states, which reach the typed
// decode through interface{} and so lose a name YAML read as a boolean.
func refuseNestedExpressionBooleans(fields map[string]interface{}) error {
	run, _ := fields["run"].(map[interface{}]interface{})
	runs := map[string]interface{}{"run": run}
	embedded, _ := run["embedded"].([]interface{})
	for i, entry := range embedded {
		if entry, ok := entry.(map[interface{}]interface{}); ok {
			runs[fmt.Sprintf("run.embedded[%d].run", i)] = entry["run"]
		}
	}
	for _, at := range sortedKeys(runs) {
		inner, _ := runs[at].(map[interface{}]interface{})
		if err := refuseBooleanValues(inner["expressions"], at+".expressions"); err != nil {
			return err
		}
	}
	return nil
}

// nestedRunOf decodes the run a partition's embedded_simulation_run states, for
// reading, once loading has resolved (and so checked) it. Decoding in the
// loaded config's scope reuses the environments it built, so a downstream's
// environment is not built again to be read; a config assembled in Go has no
// scope and decodes in a fresh one.
func nestedRunOf(partition simulator.PartitionConfig, scope *runScope) (*ApiRunConfig, bool) {
	if partition.IterationSpec.Type != "embedded_simulation_run" {
		return nil, false
	}
	if scope == nil {
		scope = newRunScope()
	}
	config, _ := decodeNestedRun(partition.IterationSpec.Fields, scope)
	return config, true
}

// validateNestedRun checks a nested run as a run of its own, except for the
// memory its partitions are fed, which the outermost config checks with their
// hosts in view (validateMemoryFeeds). It also refuses what a nested run cannot
// have: an output of its own, which the outer run's black-box rule leaves to
// top-level embedded: runs as a debug log, and params_from_input, which feeds
// only the main run.
func validateNestedRun(config *ApiRunConfig) error {
	if err := validateRunShape(config); err != nil {
		return err
	}
	runs := []RunConfig{config.Main}
	where := []string{"its run"}
	for _, embedded := range config.Embedded {
		runs = append(runs, embedded.Run)
		where = append(where, fmt.Sprintf("its embedded run %q", embedded.Name))
	}
	for index, run := range runs {
		if function := run.SimulationStrings.OutputFunction.Type; function != "" && function != "nil" {
			return fmt.Errorf("%s writes output_function %q, but a nested run is a black box: "+
				"its host's row is its partitions' final states; to see inside it, run it as a "+
				"top-level embedded: run, whose own output is a debug log", where[index], function)
		}
		for _, partition := range run.Partitions {
			if len(partition.ParamsFromInput) > 0 {
				return fmt.Errorf("params_from_input is not yet supported inside a nested "+
					"run (partition %q)", partition.Name)
			}
		}
	}
	return nil
}
