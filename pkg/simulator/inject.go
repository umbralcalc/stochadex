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
// The values are copied, so the caller may reuse its slice. The write lands in
// the params map the partition was built with, which is the Settings' own (as
// params_from_upstream's writes do), so a coordinator built afresh from the
// same Settings starts from the last injected values: to start from the
// configured ones, build from the config again.
func (c *PartitionCoordinator) InjectParams(partition, key string, values []float64) error {
	var iterator *StateIterator
	for _, candidate := range c.Iterators {
		if candidate.Partition.Name == partition {
			iterator = candidate
			break
		}
	}
	if iterator == nil {
		return fmt.Errorf("simulator: InjectParams: no partition named %q", partition)
	}
	if _, upstream := iterator.ValueChannels.Upstreams[key]; upstream {
		return fmt.Errorf("simulator: InjectParams: partition %q sets params key %q "+
			"from params_from_upstream each step, which would overwrite an injection",
			partition, key)
	}
	current, ok := iterator.Params.Map[key]
	if !ok {
		return fmt.Errorf("simulator: InjectParams: partition %q has no params key %q; "+
			"declare it in the partition's params with its initial value", partition, key)
	}
	if len(values) != len(current) {
		return fmt.Errorf("simulator: InjectParams: partition %q params key %q has "+
			"width %d, got %d values", partition, key, len(current), len(values))
	}
	iterator.Params.Set(key, append([]float64(nil), values...))
	return nil
}
