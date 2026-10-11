package api

import (
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/agents"
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
// The environment is a component: {type: simulation, ...} is a decision
// problem over a forward model, with mcts_planning's fields (horizon, actions,
// action_partition, action_param, reward_partition, model, ...).

// typedMCTSEnvironment is an environment as the MCTS iterations take it, for
// its own state and action types: the environment, its codec, the shapes the
// iterations' rows are built from, and its rollout.
type typedMCTSEnvironment[S any, A any] struct {
	env       agents.Environment[S, A]
	rollout   agents.MCTSRolloutFn[S, A]
	progress  func(s S, player int) (float64, bool)
	initState S
	decoder   func([]float64) (S, error)
	encoder   func(S) []float64

	maxLegalActions, stateWidth, players int
	// searchDepth is the default tree depth and rollout length, or zero for
	// the agents defaults.
	searchDepth int
}

// mctsTreeSettings are the search settings the tree iteration reads.
type mctsTreeSettings struct {
	exploration  float64
	maxTreeDepth int
	pinnedNoise  bool
}

func (t *typedMCTSEnvironment[S, A]) tree(settings mctsTreeSettings) simulator.Iteration {
	config := agents.MCTSConfig[S, A]{Exploration: settings.exploration,
		MaxTreeDepth: settings.maxTreeDepth, Progress: t.progress}
	if config.MaxTreeDepth == 0 {
		config.MaxTreeDepth = t.searchDepth
	}
	return &agents.MCTSTreeIteration[S, A]{
		Env: t.env, Cfg: config, Decoder: t.decoder, Encoder: t.encoder,
		MaxLegalActions: t.maxLegalActions, StateWidth: t.stateWidth,
		Players: t.players, ChanceNodes: !settings.pinnedNoise,
	}
}

func (t *typedMCTSEnvironment[S, A]) rolloutIteration(maxSteps int) simulator.Iteration {
	if maxSteps == 0 {
		maxSteps = t.searchDepth
	}
	return &agents.MCTSRolloutIteration[S, A]{
		Env: t.env, Decoder: t.decoder, Players: t.players,
		Cfg: agents.MCTSConfig[S, A]{Rollout: t.rollout, Progress: t.progress,
			RolloutMaxSteps: maxSteps},
	}
}

func (t *typedMCTSEnvironment[S, A]) apply(bestIndexSlot int) simulator.Iteration {
	return &agents.ApplyIteration[S, A]{
		Env: t.env, Decoder: t.decoder, Encoder: t.encoder, BestIdxSlot: bestIndexSlot}
}

// mctsEnvironment builds the MCTS iterations over one environment, whatever
// its state and action types.
type mctsEnvironment interface {
	tree(mctsTreeSettings) simulator.Iteration
	rolloutIteration(maxSteps int) simulator.Iteration
	apply(bestIndexSlot int) simulator.Iteration
}

// resolveMCTSEnvironment builds the environment an mcts_* spec names.
func resolveMCTSEnvironment(spec simulator.ComponentSpec) (mctsEnvironment, error) {
	switch spec.Type {
	case "simulation":
		raw, _ := yaml.Marshal(spec.Fields)
		var problem simulationEnvironmentSpec
		if err := yaml.UnmarshalStrict(raw, &problem); err != nil {
			return nil, fmt.Errorf("environment simulation: %w", err)
		}
		if problem.Parameters != nil &&
			(problem.Parameters.SamplesFrom != nil || problem.Parameters.WeightsFrom != nil) {
			return nil, fmt.Errorf("environment simulation: parameters.samples_from and " +
				"weights_from read recorded rows, which only mcts_planning has; list the " +
				"samples and weights inline")
		}
		environment, err := problem.build(nil, "environment simulation")
		if err != nil {
			return nil, err
		}
		return environment.typed(), nil
	default:
		return nil, fmt.Errorf("unknown environment type %q (expected simulation)", spec.Type)
	}
}

// environment reads an mcts_* spec's environment field.
func (r *specReader) environment(key string) mctsEnvironment {
	spec, ok := r.nestedSpec(key, true)
	if !ok {
		return nil
	}
	environment, err := resolveMCTSEnvironment(spec)
	if err != nil {
		r.fail("field %q: %v", key, err)
	}
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
	iterationBuilders["mcts_tree"] = func(f map[string]interface{}) (simulator.Iteration, error) {
		r := newSpecReader("mcts_tree", f)
		environment := r.environment("environment")
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
	iterationBuilders["mcts_rollout"] = func(f map[string]interface{}) (simulator.Iteration, error) {
		r := newSpecReader("mcts_rollout", f)
		environment := r.environment("environment")
		maxSteps := optional[int](r, "rollout_max_steps", "an integer")
		if err := r.done(); err != nil {
			return nil, err
		}
		return environment.rolloutIteration(maxSteps), nil
	}
	iterationBuilders["mcts_apply"] = func(f map[string]interface{}) (simulator.Iteration, error) {
		r := newSpecReader("mcts_apply", f)
		environment := r.environment("environment")
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
