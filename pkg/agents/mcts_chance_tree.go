package agents

import (
	"math"
	"math/rand/v2"
)

// MCTSChanceTree is the UCT tree for genuinely stochastic environments: it
// separates a decision from its outcome, so a value is an average over sampled
// successors rather than a commitment to the first one drawn.
//
// # Why this exists alongside MCTSTree
//
// MCTSTree materialises one successor per action edge and reuses it. For a
// deterministic environment that is exactly right and costs nothing. For a
// stochastic one it silently pins the noise: the search solves a single
// realisation and can exploit the particular draw it is going to receive, so the
// value it reports is optimistic relative to the expectation.
//
// The two are kept separate rather than merged behind a flag because the game
// path is deterministic, shipped, and would pay complexity for a case it never
// hits — and because the algorithms genuinely differ, rather than one being a
// parameterisation of the other.
//
// # Shape
//
// Levels alternate. A DECISION node holds a state and one chance child per legal
// action. A CHANCE node holds no state; it stands for "action a was taken from
// the parent's state, and nature has not resolved yet", and its children are
// sampled successors. Action selection at a decision node is UCB1 over the chance
// children's aggregate statistics — which are, by construction, averages over
// outcomes. That is the whole point.
//
// # Progressive widening
//
// A chance node cannot enumerate its outcomes: for a continuous-noise model there
// are infinitely many. Instead the number of sampled outcomes is allowed to grow
// with the number of visits, as ceil(k * visits^alpha) with alpha in (0,1). Early
// on a chance node commits to few outcomes so statistics accumulate; as it is
// visited more, new draws are added and the average converges on the expectation.
// Without the widening bound, every visit would sample a fresh successor and no
// node would ever collect enough visits to be worth anything.
type MCTSChanceTree[S any, A any] struct {
	nodes []chanceTreeNode[S]
}

// chanceTreeNode is one node of MCTSChanceTree. The isChance flag distinguishes
// the two levels; state is meaningful only on decision nodes.
type chanceTreeNode[S any] struct {
	state    S
	parent   int
	isChance bool
	// legalIdx is the parent's legal-action index that created this node
	// (chance nodes only; -1 on decision nodes).
	legalIdx int
	// actor is who chose the action leading here, so backups credit the right
	// player. A chance node inherits its deciding player's index.
	actor    int
	visits   int
	wins     float64
	children []int
	expanded bool
	// sampleCount counts draws taken at a chance node, so each new outcome gets
	// a distinct sample seed.
	sampleCount int
}

// Default progressive-widening parameters. alpha = 0.5 is the usual choice: the
// outcome count grows as the square root of the visit count, which keeps enough
// visits per outcome for their averages to mean something.
const (
	MCTSDefaultChanceWideningFactor   = 1.0
	MCTSDefaultChanceWideningExponent = 0.5
)

// NewMCTSChanceTree returns a tree with a single decision node at root.
func NewMCTSChanceTree[S any, A any](root S) *MCTSChanceTree[S, A] {
	return &MCTSChanceTree[S, A]{
		nodes: []chanceTreeNode[S]{{state: root, parent: -1, legalIdx: -1}},
	}
}

// Reset replaces the tree with a fresh root.
func (t *MCTSChanceTree[S, A]) Reset(root S) {
	t.nodes = t.nodes[:0]
	t.nodes = append(t.nodes, chanceTreeNode[S]{state: root, parent: -1, legalIdx: -1})
}

// Root returns the root state.
func (t *MCTSChanceTree[S, A]) Root() S { return t.nodes[0].state }

// NodeCount returns the number of nodes, decision and chance alike.
func (t *MCTSChanceTree[S, A]) NodeCount() int { return len(t.nodes) }

// SelectLeafWithOutcome walks from the root to a leaf, alternating UCB1 action
// choices at decision nodes with sampled outcomes at chance nodes, and reports
// why it stopped. It mirrors MCTSTree.SelectLeafWithOutcome so the same
// backup rules apply, and pairs with BackupScores / BackupVisits.
func (t *MCTSChanceTree[S, A]) SelectLeafWithOutcome(
	env Environment[S, A],
	cfg *MCTSConfig[S, A],
	rng *rand.Rand,
) (path []int, leafState S, leafIdx int, outcome MCTSLeafOutcome) {
	cfg.applyDefaults()
	sampler, ok := env.(StochasticEnvironment[S, A])
	if !ok {
		// Without a way to resample there are no chance nodes to build. Fail
		// loudly rather than silently degrading to a pinned-noise search, which
		// is the bias this type exists to remove.
		panic("agents.MCTSChanceTree: env must implement StochasticEnvironment")
	}
	factor, exponent := chanceWidening(cfg)

	path = make([]int, 0, 32)
	cur := 0
	depth := 0
	for {
		node := &t.nodes[cur]
		if !node.isChance {
			if _, done := env.Terminal(node.state); done {
				return path, node.state, cur, MCTSLeafTerminal
			}
			if depth >= cfg.MaxTreeDepth {
				return path, node.state, cur, MCTSLeafDepthCapped
			}
			legal := env.Legal(node.state)
			if len(legal) == 0 {
				return path, node.state, cur, MCTSLeafNoLegalActions
			}
			if !node.expanded {
				node.children = make([]int, len(legal))
				for i := range node.children {
					node.children[i] = -1
				}
				node.expanded = true
			}
			pick := t.bestAction(cur, cfg.Exploration)
			if node.children[pick] < 0 {
				// Creating a chance node costs nothing: no state, no env call.
				// Nature resolves on the next iteration of this loop.
				child := len(t.nodes)
				t.nodes = append(t.nodes, chanceTreeNode[S]{
					parent:   cur,
					isChance: true,
					legalIdx: pick,
					actor:    env.Actor(node.state),
				})
				t.nodes[cur].children[pick] = child
			}
			next := t.nodes[cur].children[pick]
			path = append(path, next)
			cur = next
			continue
		}

		// Chance node: either widen with a fresh draw, or revisit an outcome.
		parent := &t.nodes[node.parent]
		legal := env.Legal(parent.state)
		if node.legalIdx >= len(legal) {
			return path, parent.state, cur, MCTSLeafApplyFailed
		}
		allowed := int(math.Ceil(factor * math.Pow(float64(node.visits+1), exponent)))
		if allowed < 1 {
			allowed = 1
		}
		if len(node.children) < allowed {
			node.sampleCount++
			// Seed from the node's identity and draw index so a given outcome is
			// reproducible, and two outcomes of one chance node never coincide.
			seed := uint64(cur+1)*0x9e3779b97f4a7c15 ^ uint64(node.sampleCount)
			sampled, err := sampler.ApplySample(parent.state, legal[node.legalIdx], seed)
			if err != nil {
				return path, parent.state, cur, MCTSLeafApplyFailed
			}
			child := len(t.nodes)
			t.nodes = append(t.nodes, chanceTreeNode[S]{
				state:    sampled,
				parent:   cur,
				legalIdx: -1,
				actor:    t.nodes[cur].actor,
			})
			t.nodes[cur].children = append(t.nodes[cur].children, child)
			path = append(path, child)
			return path, sampled, child, MCTSLeafExpanded
		}
		// Revisit an existing outcome, uniformly. Each was drawn from the
		// transition distribution, so visiting them equally keeps the running
		// average an unbiased estimate of the expectation.
		pick := node.children[rng.IntN(len(node.children))]
		path = append(path, pick)
		cur = pick
		depth++
	}
}

