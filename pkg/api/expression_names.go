package api

import (
	"fmt"
	"sort"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// validateExpressionNames checks, at load, that no expression partition declares a
// name its expressions would resolve ambiguously (general.ExpressionIteration's
// CheckNames: a name spelled like a builtin, or declared twice). Configure refuses
// the same thing, but only once the run starts; here the whole config is in view,
// including the params a host forwards into its embedded runs, which the inner
// partition's own Configure never sees.
func validateExpressionNames(config *ApiRunConfig) error {
	return checkExpressionNames(config.Main, config.Embedded, nil, "main", config.scope)
}

// checkExpressionNames checks one run's expression partitions and those of the runs
// they host: the embedded runs, and each embedded_simulation_run's own run. extra
// maps a partition name to params keys it is given from outside its own config —
// forwarded by its host, or injected by a planner. where names the run in errors.
func checkExpressionNames(
	run RunConfig,
	embedded []EmbeddedRunConfig,
	extra map[string][]string,
	where string,
	scope *runScope,
) error {
	expressions := map[string]*general.ExpressionIteration{}
	for i := range run.Expressions {
		expressions[run.Expressions[i].Partition] = &run.Expressions[i].ExpressionIteration
	}
	hosts := map[string]simulator.PartitionConfig{}
	for _, partition := range run.Partitions {
		hosts[partition.Name] = partition
		iteration, ok := partition.Iteration.(*general.ExpressionIteration)
		if !ok {
			iteration, ok = expressions[partition.Name]
		}
		if ok {
			if err := iteration.CheckNames(
				paramsKeysOf(partition, extra[partition.Name])); err != nil {
				return fmt.Errorf("%s expression partition %q: %w", where, partition.Name, err)
			}
		}
		if nested, ok := nestedRunOf(partition, scope); ok {
			if err := checkExpressionNames(nested.Main, nested.Embedded,
				forwardedParams(partition),
				fmt.Sprintf("partition %q's nested run:", partition.Name), scope); err != nil {
				return err
			}
		}
	}
	for _, inner := range embedded {
		host, hosted := hosts[inner.Name]
		if !hosted {
			continue // an embedded run no partition hosts never runs
		}
		if err := checkExpressionNames(inner.Run, nil, forwardedParams(host),
			fmt.Sprintf("embedded run %q:", inner.Name), scope); err != nil {
			return err
		}
	}
	return nil
}

// paramsKeysOf is every params key a partition holds while it runs: its params,
// the keys params_as_partitions and params_from_upstream set, and any extra.
func paramsKeysOf(partition simulator.PartitionConfig, extra []string) []string {
	keys := make([]string, 0, len(partition.Params.Map)+
		len(partition.ParamsAsPartitions)+len(partition.ParamsFromUpstream)+len(extra))
	for key := range partition.Params.Map {
		keys = append(keys, key)
	}
	for key := range partition.ParamsAsPartitions {
		keys = append(keys, key)
	}
	for key := range partition.ParamsFromUpstream {
		keys = append(keys, key)
	}
	return append(keys, extra...)
}

// forwardedParams maps each partition of a host's embedded run to the params keys
// the host forwards into it ("<inner partition>/<param>").
func forwardedParams(host simulator.PartitionConfig) map[string][]string {
	forwarded := map[string][]string{}
	keys := paramsKeysOf(host, nil)
	sort.Strings(keys)
	for _, key := range keys {
		inner, param, ok := general.ForwardedParamTarget(key)
		if ok && param != "init_state_values" {
			forwarded[inner] = append(forwarded[inner], param)
		}
	}
	return forwarded
}

// refuseBooleanValues rejects a boolean anywhere in an expression spec decoded into
// interface{}, as a {type: ...} spec's fields are. Nothing in an expression spec is
// a boolean — names, aliases, partitions and expressions are all text — so one is
// always an unquoted y, n, yes, no, on or off that YAML 1.1 read as a boolean, and
// which would otherwise reach the expression as the name true or false. at is the
// path to value, for the message.
func refuseBooleanValues(value interface{}, at string) error {
	switch typed := value.(type) {
	case bool:
		return fmt.Errorf("%s is the boolean %v, but every value in an expression spec "+
			"is text: YAML reads an unquoted y, n, yes, no, on, off, true or false as a "+
			"boolean, so quote it (e.g. name: \"n\")", at, typed)
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := refuseBooleanValues(typed[key], at+"."+key); err != nil {
				return err
			}
		}
	case map[interface{}]interface{}:
		for key, inner := range typed {
			if err := refuseBooleanValues(inner, fmt.Sprintf("%s.%v", at, key)); err != nil {
				return err
			}
		}
	case []interface{}:
		for i, inner := range typed {
			if err := refuseBooleanValues(inner, fmt.Sprintf("%s[%d]", at, i)); err != nil {
				return err
			}
		}
	}
	return nil
}
