package simulator

import "fmt"

// InjectParams sets the params key of the named partition to values, from the
// next step on: a step that starts after InjectParams returns sees the new
// values, and they stay until injected again. This is the engine's port for
// input from outside a run — a user's action, a live feed — and it is the only
// way such input reaches a partition: through its params, at a step boundary,
// never inside Iterate (PLAN.md rule 6).
//
// Call it only between steps: after Run, or a Stepper's Step, has returned and
// before the next Step begins — never concurrently with one. Step's own
// synchronisation then orders the write before every partition's next read,
// under every execution strategy.
//
// The key must already be in the partition's params, declared in its config
// with its initial value, and values must have the same width: a misspelled key
// or a wrong width is an error rather than a param no iteration reads. A key
// set from params_from_upstream is rejected too, since the upstream would
// overwrite it at the start of the next step. An iteration that reads the key
// only in Configure does not see injected values.
//
// The values are copied into the key's existing slice, so the caller may reuse
// its own, and nothing is allocated. That slice is the one the partition was
// built with, which is the Settings' own (params_from_upstream writes there too),
// so a coordinator built afresh from the same Settings starts from the last
// injected values: to start from the configured ones, build from the config
// again.
//
// InjectParams finds the partition and checks the key on every call. To set the
// same key every step, resolve it once with NewParamsInjector.
func (c *PartitionCoordinator) InjectParams(partition, key string, values []float64) error {
	injector, err := c.NewParamsInjector(partition, key)
	if err != nil {
		return err
	}
	return injector.Inject(values)
}

// ParamsInjector sets one partition's params key between steps, as
// InjectParams does, with the partition found and the key checked once, when
// it is made: each Inject is a width check and a copy.
type ParamsInjector struct {
	partition, key string
	values         []float64 // the key's slice in the partition's params
}

// NewParamsInjector resolves partition's params key for injection, with the
// checks InjectParams makes.
func (c *PartitionCoordinator) NewParamsInjector(partition, key string) (*ParamsInjector, error) {
	var iterator *StateIterator
	for _, candidate := range c.Iterators {
		if candidate.Partition.Name == partition {
			iterator = candidate
			break
		}
	}
	if iterator == nil {
		return nil, fmt.Errorf("simulator: InjectParams: no partition named %q", partition)
	}
	if _, upstream := iterator.ValueChannels.Upstreams[key]; upstream {
		return nil, fmt.Errorf("simulator: InjectParams: partition %q sets params key %q "+
			"from params_from_upstream each step, which would overwrite an injection",
			partition, key)
	}
	current, ok := iterator.Params.Map[key]
	if !ok {
		return nil, fmt.Errorf("simulator: InjectParams: partition %q has no params key %q; "+
			"declare it in the partition's params with its initial value", partition, key)
	}
	return &ParamsInjector{partition: partition, key: key, values: current}, nil
}

// Inject sets the key to values from the next step on. Call it only between
// steps, as InjectParams.
func (p *ParamsInjector) Inject(values []float64) error {
	if len(values) != len(p.values) {
		return fmt.Errorf("simulator: InjectParams: partition %q params key %q has "+
			"width %d, got %d values", p.partition, p.key, len(p.values), len(values))
	}
	copy(p.values, values)
	return nil
}