// SelectLeaf is the boolean-shaped form, matching MCTSTree.SelectLeaf.
func (t *MCTSChanceTree[S, A]) SelectLeaf(
	env Environment[S, A],
	cfg *MCTSConfig[S, A],
	rng *rand.Rand,
) (path []int, leafState S, leafIdx int, ok bool) {
	path, leafState, leafIdx, outcome := t.SelectLeafWithOutcome(env, cfg, rng)
	return path, leafState, leafIdx, outcome == MCTSLeafExpanded
}

// BackupScores credits each node on path with its actor's score.
func (t *MCTSChanceTree[S, A]) BackupScores(path []int, scores []float64) {
	if len(scores) == 0 {
		return
	}
	t.BackupVisits(path, scores)
}

// BackupVisits increments visits along path, crediting wins only when scores are
// present — the no-signal-tolerant form, for the same reason as MCTSTree's.
func (t *MCTSChanceTree[S, A]) BackupVisits(path []int, scores []float64) {
	for _, index := range path {
		node := &t.nodes[index]
		node.visits++
		if scores != nil && node.actor >= 0 && node.actor < len(scores) {
			node.wins += scores[node.actor]
		}
	}
}

// RootStatsByLegalIdx returns per-action visits and win sums at the root, padded
// to maxLegalActions. The statistics come from the chance children, so each is
// already an average over that action's sampled outcomes.
func (t *MCTSChanceTree[S, A]) RootStatsByLegalIdx(
	maxLegalActions int,
) (visits, wins []float64) {
	visits = make([]float64, maxLegalActions)
	wins = make([]float64, maxLegalActions)
	root := &t.nodes[0]
	for action, child := range root.children {
		if action >= maxLegalActions || child < 0 {
			continue
		}
		visits[action] = float64(t.nodes[child].visits)
		wins[action] = t.nodes[child].wins
	}
	return visits, wins
}

// RootBestLegalIdx returns the most-visited action at the root.
func (t *MCTSChanceTree[S, A]) RootBestLegalIdx() (int, bool) {
	root := &t.nodes[0]
	best, bestVisits := -1, -1
	for action, child := range root.children {
		if child < 0 {
			continue
		}
		if visits := t.nodes[child].visits; visits > bestVisits {
			best, bestVisits = action, visits
		}
	}
	return best, best >= 0
}

// bestAction picks the root-to-leaf action at a decision node: any never-tried
// action first, then UCB1 over the chance children's averages.
func (t *MCTSChanceTree[S, A]) bestAction(parent int, exploration float64) int {
	node := &t.nodes[parent]
	for action, child := range node.children {
		if child < 0 || t.nodes[child].visits == 0 {
			return action
		}
	}
	best, bestScore := 0, math.Inf(-1)
	total := 0
	for _, child := range node.children {
		total += t.nodes[child].visits
	}
	logTotal := math.Log(float64(max(total, 1)))
	for action, child := range node.children {
		stats := &t.nodes[child]
		mean := stats.wins / float64(stats.visits)
		score := mean + exploration*math.Sqrt(logTotal/float64(stats.visits))
		if score > bestScore {
			best, bestScore = action, score
		}
	}
	return best
}

// chanceWidening resolves the progressive-widening parameters, defaulting when
// unset so a bare MCTSConfig works.
func chanceWidening[S any, A any](cfg *MCTSConfig[S, A]) (factor, exponent float64) {
	factor, exponent = cfg.ChanceWideningFactor, cfg.ChanceWideningExponent
	if factor <= 0 {
		factor = MCTSDefaultChanceWideningFactor
	}
	if exponent <= 0 || exponent >= 1 {
		exponent = MCTSDefaultChanceWideningExponent
	}
	return factor, exponent
}
