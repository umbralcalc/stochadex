package simulator

import "fmt"

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
	c.Type = kind
	c.Fields = asMap
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

// ResolveOutputCondition builds an OutputCondition from a data spec.
func ResolveOutputCondition(spec ComponentSpec) (OutputCondition, error) {
	reader := newFieldReader(spec.Type, spec.Fields)
	var result OutputCondition
	switch spec.Type {
	case "nil":
		result = &NilOutputCondition{}
	case "every_step":
		result = &EveryStepOutputCondition{}
	case "every_n_steps":
		result = &EveryNStepsOutputCondition{N: reader.int("n")}
	case "only_given_partitions":
		names := reader.stringSlice("partitions")
		partitions := make(map[string]bool, len(names))
		for _, name := range names {
			partitions[name] = true
		}
		result = &OnlyGivenPartitionsOutputCondition{Partitions: partitions}
	default:
		if value, ok, err := resolveExtra("output_condition", spec); ok {
			if err != nil {
				return nil, err
			}
			return value.(OutputCondition), nil
		}
		return nil, unknownType("output_condition", spec.Type)
	}
	return result, reader.done()
}

// ResolveOutputFunction builds an OutputFunction from a data spec. Live-object
// sinks (state storage, channel, websocket) have no data form and are absent.
func ResolveOutputFunction(spec ComponentSpec) (OutputFunction, error) {
	reader := newFieldReader(spec.Type, spec.Fields)
	var result OutputFunction
	switch spec.Type {
	case "nil":
		result = &NilOutputFunction{}
	case "stdout":
		result = &StdoutOutputFunction{}
	case "json_log":
		result = NewJsonLogOutputFunction(reader.str("path"))
	default:
		if value, ok, err := resolveExtra("output_function", spec); ok {
			if err != nil {
				return nil, err
			}
			return value.(OutputFunction), nil
		}
		return nil, unknownType("output_function", spec.Type)
	}
	return result, reader.done()
}

// ResolveTerminationCondition builds a TerminationCondition from a data spec.
func ResolveTerminationCondition(spec ComponentSpec) (TerminationCondition, error) {
	reader := newFieldReader(spec.Type, spec.Fields)
	var result TerminationCondition
	switch spec.Type {
	case "number_of_steps":
		result = &NumberOfStepsTerminationCondition{MaxNumberOfSteps: reader.int("max_steps")}
	case "time_elapsed":
		result = &TimeElapsedTerminationCondition{MaxTimeElapsed: reader.float("max_time_elapsed")}
	default:
		if value, ok, err := resolveExtra("termination_condition", spec); ok {
			if err != nil {
				return nil, err
			}
			return value.(TerminationCondition), nil
		}
		return nil, unknownType("termination_condition", spec.Type)
	}
	return result, reader.done()
}

// extraComponentBuilders holds data-spec builders for framework components that
// live in packages downstream of simulator (e.g. a general.FromHistoryTimestepFunction).
// A downstream package registers them from an init() so the four Resolve
// functions below can reach them without simulator importing that package. Keyed
// by family then type name.
var extraComponentBuilders = map[string]map[string]func(ComponentSpec) (interface{}, error){}

// RegisterComponent registers a data-spec builder for a component that lives
// downstream of simulator. family is one of "output_condition", "output_function",
// "termination_condition", "timestep_function". Call it from an init(); it panics
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
	reader := newFieldReader(spec.Type, spec.Fields)
	var result TimestepFunction
	switch spec.Type {
	case "constant":
		result = &ConstantTimestepFunction{Stepsize: reader.float("stepsize")}
	case "exponential_distribution":
		result = &ExponentialDistributionTimestepFunction{
			Mean: reader.float("mean"),
			Seed: reader.uint64("seed"),
		}
	default:
		if value, ok, err := resolveExtra("timestep_function", spec); ok {
			if err != nil {
				return nil, err
			}
			return value.(TimestepFunction), nil
		}
		return nil, unknownType("timestep_function", spec.Type)
	}
	return result, reader.done()
}

// ResolveExecutionStrategy builds an ExecutionStrategy from a data spec. The three
// strategies are zero-field structs, so each is a nullary construction. An empty
// (omitted) spec resolves to nil, which selects the default spawn-per-step policy.
func ResolveExecutionStrategy(spec ComponentSpec) (ExecutionStrategy, error) {
	if spec.IsZero() {
		return nil, nil
	}
	reader := newFieldReader(spec.Type, spec.Fields)
	var result ExecutionStrategy
	switch spec.Type {
	case "spawn_per_step":
		result = &SpawnPerStepExecution{}
	case "persistent_worker":
		result = &PersistentWorkerExecution{}
	case "inline":
		result = &InlineExecution{}
	default:
		if value, ok, err := resolveExtra("execution_strategy", spec); ok {
			if err != nil {
				return nil, err
			}
			return value.(ExecutionStrategy), nil
		}
		return nil, unknownType("execution_strategy", spec.Type)
	}
	return result, reader.done()
}

func unknownType(family, kind string) error {
	return fmt.Errorf("%s: unknown data-spec type %q", family, kind)
}
