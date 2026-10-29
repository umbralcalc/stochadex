package api

import (
	"fmt"
	"strings"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gopkg.in/yaml.v2"
)

// The three MCTS iterations of pkg/agents as data, the same partitions
// mcts_planning wires (see macros.NewMCTSSelfPlayPartitions):
//
//	{type: mcts_tree, environment: {...}, exploration, max_tree_depth, pinned_noise}
//	{type: mcts_rollout, environment: {...}, rollout_max_steps}
//	{type: mcts_apply, environment: {...}, best_index_slot}
//
// Each spec carries the environment and only the settings its iteration reads.
// Everything else that shapes a search is wiring, stated where wiring is: the
// number of simulations per decision is the search run's step count, seeds are
// the partitions' seeds, and the root is the tree's init_state_values. Each
// partition builds its own environment from the same spec; a YAML anchor
// states it once.
//
// The environment is a component (an MCTSEnvironment): {type: simulation, ...}
// is a decision problem over a forward model, with mcts_planning's fields
// (horizon, actions, action_partition, action_param, reward_partition, model,
// ...), and any other type is an environment registered with
// RegisterEnvironment, such as the tictactoe fixture.

// resolveMCTSEnvironment builds the environment an mcts_* spec names: the
// engine's own {type: simulation}, or a registered one.
// An environment whose parameters are read from inputs is recorded in scope,
// to be bound when the run starts.
func resolveMCTSEnvironment(spec simulator.ComponentSpec, scope *runScope) (MCTSEnvironment, error) {
	switch spec.Type {
	case simulationEnvironmentType:
		raw, _ := yaml.Marshal(spec.Fields)
		var problem simulationEnvironmentSpec
		if err := yaml.UnmarshalStrict(raw, &problem); err != nil {
			return nil, fmt.Errorf("environment simulation: %w", err)
		}
		if parameters := problem.Parameters; parameters != nil &&
			((parameters.SamplesFrom != nil && parameters.SamplesFrom.Input == "") ||
				(parameters.WeightsFrom != nil && parameters.WeightsFrom.Input == "")) {
			return nil, fmt.Errorf("environment simulation: parameters.samples_from and " +
				"weights_from read an input's recorded rows when the run starts, so name it: " +
				"samples_from: {input: <name>, partition_name: ...}; or list the samples and " +
				"weights inline")
		}
		environment, err := problem.build(nil, "environment simulation")
		if err != nil {
			return nil, err
		}
		if environment.deferred != nil {
			scope.readers.environments = append(scope.readers.environments, environment)
		}
		return environment.typed(), nil
	default:
		if _, registered := environmentBuilders[spec.Type]; !registered {
			return nil, fmt.Errorf("unknown environment type %q (expected simulation, or a "+
				"registered environment: %s)", spec.Type, strings.Join(RegisteredEnvironments(), ", "))
		}
		return ResolveEnvironment(spec)
	}
}

// environment reads an mcts_* spec's environment field. Within a run, every
// partition naming the same environment spec searches one environment, built
// once: an environment may keep state between calls (a codec that learns
// encodings as it meets them), and the partitions of one search must agree on
// it, as mcts_self_play's partitions, sharing one, always have.
func (r *specReader) environment(key string, scope *runScope) MCTSEnvironment {
	spec, ok := r.nestedSpec(key, true)
	if !ok {
		return nil
	}
	// Marshalling sorts mapping keys, so equal specs give equal text.
	text, _ := yaml.Marshal(spec)
	if environment, shared := scope.environments[string(text)]; shared {
		return environment
	}
	environment, err := resolveMCTSEnvironment(spec, scope)
	if err != nil {
		r.fail("field %q: %v", key, err)
		return nil
	}
	scope.environments[string(text)] = environment
	return environment
}

// optional reads an optional field of type T, leaving zero when it is unset.
func optional[T any](r *specReader, key, kind string) T {
	var typed T
	value, ok := r.value(key, false)
	if !ok {
		return typed
	}
	if converted, ok := value.(T); ok {
		return converted
	}
	r.fail("field %q must be %s, got %T", key, kind, value)
	return typed
}

func init() {
	scopedIterationBuilders["mcts_tree"] = func(f map[string]interface{}, scope *runScope) (simulator.Iteration, error) {
		r := newSpecReader("mcts_tree", f)
		environment := r.environment("environment", scope)
		settings := mctsTreeSettings{
			exploration:  optionalFloat(r, "exploration"),
			maxTreeDepth: optional[int](r, "max_tree_depth", "an integer"),
			pinnedNoise:  optional[bool](r, "pinned_noise", "a bool"),
		}
		if err := r.done(); err != nil {
			return nil, err
		}
		return environment.tree(settings), nil
	}
	scopedIterationBuilders["mcts_rollout"] = func(f map[string]interface{}, scope *runScope) (simulator.Iteration, error) {
		r := newSpecReader("mcts_rollout", f)
		environment := r.environment("environment", scope)
		maxSteps := optional[int](r, "rollout_max_steps", "an integer")
		if err := r.done(); err != nil {
			return nil, err
		}
		return environment.rolloutIteration(maxSteps), nil
	}
	scopedIterationBuilders["mcts_apply"] = func(f map[string]interface{}, scope *runScope) (simulator.Iteration, error) {
		r := newSpecReader("mcts_apply", f)
		environment := r.environment("environment", scope)
		slot := optional[int](r, "best_index_slot", "an integer")
		if err := r.done(); err != nil {
			return nil, err
		}
		return environment.apply(slot), nil
	}
}

// optionalFloat reads an optional number, which YAML may give as an integer.
func optionalFloat(r *specReader, key string) float64 {
	value, ok := r.value(key, false)
	if !ok {
		return 0
	}
	switch number := value.(type) {
	case float64:
		return number
	case int:
		return float64(number)
	}
	r.fail("field %q must be a number, got %T", key, value)
	return 0
}
