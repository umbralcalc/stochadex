package api

import (
	"fmt"
	"sort"
	"strings"

	"github.com/umbralcalc/stochadex/pkg/agents"
	"github.com/umbralcalc/stochadex/pkg/macros"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// The environment registry lets a config name an agents.Environment: as the
// environment of the mcts_tree / mcts_rollout / mcts_apply iterations
// (environment: {type: ...}), or as the mcts_self_play macro's env:.
//
// It is a hook rather than a {type: ...} data spec because an environment is
// arbitrary decision rules (Legal / Apply / Terminal), which the repo boundary
// places downstream. Spelling those as data would mean a rules language inside
// the config. The registered builder receives the whole ComponentSpec verbatim
// and returns a component, so the fields of an environment spec are parsed by
// the downstream and never by this package.
//
// The component is an MCTSEnvironment, built by NewMCTSEnvironment from a
// typed MCTSEnvironmentSpec[S, A]. It keeps the state and action types on the
// Go side: it builds the MCTS iterations for its own S and A, so no generic
// type is spelled in YAML and nothing is encoded and decoded through every
// Apply in the search's hot loop.

// MCTSEnvironment is an environment the MCTS iterations, and mcts_self_play,
// search. Build one with NewMCTSEnvironment.
type MCTSEnvironment interface {
	tree(mctsTreeSettings) simulator.Iteration
	rolloutIteration(maxSteps int) simulator.Iteration
	apply(bestIndexSlot int) simulator.Iteration
	selfPlay(settings macros.MCTSSearchSettings) []*simulator.PartitionConfig
	// selfPlayLayout is the layout selfPlay wires its partitions with.
	selfPlayLayout(settings macros.MCTSSearchSettings) macros.MCTSSelfPlayLayout
	// samplesSuccessors reports that the environment can sample successors,
	// so an mcts_tree over it searches chance nodes unless pinned_noise is set.
	samplesSuccessors() bool
}

// MCTSEnvironmentSpec is an environment for its own state and action types,
// with what the MCTS iterations need to know about it.
type MCTSEnvironmentSpec[S any, A any] struct {
	// Env is the decision rules.
	Env agents.Environment[S, A]
	// Rollout plays out a leaf; nil plays uniformly at random. Progress
	// optionally scores a position for a rollout cut short.
	Rollout  agents.MCTSRolloutFn[S, A]
	Progress func(s S, player int) (float64, bool)
	// InitState is where mcts_self_play starts. Encoder and Decoder are its
	// codec into a partition's row, Decoder round-tripping Encoder.
	InitState S
	Encoder   func(S) []float64
	Decoder   func([]float64) (S, error)
	// MaxLegalActions bounds the legal actions any state has, StateWidth is
	// the encoded state's length, and Players the number of players. They
	// are the environment's own shape, so never config's to state: a
	// mismatch is a silently misshapen row rather than a loud error.
	MaxLegalActions, StateWidth, Players int
	// SearchDepth bounds the tree and the rollouts when the config sets no
	// limit; zero leaves the agents defaults.
	SearchDepth int
	// SimsPerPly is mcts_self_play's sims_per_ply when the config sets none.
	SimsPerPly int
}

// NewMCTSEnvironment makes an environment a component the MCTS iterations
// and mcts_self_play can search. Chance nodes, which average each action over
// sampled successors, are available only when Env is an
// agents.StochasticEnvironment; for deterministic rules the search commits to
// the one successor, which is exactly right.
func NewMCTSEnvironment[S any, A any](spec MCTSEnvironmentSpec[S, A]) (MCTSEnvironment, error) {
	switch {
	case spec.Env == nil:
		return nil, fmt.Errorf("an MCTS environment needs Env")
	case spec.Encoder == nil || spec.Decoder == nil:
		return nil, fmt.Errorf("an MCTS environment needs Encoder and Decoder")
	case spec.MaxLegalActions <= 0 || spec.StateWidth <= 0 || spec.Players <= 0:
		return nil, fmt.Errorf("an MCTS environment needs MaxLegalActions, StateWidth and "+
			"Players > 0, got %d, %d and %d", spec.MaxLegalActions, spec.StateWidth, spec.Players)
	}
	if spec.Rollout == nil {
		spec.Rollout = agents.UniformRandomRollout[S, A]()
	}
	_, stochastic := spec.Env.(agents.StochasticEnvironment[S, A])
	return &typedMCTSEnvironment[S, A]{
		env: spec.Env, rollout: spec.Rollout, progress: spec.Progress,
		initState: spec.InitState, encoder: spec.Encoder, decoder: spec.Decoder,
		maxLegalActions: spec.MaxLegalActions, stateWidth: spec.StateWidth,
		players: spec.Players, searchDepth: spec.SearchDepth,
		simsPerPly: spec.SimsPerPly, stochastic: stochastic,
	}, nil
}

// EnvironmentBuilder constructs an environment from its data spec, whose
// fields only the builder parses.
type EnvironmentBuilder func(spec simulator.ComponentSpec) (MCTSEnvironment, error)

// environmentBuilders holds every registered environment spelling. The engine
// claims only the "tictactoe" fixture name, registered through the same entry
// point, and "simulation", the data-defined environment it builds itself.
var environmentBuilders = map[string]EnvironmentBuilder{}

// simulationEnvironmentType is the engine's own environment spelling: a decision
// problem over a forward model, built by resolveMCTSEnvironment.
const simulationEnvironmentType = "simulation"

// RegisterEnvironment adds an environment spelling, usable as the mcts_*
// iterations' environment: type and as the mcts_self_play macro's env: type.
// Call it from an init function in the module that owns the environment; the
// engine only sees it if the binary links that module, so a downstream ships
// its own CLI (as cmd/stochadex does for the ONNX partition).
//
// Panics on a duplicate name, so two modules cannot silently claim one
// spelling, and on "simulation", which is the engine's own.
func RegisterEnvironment(typeName string, build EnvironmentBuilder) {
	if _, exists := environmentBuilders[typeName]; exists || typeName == simulationEnvironmentType {
		panic("api: duplicate environment registration " + typeName)
	}
	environmentBuilders[typeName] = build
}

// RegisteredEnvironments returns the registered spellings in sorted order, for
// error messages and for a downstream to check what its binary has linked.
func RegisteredEnvironments() []string {
	names := make([]string, 0, len(environmentBuilders))
	for name := range environmentBuilders {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ResolveEnvironment builds the registered environment a spec names.
func ResolveEnvironment(spec simulator.ComponentSpec) (MCTSEnvironment, error) {
	build, ok := environmentBuilders[spec.Type]
	if !ok {
		return nil, fmt.Errorf(
			"environment: unknown type %q (registered: %s). An environment is "+
				"supplied by the downstream module that owns the decision rules and "+
				"registered with api.RegisterEnvironment — the engine ships only the "+
				"tictactoe fixture. If you registered one, check the binary links "+
				"that module",
			spec.Type,
			strings.Join(RegisteredEnvironments(), ", "),
		)
	}
	environment, err := build(spec)
	if err != nil {
		return nil, fmt.Errorf("environment %q: %w", spec.Type, err)
	}
	if environment == nil {
		return nil, fmt.Errorf("environment %q: builder returned no environment", spec.Type)
	}
	return environment, nil
}

// typedMCTSEnvironment is an MCTSEnvironment for its own state and action
// types.
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
	// simsPerPly is mcts_self_play's default sims_per_ply.
	simsPerPly int
	// stochastic says env can sample successors, which chance nodes need.
	stochastic bool
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
		Players: t.players, ChanceNodes: t.stochastic && !settings.pinnedNoise,
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

// selfPlay builds mcts_self_play's partitions.
func (t *typedMCTSEnvironment[S, A]) selfPlay(
	settings macros.MCTSSearchSettings,
) []*simulator.PartitionConfig {
	return macros.NewMCTSSelfPlayPartitions(t.selfPlaySpec(settings))
}

func (t *typedMCTSEnvironment[S, A]) selfPlayLayout(
	settings macros.MCTSSearchSettings,
) macros.MCTSSelfPlayLayout {
	return macros.NewMCTSSelfPlayLayout(t.selfPlaySpec(settings))
}

func (t *typedMCTSEnvironment[S, A]) samplesSuccessors() bool {
	return t.stochastic
}

// selfPlaySpec is mcts_self_play's search, the config's settings applied over
// the environment's defaults.
func (t *typedMCTSEnvironment[S, A]) selfPlaySpec(
	settings macros.MCTSSearchSettings,
) macros.MCTSSelfPlaySpec[S, A] {
	spec := macros.MCTSSelfPlaySpec[S, A]{
		Env: t.env,
		Cfg: agents.MCTSConfig[S, A]{Rollout: t.rollout, Progress: t.progress,
			MaxTreeDepth: t.searchDepth, RolloutMaxSteps: t.searchDepth},
		InitState: t.initState, Decoder: t.decoder, Encoder: t.encoder,
		MaxLegalActions: t.maxLegalActions, StateWidth: t.stateWidth,
		Players: t.players, SimsPerPly: t.simsPerPly,
	}
	macros.ApplyMCTSSearchSettings(&spec, settings)
	return spec
}
