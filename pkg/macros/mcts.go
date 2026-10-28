package macros

import (
	"github.com/umbralcalc/stochadex/pkg/agents"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// MCTSSelfPlaySpec captures the inputs needed to wire a fully-decomposed
// MCTS self-play stack into a stochadex simulation. The result is three
// outer partitions plus an embedded sub-simulation hosting the
// (MCTSTreeIteration + MCTSRolloutIteration) pipeline:
//
//	outer:
//	  <Name>_apply   : ApplyIteration  — encoded game state; advances by
//	                                     one ply per outer step using the
//	                                     best-action signal from the
//	                                     embedded search.
//	  <Name>_search  : EmbeddedSimulationRunIteration — runs SimsPerPly
//	                                     inner steps per outer step, with
//	                                     the inner sim's tree root
//	                                     re-seeded each outer step from
//	                                     the apply partition's row.
//
//	inner sim (inside <Name>_search):
//	  <Name>_tree    : MCTSTreeIteration    — selection + backup; tree on
//	                                     struct; row exposes leaf state
//	                                     and root edge stats.
//	  <Name>_rollout : MCTSRolloutIteration — one rollout per inner step,
//	                                     consuming the leaf from
//	                                     <Name>_tree.
//
// The (selection + expansion + backup) bundle stays together because
// both selection and backup mutate the same shared graph state, but
// rollouts split out as a first-class partition — making per-rollout
// scores and selection paths into stochadex-native rows that other
// partitions can consume via params_from_upstream.
type MCTSSelfPlaySpec[S any, A any] struct {
	// Name prefix used to build per-partition names (apply, search,
	// tree, rollout). Each partition's actual name will be
	// "<Name>_apply" / "<Name>_search" / "<Name>_tree" / "<Name>_rollout".
	Name string
	// Env is the typed Environment[S, A] for both the search tree and
	// the rollout playouts.
	Env agents.Environment[S, A]
	// Cfg supplies UCT hyperparameters and the rollout function. The
	// rollout function is consumed by the rollout partition; tree
	// hyperparameters (Simulations / MaxTreeDepth / Exploration /
	// RolloutMaxSteps) are forwarded to the tree partition's selection
	// loop.
	Cfg agents.MCTSConfig[S, A]
	// InitState is the encoded-via-Encoder initial game state used as
	// both the apply partition's row and the inner tree partition's
	// initial root.
	InitState S
	// Decoder / Encoder define the codec for game state. Decoder must
	// round-trip Encoder.
	Decoder func([]float64) (S, error)
	Encoder func(S) []float64
	// SimsPerPly is the inner-sim termination step count — i.e. the
	// number of MCTS iterations to run between each outer ply. After a
	// 2-step pipeline fill in the inner sim, this is approximately the
	// number of distinct UCT iterations completed per ply.
	SimsPerPly int
	// MaxLegalActions is the K bound used for the tree partition's row
	// layout (per-action visit / win slots, padded with zeros for
	// inactive slots). Set to the maximum legal-action count any state
	// can produce. Tic-tac-toe = 9; card games typically 50+.
	MaxLegalActions int
	// StateWidth is the encoded-state vector length (= len(Encoder(s))
	// for any s).
	StateWidth int
	// Players is the player count (= per-player score vector length).
	Players int
	// Seed is the base RNG seed for both inner partitions.
	Seed uint64
	// ChanceNodes searches an agents.MCTSChanceTree, averaging each action's
	// value over sampled successors instead of committing to the first one drawn.
	// Requires Env to implement agents.StochasticEnvironment. See the field on
	// agents.MCTSTreeIteration for when it is worth the extra sampling.
	ChanceNodes bool
}

// MCTSSearchSettings is the env-independent half of MCTSSelfPlaySpec: the
// partition naming, seed, and UCT hyperparameters, all of which are plain
// scalars a config can state as data. The other half of the spec — the typed
// Environment, the codecs, the initial state, and the row-shape constants
// derived from them — has no data spelling and can only come from Go.
//
// That split is what the pkg/api environment registry is built on. A config
// states the search settings; a downstream-registered environment component
// supplies the environment half, and pkg/api calls ApplyMCTSSearchSettings to
// combine the two. The engine therefore never has to know what the environment
// *is*.
type MCTSSearchSettings struct {
	// Name prefix for the generated partitions (see MCTSSelfPlaySpec.Name).
	Name string
	// SimsPerPly is the inner-sim step count between outer plies.
	SimsPerPly int
	// Simulations, Exploration, MaxTreeDepth and RolloutMaxSteps are the UCT
	// hyperparameters. Zero means "leave whatever the builder set", which in
	// turn leaves the agents package defaults when the builder set nothing.
	Simulations     int
	Exploration     float64
	MaxTreeDepth    int
	RolloutMaxSteps int
	// Seed is the base RNG seed for the inner partitions.
	Seed uint64
}

// ApplyMCTSSearchSettings copies the config-stated settings onto spec, leaving
// the typed environment fields (Env, Cfg.Rollout, Cfg.Progress, InitState,
// Decoder, Encoder, MaxLegalActions, StateWidth, Players) untouched.
//
// Every numeric field is override-if-set: a zero in the settings leaves the
// builder's own value in place. This makes a registered environment free to
// carry sensible per-ruleset defaults that a config may selectively override,
// rather than having every config restate every knob. Name is the exception —
// it always wins when non-empty, because it determines the partition names the
// surrounding config reads output from.
func ApplyMCTSSearchSettings[S any, A any](
	spec *MCTSSelfPlaySpec[S, A],
	settings MCTSSearchSettings,
) {
	if settings.Name != "" {
		spec.Name = settings.Name
	}
	if settings.SimsPerPly > 0 {
		spec.SimsPerPly = settings.SimsPerPly
	}
	if settings.Seed != 0 {
		spec.Seed = settings.Seed
	}
	if settings.Simulations > 0 {
		spec.Cfg.Simulations = settings.Simulations
	}
	if settings.Exploration > 0 {
		spec.Cfg.Exploration = settings.Exploration
	}
	if settings.MaxTreeDepth > 0 {
		spec.Cfg.MaxTreeDepth = settings.MaxTreeDepth
	}
	if settings.RolloutMaxSteps > 0 {
		spec.Cfg.RolloutMaxSteps = settings.RolloutMaxSteps
	}
}

// NewMCTSSelfPlayPartitions returns the outer partitions for an MCTS
// self-play simulation built around the spec. The returned slice contains
// two entries: the apply partition (outer state, one ply per outer step)
// and the search partition (an embedded sub-simulation that runs the
// tree + rollout pipeline for SimsPerPly inner steps per outer step).
//
// Add both partitions to your ConfigGenerator. The tree + rollout inner
// partitions are encapsulated inside the search partition's
// EmbeddedSimulationRunIteration and do not appear in the outer config.
//
// Naming: the apply partition is exposed as "<Name>_apply" (handy for
// reading the encoded game state via params_from_upstream from any other
// outer partition you might add — telemetry, logging, custom analyses).
// The search partition is exposed as "<Name>_search".
func NewMCTSSelfPlayPartitions[S any, A any](spec MCTSSelfPlaySpec[S, A]) []*simulator.PartitionConfig {
	if spec.Name == "" {
		panic("macros.NewMCTSSelfPlayPartitions: Name required")
	}
	if spec.Encoder == nil || spec.Decoder == nil {
		panic("macros.NewMCTSSelfPlayPartitions: Encoder and Decoder required")
	}
	if spec.MaxLegalActions <= 0 {
		panic("macros.NewMCTSSelfPlayPartitions: MaxLegalActions must be > 0")
	}
	if spec.StateWidth <= 0 {
		panic("macros.NewMCTSSelfPlayPartitions: StateWidth must be > 0")
	}
	if spec.Players <= 0 {
		panic("macros.NewMCTSSelfPlayPartitions: Players must be > 0")
	}
	layout := NewMCTSSelfPlayLayout(spec)

	applyName := spec.Name + "_apply"
	searchName := spec.Name + "_search"
	treeName := spec.Name + "_tree"
	rolloutName := spec.Name + "_rollout"

	innerTree := &simulator.PartitionConfig{
		Name: treeName,
		Iteration: &agents.MCTSTreeIteration[S, A]{
			Env:             spec.Env,
			Cfg:             spec.Cfg,
			Decoder:         spec.Decoder,
			Encoder:         spec.Encoder,
			MaxLegalActions: spec.MaxLegalActions,
			StateWidth:      spec.StateWidth,
			Players:         spec.Players,
			ChanceNodes:     spec.ChanceNodes,
		},
		// MCTSTree reads rollout's previous-step scores via state-history
		// mode (params_as_partitions). This breaks the within-step
		// tree ↔ rollout cycle: rollout reads tree within-step
		// (params_from_upstream), tree reads rollout lag-1 (state
		// history). The 1-step lag aligns correctly — at step N+1 tree
		// backs up the path it selected at step N with scores from
		// rollout at step N (which were for that very leaf).
		ParamsAsPartitions: map[string][]string{
			agents.MCTSTreeParamRolloutScoresPartition: {rolloutName},
		},
		InitStateValues:   layout.TreeInit,
		StateHistoryDepth: 1,
		Seed:              layout.Seed,
	}

	innerRollout := &simulator.PartitionConfig{
		Name: rolloutName,
		Iteration: &agents.MCTSRolloutIteration[S, A]{
			Env:     spec.Env,
			Cfg:     spec.Cfg,
			Decoder: spec.Decoder,
			Players: spec.Players,
		},
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			agents.MCTSRolloutParamLeaf: {Upstream: treeName, Indices: layout.LeafIndices},
		},
		InitStateValues:   layout.RolloutInit,
		StateHistoryDepth: 1,
		Seed:              layout.RolloutSeed,
	}

	innerGen := simulator.NewConfigGenerator()
	innerGen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.NilOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: layout.SimsPerPly},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	innerGen.SetPartition(innerTree)
	innerGen.SetPartition(innerRollout)
	innerSettings, innerImpls := innerGen.GenerateConfigs()
	embedded := general.NewEmbeddedSimulationRunIteration(innerSettings, innerImpls)

	// Apply ↔ search dependency: search reads apply WITHIN-STEP via
	// params_from_upstream (so search's tree is always seeded with apply's
	// freshest game state); apply reads search LAG-1 via
	// params_as_partitions + state-history (so search does not have to
	// wait on apply for the previous step's best_idx). This breaks the
	// otherwise-circular dependency.
	searchParams := simulator.NewParams(map[string][]float64{
		// EmbeddedSimulationRunIteration.Configure unconditionally reads
		// burn_in_steps via Params.GetIndex (which panics if missing), so
		// we provide a default of 0.
		"burn_in_steps": {0},
	})

	search := &simulator.PartitionConfig{
		Name:      searchName,
		Iteration: embedded,
		Params:    searchParams,
		// Outer search reads the apply partition's encoded game state and
		// forwards it to the inner tree partition as its root_state param
		// (via the "<innerName>/<paramName>" forwarding the embedded run
		// recognises). Each outer step the tree sees this fresh value and
		// resets its tree if it differs from its cached root.
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			treeName + "/" + agents.MCTSTreeParamRootState: {Upstream: applyName, Indices: layout.RootStateIndices},
		},
		InitStateValues:   layout.SearchInit,
		StateHistoryDepth: 1,
		Seed:              layout.Seed,
	}

	// Apply uses state-history mode (BestIdxSlot + ParamsAsPartitions) to
	// read search's row from the previous step.
	apply := &simulator.PartitionConfig{
		Name: applyName,
		Iteration: &agents.ApplyIteration[S, A]{
			Env:         spec.Env,
			Decoder:     spec.Decoder,
			Encoder:     spec.Encoder,
			BestIdxSlot: layout.BestIdxSlot,
		},
		ParamsAsPartitions: map[string][]string{
			agents.ApplyParamBestIdxPartition: {searchName},
		},
		InitStateValues:   layout.ApplyInit,
		StateHistoryDepth: 1,
		Seed:              layout.Seed,
	}

	return []*simulator.PartitionConfig{apply, search}
}

