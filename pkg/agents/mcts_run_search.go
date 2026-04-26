package agents

import (
	"fmt"
	"math/rand/v2"
)

// RunMCTSSearch runs sims UCT simulations from root and returns the best legal
// action plus per-edge stats. Independent of stochadex partitions — useful
// for one-shot "what's the best move?" queries.
//
// Defaults are filled in from the package constants. If cfg.Rollout is nil,
// UniformRandomRollout is used.
func RunMCTSSearch[S any, A any](
	env Environment[S, A],
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
	leg := env.Legal(root)
	if len(leg) == 0 {
		return zero, nil, fmt.Errorf("mcts: no legal actions")
	}
	tree := NewMCTSTree[S, A](root)
	for i := 0; i < sims; i++ {
		rng := rand.New(rand.NewPCG(
			baseSeed^uint64(i+1),
			uint64(i)*0x9e3779b97f4a7c15+1,
		))
		tree.RunOne(env, &cfg, rng)
	}
	bestI, ok := tree.RootBestLegalIdx()
	if !ok || bestI < 0 || bestI >= len(leg) {
		return leg[0], nil, nil
	}
	return leg[bestI], tree.RootEdgeStats(leg), nil
}
