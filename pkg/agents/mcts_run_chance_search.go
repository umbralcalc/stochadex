package agents

import (
	"fmt"
	"math/rand/v2"
)

// RunChanceMCTSSearch is RunMCTSSearch over an MCTSChanceTree, so each action's
// value averages over sampled outcomes. Prefer it whenever the environment has a
// real transition distribution and the value it reports will be believed.
func RunChanceMCTSSearch[S any, A any](
	env StochasticEnvironment[S, A],
	root S,
	cfg MCTSConfig[S, A],
	baseSeed uint64,
	sims int,
) (A, []MCTSEdgeStat[A], error) {
	var zero A
	if env == nil {
		return zero, nil, fmt.Errorf("mcts: env is nil")
	}
	cfg.applyDefaults()
	if cfg.Rollout == nil {
		cfg.Rollout = UniformRandomRollout[S, A]()
	}
	if sims < 1 {
		sims = cfg.Simulations
	}
	legal := env.Legal(root)
	if len(legal) == 0 {
		return zero, nil, fmt.Errorf("mcts: no legal actions")
	}

	tree := NewMCTSChanceTree[S, A](root)
	for i := 0; i < sims; i++ {
		rng := rand.New(rand.NewPCG(
			baseSeed^uint64(i+1),
			uint64(i)*0x9e3779b97f4a7c15+1,
		))
		path, leafState, _, outcome := tree.SelectLeafWithOutcome(env, &cfg, rng)
		switch outcome {
		case MCTSLeafTerminal:
			// Exact scores from the environment; no rollout needed.
			if scores, done := env.Terminal(leafState); done {
				tree.BackupScores(path, scores)
			}
		case MCTSLeafExpanded, MCTSLeafDepthCapped, MCTSLeafNoLegalActions:
			scores, ok, _ := cfg.Rollout(env, leafState, cfg.RolloutMaxSteps, rng.Uint64())
			if !ok {
				scores = nil
			}
			tree.BackupVisits(path, scores)
		case MCTSLeafApplyFailed:
			// An environment fault, not a search result: back up nothing.
		}
	}

	best, ok := tree.RootBestLegalIdx()
	if !ok || best < 0 || best >= len(legal) {
		return legal[0], nil, nil
	}
	return legal[best], tree.rootEdgeStats(legal), nil
}

// rootEdgeStats reports per-action telemetry at the root, mirroring
// MCTSTree.RootEdgeStats.
func (t *MCTSChanceTree[S, A]) rootEdgeStats(legal []A) []MCTSEdgeStat[A] {
	stats := make([]MCTSEdgeStat[A], 0, len(legal))
	root := &t.nodes[0]
	for action, child := range root.children {
		if action >= len(legal) || child < 0 {
			continue
		}
		node := &t.nodes[child]
		mean := 0.0
		if node.visits > 0 {
			mean = node.wins / float64(node.visits)
		}
		stats = append(stats, MCTSEdgeStat[A]{
			Action:       legal[action],
			Visits:       node.visits,
			MeanForActor: mean,
		})
	}
	return stats
}
