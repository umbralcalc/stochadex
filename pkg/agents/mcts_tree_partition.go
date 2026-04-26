package agents

import (
	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// MCTSTreePartition runs the (selection + expansion + backup) phase of a UCT
// MCTS search as a stochadex iteration. The tree itself lives on the
// struct (graph state, fundamentally not []float64-shaped); the partition
// row exposes a fixed-width summary that downstream partitions can consume
// via params_from_upstream:
//
//   row[MCTSTreeRowBestRootIdx]                        — most-visited root
//                                                    legal-action index
//                                                    after the most recent
//                                                    update (-1 if not
//                                                    yet decided)
//   row[MCTSTreeRowLeafStateOffset .. +StateWidth-1]   — encoded state of the
//                                                    leaf the search just
//                                                    selected (input to a
//                                                    rollout partition)
//   row[MCTSTreeRowHasLeafOffset(W)]                   — 1 if the leaf_state
//                                                    slot is real, 0
//                                                    otherwise (used by
//                                                    rollout partitions to
//                                                    short-circuit when
//                                                    nothing has been
//                                                    selected yet)
//   row[MCTSTreeRowVisitsOffset(W) .. +MaxLegalActions-1] — per-legal-action
//                                                       root visit counts,
//                                                       padded with zeros
//   row[MCTSTreeRowWinsOffset(W,K) .. +MaxLegalActions-1] — per-legal-action
//                                                       root win sums,
//                                                       padded with zeros
//
// Use MCTSTreeRowWidth(W, K) to compute the required state_width / init slice
// length.
//
// # Pipeline lag
//
// MCTSTreePartition is one half of a 2-step pipeline with a downstream rollout
// partition. The rollout partition reads (leaf_state, has_leaf) and
// outputs scores; MCTSTreePartition then reads those scores via
// params_from_upstream (key MCTSTreeParamRolloutScores) and applies a backup
// to the path it selected two steps earlier. The 2-step lag is
// fundamental to expressing selection-then-backup as stochadex's
// single-row dataflow.
//
// In steady state each outer step does one selection + one backup, so the
// throughput is one MCTS iteration per stochadex step (after a 2-step
// fill).
//
// Warm fields (must be set before Configure):
//   - Env: the typed Environment[S, A] to search.
//   - Cfg: UCT hyperparameters and the rollout function (the rollout fn
//     here is only used by MCTSTree.RunOne when no upstream rollout is wired —
//     left nil if a separate MCTSRolloutPartition supplies the scores).
//   - Decoder, Encoder: codec for the leaf state slots.
//   - MaxLegalActions: K, the maximum legal-action count at any node.
//     Stats slots beyond the actual legal count are zero-padded.
//   - StateWidth: W, the width of one encoded state.
//   - Players: P, the per-player score vector length.
type MCTSTreePartition[S any, A any] struct {
	Env             Environment[S, A]
	Cfg             MCTSConfig[S, A]
	Decoder         func([]float64) (S, error)
	Encoder         func(S) []float64
	MaxLegalActions int
	StateWidth      int
	Players         int

	tree        *MCTSTree[S, A]
	pendingPath []int // path selected last step, awaiting scores this step
	seed        uint64
	root        S
	rootEncoded []float64
}

// Param key used by MCTSTreePartition.Iterate to read rollout scores from an
// upstream MCTSRolloutPartition via params_from_upstream (within-step). The
// slice should be of length Players + 1 (P scores + ok flag) — the layout
// produced by MCTSRolloutPartition.
//
// This mode creates a within-step dependency that breaks if the rollout
// partition also depends on the tree (the standard MCTS pipeline does).
// Use MCTSTreeParamRolloutScoresPartition for the lag-1 state-history mode
// instead when wiring the tree + rollout pipeline.
const MCTSTreeParamRolloutScores = "rollout_scores"

// Param key used by MCTSTreePartition.Iterate to read rollout scores from an
// upstream MCTSRolloutPartition via params_as_partitions (state-history,
// lag-1). The value is a 1-element slice containing the rollout
// partition's index; the tree reads stateHistories[idx].Values row 0
// (= the previous step's rollout output) at runtime.
//
// This is the standard wiring used by NewMCTSSelfPlayPartitions: rollout
// reads tree's leaf within-step (so the rollout sees the freshest leaf),
// and tree reads rollout's scores lag-1 (so rollout doesn't have to wait
// on tree for last step's scores). The 1-step lag aligns correctly: at
// step N+1 tree backs up the path it selected at step N with scores from
// rollout at step N (which were for that very leaf).
//
// State-history mode takes priority over within-step mode if both keys
// are present.
const MCTSTreeParamRolloutScoresPartition = "rollout_scores_partition"

// Param key used by MCTSTreePartition.Iterate to read the current search
// root state. Whenever this param's value differs from the cached root
// encoding, the tree is reset to the decoded new root. The slice should
// be of length StateWidth.
//
// Set this via the embedded simulation run's outer params_from_upstream
// (e.g. an outer apply partition piping its current game state into the
// inner sim via the "<innerName>/root_state" forwarding mechanism). When
// MCTSTreePartition is used standalone with no outer pipeline, the param is
// absent and the tree retains the root set at Configure time.
const MCTSTreeParamRootState = "root_state"

// Row layout slot accessors. Use these to compute params_from_upstream
// indices when wiring downstream partitions to MCTSTreePartition's row.
const MCTSTreeRowBestRootIdx = 0
const MCTSTreeRowLeafStateOffset = 1

func MCTSTreeRowHasLeafOffset(stateWidth int) int { return 1 + stateWidth }
func MCTSTreeRowVisitsOffset(stateWidth int) int  { return 2 + stateWidth }
func MCTSTreeRowWinsOffset(stateWidth, maxLegalActions int) int {
	return 2 + stateWidth + maxLegalActions
}

// MCTSTreeRowWidth returns the required InitStateValues / StateWidth for a
// MCTSTreePartition with the given encoded-state width and max legal-action
// count.
func MCTSTreeRowWidth(stateWidth, maxLegalActions int) int {
	return 2 + stateWidth + 2*maxLegalActions
}

// Configure implements simulator.Iteration. Decodes the encoded root from
// is.InitStateValues[1 .. 1+StateWidth] and resets the tree. The first
// slot (MCTSTreeRowBestRootIdx) and the stats slots can be left zero in the
// init: best_root_idx is initialised to -1 to signal "not yet decided".
func (m *MCTSTreePartition[S, A]) Configure(partitionIndex int, settings *simulator.Settings) {
	if m.Env == nil {
		panic("agents.MCTSTreePartition: Env required")
	}
	if m.Decoder == nil {
		panic("agents.MCTSTreePartition: Decoder required")
	}
	if m.Encoder == nil {
		panic("agents.MCTSTreePartition: Encoder required")
	}
	if m.MaxLegalActions <= 0 {
		panic("agents.MCTSTreePartition: MaxLegalActions must be > 0")
	}
	if m.StateWidth <= 0 {
		panic("agents.MCTSTreePartition: StateWidth must be > 0")
	}
	if m.Players <= 0 {
		panic("agents.MCTSTreePartition: Players must be > 0")
	}
	is := settings.Iterations[partitionIndex]
	expected := MCTSTreeRowWidth(m.StateWidth, m.MaxLegalActions)
	if is.StateWidth != expected {
		panic("agents.MCTSTreePartition: StateWidth must equal MCTSTreeRowWidth(W, K)")
	}
	encoded := make([]float64, m.StateWidth)
	copy(encoded, is.InitStateValues[MCTSTreeRowLeafStateOffset:MCTSTreeRowLeafStateOffset+m.StateWidth])
	root, err := m.Decoder(encoded)
	if err != nil {
		panic("agents.MCTSTreePartition: decoder failed: " + err.Error())
	}
	m.root = root
	m.rootEncoded = encoded
	m.tree = NewMCTSTree[S, A](root)
	m.pendingPath = nil
	m.seed = is.Seed
}

// Iterate implements simulator.Iteration.
func (m *MCTSTreePartition[S, A]) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	cfg := m.Cfg
	cfg.applyDefaults()

	// Phase 0: if upstream has supplied a root_state and it differs from
	// our cached root, reset the tree. This is how the outer self-play
	// pipeline signals "the game advanced; start a fresh search from the
	// new root."
	if rootSlice, ok := params.GetOk(MCTSTreeParamRootState); ok && len(rootSlice) == m.StateWidth {
		if !floatsEqual(rootSlice, m.rootEncoded) {
			rootCopy := make([]float64, m.StateWidth)
			copy(rootCopy, rootSlice)
			newRoot, err := m.Decoder(rootCopy)
			if err == nil {
				m.root = newRoot
				m.rootEncoded = rootCopy
				m.tree.Reset(newRoot)
				m.pendingPath = nil
			}
		}
	}

	// Phase A: backup the path selected last step, if upstream has just
	// produced valid scores for it. The 1-step lag matches the
	// rollout-partition pipeline: rollout at step N reads tree's leaf
	// from step N within-step, broadcasts scores; tree at step N+1 reads
	// rollout's previous-step row (= scores for the leaf selected at
	// step N) and applies the backup here.
	if m.pendingPath != nil {
		if scores, ok := readUpstreamScores(params, stateHistories, m.Players); ok {
			m.tree.backupScores(m.pendingPath, scores)
		} else {
			// No-signal-tolerant: count visits even when scores are absent.
			// See MCTSTree.backupVisits docstring for why.
			m.tree.backupVisits(m.pendingPath, nil)
		}
		m.pendingPath = nil
	}

	// Phase B: walk the tree to select a new leaf and store it as the
	// pending path. The downstream rollout partition will read this leaf
	// within-step (via params_from_upstream); the resulting scores will
	// arrive at this partition next step (via state-history reading).
	step := uint64(timestepsHistory.CurrentStepNumber)
	rng := rand.New(rand.NewPCG(m.seed^step, uint64(partitionIndex+911)))
	path, leafState, _, ok := m.tree.SelectLeaf(m.Env, &cfg, rng)
	hasLeaf := false
	if ok {
		m.pendingPath = path
		hasLeaf = true
	} else {
		// MCTSTree is exhausted (terminal at root, or Apply error during
		// expansion). Leave pendingPath nil; the rollout partition will
		// see has_leaf=0 and skip.
		leafState = m.root
	}

	// Phase C: compose the output row.
	row := make([]float64, MCTSTreeRowWidth(m.StateWidth, m.MaxLegalActions))
	bestI, bestOK := m.tree.RootBestLegalIdx()
	if bestOK {
		row[MCTSTreeRowBestRootIdx] = float64(bestI)
	} else {
		row[MCTSTreeRowBestRootIdx] = -1
	}
	leafEnc := m.Encoder(leafState)
	if len(leafEnc) != m.StateWidth {
		panic("agents.MCTSTreePartition: encoder produced wrong width")
	}
	copy(row[MCTSTreeRowLeafStateOffset:], leafEnc)
	if hasLeaf {
		row[MCTSTreeRowHasLeafOffset(m.StateWidth)] = 1
	}
	visits, wins := m.tree.RootStatsByLegalIdx(m.MaxLegalActions)
	copy(row[MCTSTreeRowVisitsOffset(m.StateWidth):], visits)
	copy(row[MCTSTreeRowWinsOffset(m.StateWidth, m.MaxLegalActions):], wins)
	_ = stateHistories // not consumed (the row is written from scratch each step)
	return row
}

