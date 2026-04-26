package agents

import (
	"math"
	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// node is one position in the in-memory UCT tree. State is materialised
// once when the node is created so traversal does not have to replay actions.
type node[S any] struct {
	state    S
	parent   int
	legalIdx int // index in parent's Legal() that produced this node (-1 for root)
	actor    int // env.Actor(parent.state) when this edge was created
	visits   int
	wins     float64
	children []int // child node index per legal action; -1 if not yet expanded
	expanded bool
}

// MCTSTree is the in-memory UCT search tree for a fixed root state. Methods are
// not safe for concurrent use; one MCTSTree per goroutine.
//
// The MCTSTree owns no environment or config — those are passed in to RunOne
// per-simulation. This makes MCTSTree easy to embed in iterations that may want
// to tweak config between simulations (e.g. adaptive exploration).
type MCTSTree[S any, A any] struct {
	nodes []node[S]
}

// NewMCTSTree returns a MCTSTree with a single root node containing root.
func NewMCTSTree[S any, A any](root S) *MCTSTree[S, A] {
	return &MCTSTree[S, A]{
		nodes: []node[S]{{
			state:    root,
			parent:   -1,
			legalIdx: -1,
		}},
	}
}

// Reset replaces the entire tree with a fresh root at the given state. Use
// when no usable subtree exists (opening move, or after an external change
// to the root).
func (t *MCTSTree[S, A]) Reset(root S) {
	t.nodes = t.nodes[:0]
	t.nodes = append(t.nodes, node[S]{
		state:    root,
		parent:   -1,
		legalIdx: -1,
	})
}

// Root returns the root state.
func (t *MCTSTree[S, A]) Root() S {
	return t.nodes[0].state
}

// NodeCount returns the number of nodes currently in the tree (including
// the root). Useful for telemetry and capacity tuning.
func (t *MCTSTree[S, A]) NodeCount() int {
	return len(t.nodes)
}

// SelectLeaf walks the tree from the root using UCB1 (with first-visit
// preference for unvisited children) until it reaches an unexpanded edge,
// then expands it by creating a new child node. Returns the path of node
// indices from the root's child down to the new leaf, the leaf's state,
// the leaf's node index, and ok=true. Returns ok=false if the root is
// terminal, has no legal moves, MaxTreeDepth is reached, or env.Apply
// fails during expansion (the caller should treat this as "no leaf
// selected this step").
//
// SelectLeaf does NOT roll out and does NOT back up — it is the
// (selection + expansion) half of one MCTS iteration. Pair it with
// MCTSTree.BackupScores or MCTSTree.BackupVisits to apply the scores when they
// arrive.
//
// Calls cfg.applyDefaults() so a fresh MCTSConfig works out of the box. The
// mutation is idempotent (only zero values are filled).
func (t *MCTSTree[S, A]) SelectLeaf(env Environment[S, A], cfg *MCTSConfig[S, A], rng *rand.Rand) (path []int, leafState S, leafIdx int, ok bool) {
	cfg.applyDefaults()
	_ = rng // not currently consumed (selection is deterministic given UCB ties); kept for future tie-break randomisation
	path = make([]int, 0, 32)
	cur := 0
	depth := 0
	for {
		nd := &t.nodes[cur]
		if _, done := env.Terminal(nd.state); done {
			// Already-terminal node: caller can backupScores via this path.
			return path, nd.state, cur, false
		}
		if depth >= cfg.MaxTreeDepth {
			// Depth-capped: caller may want to score via progress proxy
			// downstream; we surface this leaf's state but signal not-ok
			// so the caller knows there's no real expansion.
			return path, nd.state, cur, false
		}
		if !nd.expanded {
			leg := env.Legal(nd.state)
			nd.children = make([]int, len(leg))
			for i := range nd.children {
				nd.children[i] = -1
			}
			nd.expanded = true
		}
		leg := env.Legal(nd.state)
		if len(leg) == 0 {
			return path, nd.state, cur, false
		}
		unvisited := -1
		for i, ci := range nd.children {
			if ci < 0 {
				unvisited = i
				break
			}
			if t.nodes[ci].visits == 0 {
				unvisited = i
				break
			}
		}
		var pickIdx int
		if unvisited >= 0 {
			pickIdx = unvisited
		} else {
			pickIdx = t.bestUCB(cur, cfg.Exploration)
		}
		ci := nd.children[pickIdx]
		if ci < 0 {
			ns, err := env.Apply(nd.state, leg[pickIdx])
			if err != nil {
				return path, nd.state, cur, false
			}
			child := len(t.nodes)
			t.nodes = append(t.nodes, node[S]{
				state:    ns,
				parent:   cur,
				legalIdx: pickIdx,
				actor:    env.Actor(nd.state),
			})
			t.nodes[cur].children[pickIdx] = child
			path = append(path, child)
			return path, ns, child, true
		}
		path = append(path, ci)
		cur = ci
		depth++
	}
}

// BackupScores credits each node along path with the score belonging to
// its actor (visits and wins both increment). Exported wrapper around the
// internal backupScores so iterations split across selection / rollout /
// backup partitions can apply the scores when they arrive.
func (t *MCTSTree[S, A]) BackupScores(path []int, scores []float64) {
	t.backupScores(path, scores)
}

// BackupVisits is the no-signal-tolerant variant: visits always increment,
// but wins are only credited when scores is non-nil. See backupVisits docs
// for the engine-stall reasoning.
func (t *MCTSTree[S, A]) BackupVisits(path []int, scores []float64) {
	t.backupVisits(path, scores)
}

// RootStatsByLegalIdx returns per-legal-action visit counts and win sums
// at the root, padded with zeros up to maxLegalActions. Returns
// (visits, wins) each of length maxLegalActions. Used to expose root
// statistics in fixed-width row layouts.
func (t *MCTSTree[S, A]) RootStatsByLegalIdx(maxLegalActions int) (visits, wins []float64) {
	visits = make([]float64, maxLegalActions)
	wins = make([]float64, maxLegalActions)
	root := &t.nodes[0]
	for i, ci := range root.children {
		if i >= maxLegalActions || ci < 0 {
			continue
		}
		c := &t.nodes[ci]
		visits[i] = float64(c.visits)
		wins[i] = c.wins
	}
	return visits, wins
}

// RunOne does one UCT iteration: selection → expansion → rollout → backup.
// rng must be seeded by the caller; one call uses one RNG.
//
// Calls cfg.applyDefaults() so a fresh MCTSConfig with only Rollout set works
// out of the box. The mutation is idempotent (only zero values are filled).
//
// RunOne is the all-in-one path used by RunMCTSSearch and by callers who
// don't need the selection / rollout / backup phases as separate stochadex
// partitions. For the decomposed pipeline use SelectLeaf + BackupScores.
func (t *MCTSTree[S, A]) RunOne(env Environment[S, A], cfg *MCTSConfig[S, A], rng *rand.Rand) {
	if cfg.Rollout == nil {
		return
	}
	cfg.applyDefaults()
	path := make([]int, 0, 32)
	cur := 0
	depth := 0
	for {
		nd := &t.nodes[cur]
		if scores, done := env.Terminal(nd.state); done {
			t.backupScores(path, scores)
			return
		}
		if depth >= cfg.MaxTreeDepth {
			scores, ok, _ := cfg.Rollout(env, nd.state, cfg.RolloutMaxSteps, rng.Uint64())
			if !ok {
				scores = nil
			}
			t.backupVisits(path, scores)
			return
		}
		if !nd.expanded {
			leg := env.Legal(nd.state)
			nd.children = make([]int, len(leg))
			for i := range nd.children {
				nd.children[i] = -1
			}
			nd.expanded = true
		}
		leg := env.Legal(nd.state)
		if len(leg) == 0 {
			scores, ok, _ := cfg.Rollout(env, nd.state, cfg.RolloutMaxSteps, rng.Uint64())
			if !ok {
				scores = nil
			}
			t.backupVisits(path, scores)
			return
		}
		// Pick an unvisited child if any.
		unvisited := -1
		for i, ci := range nd.children {
			if ci < 0 {
				unvisited = i
				break
			}
			if t.nodes[ci].visits == 0 {
				unvisited = i
				break
			}
		}
		var pickIdx int
		if unvisited >= 0 {
			pickIdx = unvisited
		} else {
			pickIdx = t.bestUCB(cur, cfg.Exploration)
		}
		ci := nd.children[pickIdx]
		if ci < 0 {
			ns, err := env.Apply(nd.state, leg[pickIdx])
			if err != nil {
				return
			}
			child := len(t.nodes)
			t.nodes = append(t.nodes, node[S]{
				state:    ns,
				parent:   cur,
				legalIdx: pickIdx,
				actor:    env.Actor(nd.state),
			})
			t.nodes[cur].children[pickIdx] = child
			path = append(path, child)
			scores, ok, _ := cfg.Rollout(env, ns, cfg.RolloutMaxSteps, rng.Uint64())
			if !ok {
				scores = nil
			}
			t.backupVisits(path, scores)
			return
		}
		path = append(path, ci)
		cur = ci
		depth++
	}
}

// bestUCB returns the child legal index with the highest UCB1 score under
// the given exploration constant. Ties are broken by first-listed; the root
// tiebreaker (RootBestLegalIdx) handles the bias-correction case.
func (t *MCTSTree[S, A]) bestUCB(parent int, exploration float64) int {
	pn := &t.nodes[parent]
	parentVis := 0
	for _, ci := range pn.children {
		if ci >= 0 {
			parentVis += t.nodes[ci].visits
		}
	}
	logP := math.Log(float64(parentVis))
	best := -1e18
	pick := 0
	for i, ci := range pn.children {
		if ci < 0 {
			continue
		}
		c := &t.nodes[ci]
		if c.visits == 0 {
			return i
		}
		mean := c.wins / float64(c.visits)
		ucb := mean + exploration*math.Sqrt(logP/float64(c.visits))
		if ucb > best {
			best = ucb
			pick = i
		}
	}
	return pick
}

// backupScores credits each node along path with the score belonging to its
// actor (the player whose decision created the edge). Visits and wins both
// increment. Used when the rollout returned a confident scores vector.
func (t *MCTSTree[S, A]) backupScores(path []int, scores []float64) {
	if len(scores) == 0 {
		return
	}
	for _, ni := range path {
		n := &t.nodes[ni]
		n.visits++
		if n.actor >= 0 && n.actor < len(scores) {
			n.wins += scores[n.actor]
		}
	}
}

// backupVisits is the "no-signal-tolerant" variant: visits always increment
// (so UCB exploration counts the trial), but wins are only credited when
// scores is non-nil. Without this, a long tail of truncated rollouts that
// give no progress signal would leave every child at visits=0, and RunOne
// would deadlock on the first-listed child of the root in environments
// where that first listed legal action is a stall move (pass / wait).
// This restores the standard MCTS behaviour where exploration eventually
// visits every child.
func (t *MCTSTree[S, A]) backupVisits(path []int, scores []float64) {
	for _, ni := range path {
		n := &t.nodes[ni]
		n.visits++
		if scores != nil && n.actor >= 0 && n.actor < len(scores) {
			n.wins += scores[n.actor]
		}
	}
}

// AdvanceRoot promotes the root's child at the given legal index to be the
// new root, preserving its subtree (classic MCTS tree reuse). If that child
// was never expanded, the tree is rebuilt fresh from the resulting state.
//
// env is needed to compute the post-move state if the subtree is missing.
func (t *MCTSTree[S, A]) AdvanceRoot(env Environment[S, A], legalIdx int) {
	root := &t.nodes[0]
	if !root.expanded || legalIdx < 0 || legalIdx >= len(root.children) || root.children[legalIdx] < 0 {
		leg := env.Legal(t.nodes[0].state)
		if legalIdx < 0 || legalIdx >= len(leg) {
			return
		}
		ns, err := env.Apply(t.nodes[0].state, leg[legalIdx])
		if err != nil {
			return
		}
		t.Reset(ns)
		return
	}
	keep := root.children[legalIdx]
	// BFS from `keep`, building a new node slice with remapped indices.
	newIndex := make(map[int]int, len(t.nodes))
	queue := []int{keep}
	newIndex[keep] = 0
	newNodes := make([]node[S], 0, len(t.nodes))
	newNodes = append(newNodes, node[S]{}) // placeholder for the new root
	for head := 0; head < len(queue); head++ {
		oldIdx := queue[head]
		old := t.nodes[oldIdx]
		ni := newIndex[oldIdx]
		nn := node[S]{
			state:    old.state,
			parent:   -1,
			legalIdx: -1,
			actor:    old.actor,
			visits:   old.visits,
			wins:     old.wins,
			expanded: old.expanded,
		}
		if old.expanded {
			newChildren := make([]int, len(old.children))
			for k, ci := range old.children {
				if ci < 0 {
					newChildren[k] = -1
					continue
				}
				if _, seen := newIndex[ci]; !seen {
					newIndex[ci] = len(queue)
					queue = append(queue, ci)
					newNodes = append(newNodes, node[S]{})
				}
				newChildren[k] = newIndex[ci]
			}
			nn.children = newChildren
		}
		newNodes[ni] = nn
	}
	// Second pass: set parent links.
	for oldIdx, ni := range newIndex {
		old := t.nodes[oldIdx]
		if old.parent < 0 {
			continue
		}
		if pni, ok := newIndex[old.parent]; ok {
			n := &newNodes[ni]
			n.parent = pni
			n.legalIdx = old.legalIdx
		}
	}
	t.nodes = newNodes
}

// RootBestLegalIdx returns the most-visited (then most-winning) child legal
// index. Ties are broken via reservoir sampling over equally-good children
// so the choice is not biased toward the first-listed action — important
// for engine-heavy games where the first listed legal action is often a
// stall (recycle, pass) and a deterministic first-tie pick would deadlock
// the agent. Reservoir randomness is seeded from the current tree shape so
// the result is reproducible without taking an external rng.
func (t *MCTSTree[S, A]) RootBestLegalIdx() (int, bool) {
	root := &t.nodes[0]
	rng := rand.New(rand.NewPCG(
		uint64(len(root.children))*0x9e3779b97f4a7c15+1,
		uint64(len(t.nodes))*0xbf58476d1ce4e5b9+1,
	))
	bestI := -1
	bestVisits := -1
	bestWins := -1.0
	tied := 0
	for i, ci := range root.children {
		if ci < 0 {
			continue
		}
		c := &t.nodes[ci]
		better := c.visits > bestVisits || (c.visits == bestVisits && c.wins > bestWins)
		eq := c.visits == bestVisits && c.wins == bestWins
		switch {
		case better:
			bestI, bestVisits, bestWins = i, c.visits, c.wins
			tied = 1
		case eq:
			tied++
			if rng.IntN(tied) == 0 {
				bestI = i
			}
		}
	}
	if bestI < 0 {
		return 0, false
	}
	return bestI, true
}

// RootEdgeStats reports per-action visit counts and mean-for-actor for each
// expanded child of the root. legal must be the same slice ordering used by
// the env's Legal(root); pass env.Legal(tree.Root()) at the call site.
func (t *MCTSTree[S, A]) RootEdgeStats(legal []A) []MCTSEdgeStat[A] {
	root := &t.nodes[0]
	stats := make([]MCTSEdgeStat[A], 0, len(root.children))
	for i, ci := range root.children {
		if ci < 0 || i >= len(legal) {
			continue
		}
		c := &t.nodes[ci]
		mean := 0.0
		if c.visits > 0 {
			mean = c.wins / float64(c.visits)
		}
		stats = append(stats, MCTSEdgeStat[A]{Action: legal[i], Visits: c.visits, MeanForActor: mean})
	}
	return stats
}

// MCTSTreeIteration runs the (selection + expansion + backup) phase of a UCT
// MCTS search as a stochadex iteration. The tree itself lives on the
// struct (graph state, fundamentally not []float64-shaped); the partition
// row exposes a fixed-width summary that downstream partitions can consume
// via params_from_upstream:
//
//	row[MCTSTreeRowBestRootIdx]                        — most-visited root
//	                                                 legal-action index
//	                                                 after the most recent
//	                                                 update (-1 if not
//	                                                 yet decided)
//	row[MCTSTreeRowLeafStateOffset .. +StateWidth-1]   — encoded state of the
//	                                                 leaf the search just
//	                                                 selected (input to a
//	                                                 rollout partition)
//	row[MCTSTreeRowHasLeafOffset(W)]                   — 1 if the leaf_state
//	                                                 slot is real, 0
//	                                                 otherwise (used by
//	                                                 rollout partitions to
//	                                                 short-circuit when
//	                                                 nothing has been
//	                                                 selected yet)
//	row[MCTSTreeRowVisitsOffset(W) .. +MaxLegalActions-1] — per-legal-action
//	                                                    root visit counts,
//	                                                    padded with zeros
//	row[MCTSTreeRowWinsOffset(W,K) .. +MaxLegalActions-1] — per-legal-action
//	                                                    root win sums,
//	                                                    padded with zeros
//
// Use MCTSTreeRowWidth(W, K) to compute the required state_width / init slice
// length.
//
// # Pipeline lag
//
// MCTSTreeIteration is one half of a 2-step pipeline with a downstream rollout
// partition. The rollout partition reads (leaf_state, has_leaf) and
// outputs scores; MCTSTreeIteration then reads those scores via
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
//     left nil if a separate MCTSRolloutIteration supplies the scores).
//   - Decoder, Encoder: codec for the leaf state slots.
//   - MaxLegalActions: K, the maximum legal-action count at any node.
//     Stats slots beyond the actual legal count are zero-padded.
//   - StateWidth: W, the width of one encoded state.
//   - Players: P, the per-player score vector length.
type MCTSTreeIteration[S any, A any] struct {
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

// Param key used by MCTSTreeIteration.Iterate to read rollout scores from an
// upstream MCTSRolloutIteration via params_from_upstream (within-step). The
// slice should be of length Players + 1 (P scores + ok flag) — the layout
// produced by MCTSRolloutIteration.
//
// This mode creates a within-step dependency that breaks if the rollout
// partition also depends on the tree (the standard MCTS pipeline does).
// Use MCTSTreeParamRolloutScoresPartition for the lag-1 state-history mode
// instead when wiring the tree + rollout pipeline.
const MCTSTreeParamRolloutScores = "rollout_scores"

// Param key used by MCTSTreeIteration.Iterate to read rollout scores from an
// upstream MCTSRolloutIteration via params_as_partitions (state-history,
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

// Param key used by MCTSTreeIteration.Iterate to read the current search
// root state. Whenever this param's value differs from the cached root
// encoding, the tree is reset to the decoded new root. The slice should
// be of length StateWidth.
//
// Set this via the embedded simulation run's outer params_from_upstream
// (e.g. an outer apply partition piping its current game state into the
// inner sim via the "<innerName>/root_state" forwarding mechanism). When
// MCTSTreeIteration is used standalone with no outer pipeline, the param is
// absent and the tree retains the root set at Configure time.
const MCTSTreeParamRootState = "root_state"

// Row layout slot accessors. Use these to compute params_from_upstream
// indices when wiring downstream partitions to MCTSTreeIteration's row.
const MCTSTreeRowBestRootIdx = 0
const MCTSTreeRowLeafStateOffset = 1

func MCTSTreeRowHasLeafOffset(stateWidth int) int { return 1 + stateWidth }
func MCTSTreeRowVisitsOffset(stateWidth int) int  { return 2 + stateWidth }
func MCTSTreeRowWinsOffset(stateWidth, maxLegalActions int) int {
	return 2 + stateWidth + maxLegalActions
}

// MCTSTreeRowWidth returns the required InitStateValues / StateWidth for a
// MCTSTreeIteration with the given encoded-state width and max legal-action
// count.
func MCTSTreeRowWidth(stateWidth, maxLegalActions int) int {
	return 2 + stateWidth + 2*maxLegalActions
}

// Configure implements simulator.Iteration. Decodes the encoded root from
// is.InitStateValues[1 .. 1+StateWidth] and resets the tree. The first
// slot (MCTSTreeRowBestRootIdx) and the stats slots can be left zero in the
// init: best_root_idx is initialised to -1 to signal "not yet decided".
func (m *MCTSTreeIteration[S, A]) Configure(partitionIndex int, settings *simulator.Settings) {
	if m.Env == nil {
		panic("agents.MCTSTreeIteration: Env required")
	}
	if m.Decoder == nil {
		panic("agents.MCTSTreeIteration: Decoder required")
	}
	if m.Encoder == nil {
		panic("agents.MCTSTreeIteration: Encoder required")
	}
	if m.MaxLegalActions <= 0 {
		panic("agents.MCTSTreeIteration: MaxLegalActions must be > 0")
	}
	if m.StateWidth <= 0 {
		panic("agents.MCTSTreeIteration: StateWidth must be > 0")
	}
	if m.Players <= 0 {
		panic("agents.MCTSTreeIteration: Players must be > 0")
	}
	is := settings.Iterations[partitionIndex]
	expected := MCTSTreeRowWidth(m.StateWidth, m.MaxLegalActions)
	if is.StateWidth != expected {
		panic("agents.MCTSTreeIteration: StateWidth must equal MCTSTreeRowWidth(W, K)")
	}
	encoded := make([]float64, m.StateWidth)
	copy(encoded, is.InitStateValues[MCTSTreeRowLeafStateOffset:MCTSTreeRowLeafStateOffset+m.StateWidth])
	root, err := m.Decoder(encoded)
	if err != nil {
		panic("agents.MCTSTreeIteration: decoder failed: " + err.Error())
	}
	m.root = root
	m.rootEncoded = encoded
	m.tree = NewMCTSTree[S, A](root)
	m.pendingPath = nil
	m.seed = is.Seed
}

// Iterate implements simulator.Iteration.
func (m *MCTSTreeIteration[S, A]) Iterate(
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
		panic("agents.MCTSTreeIteration: encoder produced wrong width")
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
// MCTSRolloutIteration. Tries state-history mode first (lag-1, via
// params_as_partitions index lookup), then falls back to within-step
// mode (params_from_upstream). The expected layout in either case is
// [scores(P), ok_flag(1)] — the layout produced by MCTSRolloutIteration.
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
			// after the ok flag (e.g. MASTRolloutIteration); we only
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
func (m *MCTSTreeIteration[S, A]) MCTSTree() *MCTSTree[S, A] {
	return m.tree
}
