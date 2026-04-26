package analysis

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
		panic("analysis.NewMCTSSelfPlayPartitions: Name required")
	}
	if spec.Encoder == nil || spec.Decoder == nil {
		panic("analysis.NewMCTSSelfPlayPartitions: Encoder and Decoder required")
	}
	if spec.MaxLegalActions <= 0 {
		panic("analysis.NewMCTSSelfPlayPartitions: MaxLegalActions must be > 0")
	}
	if spec.StateWidth <= 0 {
		panic("analysis.NewMCTSSelfPlayPartitions: StateWidth must be > 0")
	}
	if spec.Players <= 0 {
		panic("analysis.NewMCTSSelfPlayPartitions: Players must be > 0")
	}
	if spec.SimsPerPly <= 0 {
		spec.SimsPerPly = 50
	}

	applyName := spec.Name + "_apply"
	searchName := spec.Name + "_search"
	treeName := spec.Name + "_tree"
	rolloutName := spec.Name + "_rollout"

	// Inner partitions for the embedded search.
	treeRowWidth := agents.MCTSTreeRowWidth(spec.StateWidth, spec.MaxLegalActions)
	rolloutRowWidth := agents.MCTSRolloutRowWidth(spec.Players)
	encodedRoot := spec.Encoder(spec.InitState)

	treeInit := make([]float64, treeRowWidth)
	// Initial best_root_idx slot stays zero (will be overwritten on first
	// Iterate; the standalone MCTSTreeIteration convention is that 0 here is
	// safe because no consumer reads best_root_idx until the tree has
	// recorded at least one visit).
	copy(treeInit[agents.MCTSTreeRowLeafStateOffset:], encodedRoot)

	rolloutInit := make([]float64, rolloutRowWidth)

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
		InitStateValues:   treeInit,
		StateHistoryDepth: 1,
		Seed:              spec.Seed,
	}

	// Build the indices to slice tree's [leaf_state(W), has_leaf(1)]
	// section out of its full row — the rollout partition expects exactly
	// (W + 1) floats in that order.
	leafIndices := make([]int, 0, spec.StateWidth+1)
	for i := 0; i < spec.StateWidth; i++ {
		leafIndices = append(leafIndices, agents.MCTSTreeRowLeafStateOffset+i)
	}
	leafIndices = append(leafIndices, agents.MCTSTreeRowHasLeafOffset(spec.StateWidth))

	innerRollout := &simulator.PartitionConfig{
		Name: rolloutName,
		Iteration: &agents.MCTSRolloutIteration[S, A]{
			Env:     spec.Env,
			Cfg:     spec.Cfg,
			Decoder: spec.Decoder,
			Players: spec.Players,
		},
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			agents.MCTSRolloutParamLeaf: {Upstream: treeName, Indices: leafIndices},
		},
		InitStateValues:   rolloutInit,
		StateHistoryDepth: 1,
		Seed:              spec.Seed ^ 0x1,
	}

	innerGen := simulator.NewConfigGenerator()
	innerGen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.NilOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: spec.SimsPerPly},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	innerGen.SetPartition(innerTree)
	innerGen.SetPartition(innerRollout)
	innerSettings, innerImpls := innerGen.GenerateConfigs()
	embedded := general.NewEmbeddedSimulationRunIteration(innerSettings, innerImpls)

	// Compute the offset of MCTSTreeRowBestRootIdx within the search row
	// (which is the concatenation of inner partitions' final states in
	// order). tree is innerGen partition #0, so its row starts at offset
	// 0 of the concat — best_root_idx slot is at concat index 0.
	bestIdxOffset := agents.MCTSTreeRowBestRootIdx

	searchInit := make([]float64, treeRowWidth+rolloutRowWidth)
	copy(searchInit[:treeRowWidth], treeInit)
	copy(searchInit[treeRowWidth:], rolloutInit)
	// Initial best_root_idx slot = -1 (sentinel: search hasn't computed
	// anything yet → apply will skip this step).
	searchInit[agents.MCTSTreeRowBestRootIdx] = -1

	rootStateIndices := make([]int, spec.StateWidth)
	for i := 0; i < spec.StateWidth; i++ {
		rootStateIndices[i] = i
	}

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
			treeName + "/" + agents.MCTSTreeParamRootState: {Upstream: applyName, Indices: rootStateIndices},
		},
		InitStateValues:   searchInit,
		StateHistoryDepth: 1,
		Seed:              spec.Seed,
	}

	// Apply uses state-history mode (BestIdxSlot + ParamsAsPartitions) to
	// read search's row from the previous step.
	apply := &simulator.PartitionConfig{
		Name: applyName,
		Iteration: &agents.ApplyIteration[S, A]{
			Env:         spec.Env,
			Decoder:     spec.Decoder,
			Encoder:     spec.Encoder,
			BestIdxSlot: bestIdxOffset,
		},
		ParamsAsPartitions: map[string][]string{
			agents.ApplyParamBestIdxPartition: {searchName},
		},
		InitStateValues:   encodedRoot,
		StateHistoryDepth: 1,
		Seed:              spec.Seed,
	}

	return []*simulator.PartitionConfig{apply, search}
}