// readUpstreamScores reads the per-player scores published by an upstream
// MCTSRolloutPartition. Tries state-history mode first (lag-1, via
// params_as_partitions index lookup), then falls back to within-step
// mode (params_from_upstream). The expected layout in either case is
// [scores(P), ok_flag(1)] — the layout produced by MCTSRolloutPartition.
// Returns (nil, false) if neither source is present, the layout is
// malformed, or ok_flag is zero.
func readUpstreamScores(
	params *simulator.Params,
	stateHistories []*simulator.StateHistory,
	players int,
) ([]float64, bool) {
	if upstreamSlice, ok := params.GetOk(MCTSTreeParamRolloutScoresPartition); ok && len(upstreamSlice) > 0 {
		idx := int(upstreamSlice[0])
		if idx >= 0 && idx < len(stateHistories) {
			row := stateHistories[idx].Values.RawRowView(0)
			// Layout: [scores(P), ok(1), ...optional extra slots]. Some
			// upstream rollout partitions append per-rollout path data
			// after the ok flag (e.g. MASTRolloutPartition); we only
			// need the leading P+1 slots here, so accept any row whose
			// length is at least P+1.
			if len(row) >= players+1 && row[players] != 0 {
				out := make([]float64, players)
				copy(out, row[:players])
				return out, true
			}
		}
		return nil, false
	}
	v, present := params.GetOk(MCTSTreeParamRolloutScores)
	if !present {
		return nil, false
	}
	if len(v) != players+1 {
		return nil, false
	}
	if v[players] == 0 {
		return nil, false
	}
	out := make([]float64, players)
	copy(out, v[:players])
	return out, true
}

// MCTSTree exposes the underlying search tree (typically for telemetry).
func (m *MCTSTreePartition[S, A]) MCTSTree() *MCTSTree[S, A] {
	return m.tree
}
