package api

import (
	"fmt"
	"sort"
	"strings"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// The inputs: tier (PLAN.md Phase 1). An input is a named, read-only storage
// that exists before the runtime is built: loaded from a source (a file, a
// database), or produced by a labelled pre-pass simulation. A main: partition
// replays one of an input's partitions with {type: from_input}; the run's clock
// can follow an input's timestamps with timestep_function {type: from_input}
// and stop when it runs out with termination_condition {type: input_exhausted}.
//
// Inputs are read when a run starts, never when a config is loaded: loading
// resolves those three specs to unbound placeholders and validates the names
// they refer to, and bindInputs loads the inputs and binds the placeholders
// immediately before the run's generator is built. A missing input is therefore
// ErrUnavailable and a partition the input lacks is ErrData, both at run time.

// InputConfig is one entry of inputs:. Exactly one field is set.
type InputConfig struct {
	// Source loads the input from a file or database (the data.source spellings).
	Source *DataSource `yaml:"source,omitempty"`
	// Simulation produces the input by running a sub-simulation first.
	Simulation *DataConfig `yaml:"simulation,omitempty"`
}

func (i *InputConfig) load() (*simulator.StateTimeStorage, error) {
	if i.Source != nil {
		storage, err := i.Source.load()
		return storage, inputError(err)
	}
	return i.Simulation.buildStorage()
}

// unboundInputIteration is what {type: from_input} resolves to at load: a
// placeholder bindInputs replaces with the input's rows when a run starts.
type unboundInputIteration struct {
	input     string
	partition string
}

func (u *unboundInputIteration) Configure(int, *simulator.Settings) {
	panic(fmt.Sprintf("api: a from_input partition (input %q) was run unbound; "+
		"run the config through api.Run, RunWith or RunToStorage, which bind "+
		"inputs when the run starts", u.input))
}

func (u *unboundInputIteration) Iterate(
	*simulator.Params, int, []*simulator.StateHistory, *simulator.CumulativeTimestepsHistory,
) []float64 {
	panic("api: unbound from_input partition")
}

// unboundInputTimesteps is timestep_function {type: from_input} before binding.
type unboundInputTimesteps struct{ input string }

func (u *unboundInputTimesteps) NextIncrement(*simulator.CumulativeTimestepsHistory) float64 {
	panic(fmt.Sprintf("api: timestep_function from_input (input %q) was run unbound", u.input))
}

// unboundInputExhausted is termination_condition {type: input_exhausted} before
// binding.
type unboundInputExhausted struct{ input string }

func (u *unboundInputExhausted) Terminate(
	[]*simulator.StateHistory, *simulator.CumulativeTimestepsHistory,
) bool {
	panic(fmt.Sprintf("api: termination_condition input_exhausted (input %q) was run unbound", u.input))
}

func buildFromInput(fields map[string]interface{}) (simulator.Iteration, error) {
	iteration := &unboundInputIteration{}
	for key, value := range fields {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("from_input: %s must be a string, got %T", key, value)
		}
		switch key {
		case "input":
			iteration.input = text
		case "partition":
			iteration.partition = text
		default:
			return nil, fmt.Errorf("from_input: unknown field %q", key)
		}
	}
	if iteration.input == "" {
		return nil, fmt.Errorf("from_input: needs input: naming an entry of inputs:")
	}
	return iteration, nil
}

// inputNameField reads the single required "input" field of the from_input
// timestep function and the input_exhausted termination condition.
func inputNameField(kind string, spec simulator.ComponentSpec) (string, error) {
	for key := range spec.Fields {
		if key != "input" {
			return "", fmt.Errorf("%s: unknown field %q", kind, key)
		}
	}
	name, ok := spec.Fields["input"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("%s: needs input: naming an entry of inputs:", kind)
	}
	return name, nil
}

func registerInputComponents() {
	iterationBuilders["from_input"] = buildFromInput
	simulator.RegisterComponent("timestep_function", "from_input",
		func(spec simulator.ComponentSpec) (interface{}, error) {
			name, err := inputNameField("timestep_function from_input", spec)
			return &unboundInputTimesteps{input: name}, err
		})
	simulator.RegisterComponent("termination_condition", "input_exhausted",
		func(spec simulator.ComponentSpec) (interface{}, error) {
			name, err := inputNameField("termination_condition input_exhausted", spec)
			return &unboundInputExhausted{input: name}, err
		})
}

func init() {
	registerInputComponents()
}

