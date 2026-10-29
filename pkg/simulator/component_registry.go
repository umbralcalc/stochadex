package simulator

import (
	"fmt"
	"sort"
)

// ComponentSpec is the value for a framework component in a config: a data spec
// ({type: every_step, ...}) resolved at load time by the registry below, needing
// no Go toolchain. A partition's bespoke maths goes through expressions: instead;
// the framework's own catalogue is named here by type.
type ComponentSpec struct {
	// Type is the data-spec discriminator (its "type" key).
	Type string
	// Fields holds the remaining data-spec keys (everything but "type").
	Fields map[string]interface{}
}

// UnmarshalYAML accepts a mapping with a non-empty string "type" key.
func (c *ComponentSpec) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var asMap map[string]interface{}
	if err := unmarshal(&asMap); err != nil {
		return fmt.Errorf(
			"component spec must be a {type: ...} mapping: %w",
			err,
		)
	}
	kind, ok := asMap["type"].(string)
	if !ok || kind == "" {
		return fmt.Errorf(
			"component data spec needs a non-empty string 'type' key, got: %v", asMap,
		)
	}
	delete(asMap, "type")
	for key, value := range asMap {
		if err := refuseBooleanKeys(value, key); err != nil {
			return fmt.Errorf("component spec {type: %s}: %w", kind, err)
		}
	}
	c.Type = kind
	c.Fields = asMap
	return nil
}