// MCTSSelfPlayLayout is the data NewMCTSSelfPlayPartitions wires its
// partitions with: everything about them but their iterations, which hold the
// typed environment. A config stating the same search as data (mcts_apply,
// mcts_tree and mcts_rollout partitions) is wired with these values.
type MCTSSelfPlayLayout struct {
	// SimsPerPly is the search run's step count (50 when the spec sets none).
	SimsPerPly int
	// ApplyInit, TreeInit, RolloutInit and SearchInit are the apply, tree,
	// rollout and search partitions' initial states. The search's is the tree's
	// and rollout's concatenated, with best_root_idx at -1: nothing searched yet.
	ApplyInit, TreeInit, RolloutInit, SearchInit []float64
	// LeafIndices slice the tree row's leaf state and has-leaf flag out for the
	// rollout; RootStateIndices are the apply row's state, forwarded to the tree
	// as its root.
	LeafIndices, RootStateIndices []int
	// BestIdxSlot is where the apply partition finds best_root_idx in the
	// search's row.
	BestIdxSlot int
	// Seed seeds the apply, search and tree partitions; RolloutSeed the rollout.
	Seed, RolloutSeed uint64
}

// NewMCTSSelfPlayLayout is the layout NewMCTSSelfPlayPartitions wires spec's
// partitions with.
func NewMCTSSelfPlayLayout[S any, A any](spec MCTSSelfPlaySpec[S, A]) MCTSSelfPlayLayout {
	layout := MCTSSelfPlayLayout{SimsPerPly: spec.SimsPerPly, Seed: spec.Seed,
		RolloutSeed: spec.Seed ^ 0x1, BestIdxSlot: agents.MCTSTreeRowBestRootIdx}
	if layout.SimsPerPly <= 0 {
		layout.SimsPerPly = 50
	}
	treeRowWidth := agents.MCTSTreeRowWidth(spec.StateWidth, spec.MaxLegalActions)
	rolloutRowWidth := agents.MCTSRolloutRowWidth(spec.Players)
	layout.ApplyInit = spec.Encoder(spec.InitState)

	// The tree's best_root_idx slot starts at zero: no consumer reads it until
	// the tree has recorded a visit.
	layout.TreeInit = make([]float64, treeRowWidth)
	copy(layout.TreeInit[agents.MCTSTreeRowLeafStateOffset:], layout.ApplyInit)
	layout.RolloutInit = make([]float64, rolloutRowWidth)

	layout.LeafIndices = make([]int, 0, spec.StateWidth+1)
	for i := 0; i < spec.StateWidth; i++ {
		layout.LeafIndices = append(layout.LeafIndices, agents.MCTSTreeRowLeafStateOffset+i)
	}
	layout.LeafIndices = append(layout.LeafIndices, agents.MCTSTreeRowHasLeafOffset(spec.StateWidth))

	// The search row is the inner partitions' final states in order, tree
	// first, so best_root_idx sits at the tree row's own offset; -1 there
	// tells apply the search has computed nothing yet, and to skip the step.
	layout.SearchInit = make([]float64, treeRowWidth+rolloutRowWidth)
	copy(layout.SearchInit[:treeRowWidth], layout.TreeInit)
	copy(layout.SearchInit[treeRowWidth:], layout.RolloutInit)
	layout.SearchInit[layout.BestIdxSlot] = -1

	layout.RootStateIndices = make([]int, spec.StateWidth)
	for i := range layout.RootStateIndices {
		layout.RootStateIndices[i] = i
	}
	return layout
}