// validateInputs checks, at load, that the inputs: block and every reference
// to it are coherent — without reading any input.
func validateInputs(config *ApiRunConfig) error {
	for name, input := range config.Inputs {
		switch {
		case (input.Source == nil) == (input.Simulation == nil):
			return fmt.Errorf("api: input %q needs exactly one of source: or simulation:", name)
		case input.Simulation != nil && input.Simulation.Source != nil:
			return fmt.Errorf("api: input %q: a simulation: input cannot also set source:", name)
		}
	}
	if len(config.Inputs) > 0 && len(config.Macros) > 0 {
		return fmt.Errorf("api: inputs: is not yet available to macros:; use data:")
	}
	used := map[string]bool{}
	reference := func(where, input string) error {
		if _, ok := config.Inputs[input]; !ok {
			return fmt.Errorf("api: %s names input %q, which inputs: does not declare", where, input)
		}
		used[input] = true
		return nil
	}
	for _, partition := range config.Main.Partitions {
		if unbound, ok := partition.Iteration.(*unboundInputIteration); ok {
			if err := reference(fmt.Sprintf("partition %q", partition.Name), unbound.input); err != nil {
				return err
			}
		}
	}
	for _, embedded := range config.Embedded {
		for _, partition := range embedded.Run.Partitions {
			if _, ok := partition.Iteration.(*unboundInputIteration); ok {
				return fmt.Errorf("api: from_input is not yet supported inside embedded "+
					"runs (partition %q of %q)", partition.Name, embedded.Name)
			}
		}
	}
	if timesteps, ok := config.Main.Simulation.TimestepFunction.(*unboundInputTimesteps); ok {
		if err := reference("timestep_function from_input", timesteps.input); err != nil {
			return err
		}
	}
	if exhausted, ok := config.Main.Simulation.TerminationCondition.(*unboundInputExhausted); ok {
		if err := reference("termination_condition input_exhausted", exhausted.input); err != nil {
			return err
		}
	}
	for name := range config.Inputs {
		if !used[name] {
			return fmt.Errorf("api: input %q is never used; replay it with "+
				"{type: from_input, input: %s} or remove it", name, name)
		}
	}
	return nil
}

// bindInputs loads the inputs a config's main run uses and binds its
// placeholders: each from_input partition to its input partition's rows (its
// init_state_values defaulting to the first row), a from_input timestep function
// to the input's times, and input_exhausted to the input's length. It runs when
// a run starts; a config with nothing to bind is left alone, and binding twice
// is a no-op. Failures are classified: a missing input is ErrUnavailable, a
// partition the input lacks or a width mismatch is ErrData.
func bindInputs(config *ApiRunConfig) error {
	storages := map[string]*simulator.StateTimeStorage{}
	storageOf := func(name string) (*simulator.StateTimeStorage, error) {
		if storage, ok := storages[name]; ok {
			return storage, nil
		}
		input := config.Inputs[name]
		storage, err := input.load()
		if err != nil {
			return nil, fmt.Errorf("api: loading input %q: %w", name, err)
		}
		storages[name] = storage
		return storage, nil
	}
	for index := range config.Main.Partitions {
		partition := &config.Main.Partitions[index]
		unbound, ok := partition.Iteration.(*unboundInputIteration)
		if !ok {
			continue
		}
		storage, err := storageOf(unbound.input)
		if err != nil {
			return err
		}
		source := unbound.partition
		if source == "" {
			source = partition.Name
		}
		rows, err := inputRows(storage, unbound.input, source)
		if err != nil {
			return err
		}
		switch {
		case len(partition.InitStateValues) == 0:
			partition.InitStateValues = append([]float64(nil), rows[0]...)
		case len(partition.InitStateValues) != len(rows[0]):
			return withKind(ErrData, fmt.Errorf(
				"api: partition %q has %d init_state_values but input %q's %q rows have %d",
				partition.Name, len(partition.InitStateValues), unbound.input, source, len(rows[0])))
		}
		partition.Iteration = &general.FromStorageIteration{Data: rows}
	}
	if unbound, ok := config.Main.Simulation.TimestepFunction.(*unboundInputTimesteps); ok {
		storage, err := storageOf(unbound.input)
		if err != nil {
			return err
		}
		config.Main.Simulation.TimestepFunction = &general.FromStorageTimestepFunction{
			Data: storage.GetTimes()}
	}
	if unbound, ok := config.Main.Simulation.TerminationCondition.(*unboundInputExhausted); ok {
		storage, err := storageOf(unbound.input)
		if err != nil {
			return err
		}
		config.Main.Simulation.TerminationCondition = &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: len(storage.GetTimes()) - 1}
	}
	return nil
}

// inputRows returns an input partition's rows, or ErrData naming what the input
// does contain.
func inputRows(storage *simulator.StateTimeStorage, input, partition string) ([][]float64, error) {
	names := storage.GetNames()
	sort.Strings(names)
	for _, name := range names {
		if name == partition {
			rows := storage.GetValues(name)
			if len(rows) == 0 {
				return nil, withKind(ErrData, fmt.Errorf(
					"api: input %q partition %q has no rows", input, partition))
			}
			return rows, nil
		}
	}
	return nil, withKind(ErrData, fmt.Errorf(
		"api: input %q has no partition %q (it has: %s)", input, partition, strings.Join(names, ", ")))
}