// refuseBooleanKeys rejects a mapping key, anywhere in a spec's value, that YAML 1.1
// read as a boolean. A spec's fields are decoded into interface{} rather than into a
// typed struct, and there yaml.v2 reads an unquoted y, n, yes, no, on or off (as well
// as true and false) as a boolean; re-encoded for the typed decode that follows, a
// key written n becomes false, and the name the config gave is lost. at is the path
// to value, for the message.
func refuseBooleanKeys(value interface{}, at string) error {
	switch typed := value.(type) {
	case map[interface{}]interface{}:
		for key, inner := range typed {
			if boolean, ok := key.(bool); ok {
				return fmt.Errorf("%s has a key that YAML reads as the boolean %v (an "+
					"unquoted y, n, yes, no, on, off, true or false is one); quote the "+
					"key to keep it a name", at, boolean)
			}
			if err := refuseBooleanKeys(inner, fmt.Sprintf("%s.%v", at, key)); err != nil {
				return err
			}
		}
	case []interface{}:
		for i, inner := range typed {
			if err := refuseBooleanKeys(inner, fmt.Sprintf("%s[%d]", at, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

// IsZero reports whether the spec was never populated (the YAML omitted it).
func (c ComponentSpec) IsZero() bool {
	return c.Type == "" && c.Fields == nil
}

// IsData reports whether the spec was populated (a {type: ...} data spec).
func (c ComponentSpec) IsData() bool { return c.Type != "" }

// fieldReader consumes a data spec's fields with strict key checking, so a
// mistyped or unknown field (e.g. a misspelled "stepsize") is rejected rather
// than silently ignored — the property the dead-key check gives the rest of the
// config, applied inside a component spec.
type fieldReader struct {
	specType string
	fields   map[string]interface{}
	used     map[string]bool
	err      error
}

func newFieldReader(specType string, fields map[string]interface{}) *fieldReader {
	return &fieldReader{
		specType: specType,
		fields:   fields,
		used:     make(map[string]bool, len(fields)),
	}
}

func (r *fieldReader) float(key string) float64 {
	r.used[key] = true
	value, ok := r.fields[key]
	if !ok {
		r.fail("missing required field %q", key)
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	default:
		r.fail("field %q must be a number, got %T", key, value)
		return 0
	}
}

func (r *fieldReader) int(key string) int {
	r.used[key] = true
	value, ok := r.fields[key]
	if !ok {
		r.fail("missing required field %q", key)
		return 0
	}
	if typed, ok := value.(int); ok {
		return typed
	}
	r.fail("field %q must be an integer, got %T", key, value)
	return 0
}

func (r *fieldReader) uint64(key string) uint64 {
	return uint64(r.int(key))
}

func (r *fieldReader) str(key string) string {
	r.used[key] = true
	value, ok := r.fields[key]
	if !ok {
		r.fail("missing required field %q", key)
		return ""
	}
	if typed, ok := value.(string); ok {
		return typed
	}
	r.fail("field %q must be a string, got %T", key, value)
	return ""
}

func (r *fieldReader) stringSlice(key string) []string {
	r.used[key] = true
	value, ok := r.fields[key]
	if !ok {
		r.fail("missing required field %q", key)
		return nil
	}
	raw, ok := value.([]interface{})
	if !ok {
		r.fail("field %q must be a list, got %T", key, value)
		return nil
	}
	out := make([]string, len(raw))
	for i, element := range raw {
		typed, ok := element.(string)
		if !ok {
			r.fail("field %q element %d must be a string, got %T", key, i, element)
			return nil
		}
		out[i] = typed
	}
	return out
}

func (r *fieldReader) fail(format string, args ...interface{}) {
	if r.err == nil {
		r.err = fmt.Errorf("component %q: "+format, append([]interface{}{r.specType}, args...)...)
	}
}

// done returns any accumulated error, plus an error for any field left unconsumed
// (an unknown key).
func (r *fieldReader) done() error {
	if r.err != nil {
		return r.err
	}
	for key := range r.fields {
		if !r.used[key] {
			return fmt.Errorf("component %q: unknown field %q", r.specType, key)
		}
	}
	return nil
}

// componentBuilder constructs a built-in component of one family from its data
// spec's fields, consuming them through reader so unknown keys are rejected.
type componentBuilder[T any] func(reader *fieldReader) T

// outputConditionBuilders are the built-in output_condition types.
var outputConditionBuilders = map[string]componentBuilder[OutputCondition]{
	"nil":           func(*fieldReader) OutputCondition { return &NilOutputCondition{} },
	"every_step":    func(*fieldReader) OutputCondition { return &EveryStepOutputCondition{} },
	"every_n_steps": func(r *fieldReader) OutputCondition { return &EveryNStepsOutputCondition{N: r.int("n")} },
	"only_given_partitions": func(r *fieldReader) OutputCondition {
		names := r.stringSlice("partitions")
		partitions := make(map[string]bool, len(names))
		for _, name := range names {
			partitions[name] = true
		}
		return &OnlyGivenPartitionsOutputCondition{Partitions: partitions}
	},
}

// outputFunctionBuilders are the built-in output_function types. Live-object
// sinks (state storage, channel, a server-side websocket connection) have no
// data form and are absent; a websocket the run pushes to as a client does.
var outputFunctionBuilders = map[string]componentBuilder[OutputFunction]{
	"nil":       func(*fieldReader) OutputFunction { return &NilOutputFunction{} },
	"stdout":    func(*fieldReader) OutputFunction { return &StdoutOutputFunction{} },
	"json_log":  func(r *fieldReader) OutputFunction { return NewJsonLogOutputFunction(r.str("path")) },
	"websocket": func(r *fieldReader) OutputFunction { return NewWebsocketPushOutputFunction(r.str("url")) },
}

// terminationConditionBuilders are the built-in termination_condition types.
var terminationConditionBuilders = map[string]componentBuilder[TerminationCondition]{
	"number_of_steps": func(r *fieldReader) TerminationCondition {
		return &NumberOfStepsTerminationCondition{MaxNumberOfSteps: r.int("max_steps")}
	},
	"time_elapsed": func(r *fieldReader) TerminationCondition {
		return &TimeElapsedTerminationCondition{MaxTimeElapsed: r.float("max_time_elapsed")}
	},
}

// timestepFunctionBuilders are the built-in timestep_function types.
var timestepFunctionBuilders = map[string]componentBuilder[TimestepFunction]{
	"constant": func(r *fieldReader) TimestepFunction {
		return &ConstantTimestepFunction{Stepsize: r.float("stepsize")}
	},
	"exponential_distribution": func(r *fieldReader) TimestepFunction {
		return &ExponentialDistributionTimestepFunction{
			Mean: r.float("mean"),
			Seed: r.uint64("seed"),
		}
	},
}

// executionStrategyBuilders are the built-in execution_strategy types: three
// zero-field structs, so each is a nullary construction.
var executionStrategyBuilders = map[string]componentBuilder[ExecutionStrategy]{
	"spawn_per_step":    func(*fieldReader) ExecutionStrategy { return &SpawnPerStepExecution{} },
	"persistent_worker": func(*fieldReader) ExecutionStrategy { return &PersistentWorkerExecution{} },
	"inline":            func(*fieldReader) ExecutionStrategy { return &InlineExecution{} },
}

// resolveComponent builds a component of family from its data spec: a built-in
// type from builders, else one registered downstream (RegisterComponent).
func resolveComponent[T any](
	family string, builders map[string]componentBuilder[T], spec ComponentSpec,
) (T, error) {
	if build, ok := builders[spec.Type]; ok {
		reader := newFieldReader(spec.Type, spec.Fields)
		result := build(reader)
		return result, reader.done()
	}
	var zero T
	if value, ok, err := resolveExtra(family, spec); ok {
		if err != nil {
			return zero, err
		}
		return value.(T), nil
	}
	return zero, unknownType(family, spec.Type)
}

// ComponentFamilies are the families of simulation component a config names by
// type, as RegisterComponent and ComponentTypes take them.
var ComponentFamilies = []string{
	"execution_strategy",
	"output_condition",
	"output_function",
	"termination_condition",
	"timestep_function",
}

// ComponentTypes returns the sorted type names a family resolves: its built-in
// types and those registered downstream with RegisterComponent. It returns nil
// for a family not in ComponentFamilies.
func ComponentTypes(family string) []string {
	var builtin []string
	switch family {
	case "execution_strategy":
		builtin = keysOf(executionStrategyBuilders)
	case "output_condition":
		builtin = keysOf(outputConditionBuilders)
	case "output_function":
		builtin = keysOf(outputFunctionBuilders)
	case "termination_condition":
		builtin = keysOf(terminationConditionBuilders)
	case "timestep_function":
		builtin = keysOf(timestepFunctionBuilders)
	default:
		return nil
	}
	for name := range extraComponentBuilders[family] {
		builtin = append(builtin, name)
	}
	sort.Strings(builtin)
	return builtin
}

func keysOf[T any](builders map[string]componentBuilder[T]) []string {
	names := make([]string, 0, len(builders))
	for name := range builders {
		names = append(names, name)
	}
	return names
}

// ResolveOutputCondition builds an OutputCondition from a data spec.
func ResolveOutputCondition(spec ComponentSpec) (OutputCondition, error) {
	return resolveComponent("output_condition", outputConditionBuilders, spec)
}

// ResolveOutputFunction builds an OutputFunction from a data spec.
func ResolveOutputFunction(spec ComponentSpec) (OutputFunction, error) {
	return resolveComponent("output_function", outputFunctionBuilders, spec)
}

// ResolveTerminationCondition builds a TerminationCondition from a data spec.
func ResolveTerminationCondition(spec ComponentSpec) (TerminationCondition, error) {
	return resolveComponent("termination_condition", terminationConditionBuilders, spec)
}

// extraComponentBuilders holds data-spec builders for framework components that
// live in packages downstream of simulator (e.g. a general.FromHistoryTimestepFunction).
// A downstream package registers them from an init() so the four Resolve
// functions below can reach them without simulator importing that package. Keyed
// by family then type name.
var extraComponentBuilders = map[string]map[string]func(ComponentSpec) (interface{}, error){}

// RegisterComponent registers a data-spec builder for a component that lives
// downstream of simulator. family is one of ComponentFamilies. Call it from an init(); it panics
// on a duplicate so two packages cannot silently claim one name.
func RegisterComponent(family, typeName string, build func(ComponentSpec) (interface{}, error)) {
	if extraComponentBuilders[family] == nil {
		extraComponentBuilders[family] = map[string]func(ComponentSpec) (interface{}, error){}
	}
	if _, exists := extraComponentBuilders[family][typeName]; exists {
		panic("simulator: duplicate component registration " + family + "/" + typeName)
	}
	extraComponentBuilders[family][typeName] = build
}

// resolveExtra looks a downstream-registered component up by family and type,
// returning (value, true, err) when a builder is found.
func resolveExtra(family string, spec ComponentSpec) (interface{}, bool, error) {
	build, ok := extraComponentBuilders[family][spec.Type]
	if !ok {
		return nil, false, nil
	}
	value, err := build(spec)
	return value, true, err
}

// ResolveTimestepFunction builds a TimestepFunction from a data spec.
func ResolveTimestepFunction(spec ComponentSpec) (TimestepFunction, error) {
	return resolveComponent("timestep_function", timestepFunctionBuilders, spec)
}

// ResolveExecutionStrategy builds an ExecutionStrategy from a data spec. An
// empty (omitted) spec resolves to nil, which selects the default
// spawn-per-step policy.
func ResolveExecutionStrategy(spec ComponentSpec) (ExecutionStrategy, error) {
	if spec.IsZero() {
		return nil, nil
	}
	return resolveComponent("execution_strategy", executionStrategyBuilders, spec)
}

func unknownType(family, kind string) error {
	return fmt.Errorf("%s: unknown data-spec type %q", family, kind)
}
