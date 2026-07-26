package agents

// Internal tests for MCTSChanceTree: they reach into the node array, because the
// property that matters — that a chance node accumulates SEVERAL successors for
// one action, at a controlled rate — is invisible from the public surface.

import (
	"math"
	"math/rand/v2"
	"testing"
)

// coinEnv is a minimal stochastic environment: the state is a running total, and
// each action adds a draw that depends on the sample seed. Two actions, so a
// decision node has two chance children.
type coinEnv struct{ horizon float64 }

func (e coinEnv) Legal(s []float64) []int {
	if s[0] >= e.horizon {
		return nil
	}
	return []int{0, 1}
}

func (e coinEnv) Apply(s []float64, a int) ([]float64, error) {
	return e.ApplySample(s, a, 0)
}

func (e coinEnv) ApplySample(s []float64, a int, seed uint64) ([]float64, error) {
	rng := rand.New(rand.NewPCG(seed, uint64(a)+1))
	return []float64{s[0] + 1, s[1] + float64(a)*rng.Float64()}, nil
}

func (e coinEnv) Terminal(s []float64) ([]float64, bool) {
	if s[0] < e.horizon {
		return nil, false
	}
	return []float64{math.Min(1, math.Max(0, s[1]/e.horizon))}, true
}

func (e coinEnv) Actor([]float64) int   { return 0 }
func (e coinEnv) Players([]float64) int { return 1 }

var _ StochasticEnvironment[[]float64, int] = coinEnv{}

// runChanceIterations drives the tree the way RunChanceMCTSSearch does.
func runChanceIterations(
	tree *MCTSChanceTree[[]float64, int],
	env StochasticEnvironment[[]float64, int],
	cfg *MCTSConfig[[]float64, int],
	sims int,
) {
	for i := 0; i < sims; i++ {
		rng := rand.New(rand.NewPCG(uint64(i+1), uint64(i)*7+1))
		path, leaf, _, outcome := tree.SelectLeafWithOutcome(env, cfg, rng)
		switch outcome {
		case MCTSLeafTerminal:
			if scores, done := env.Terminal(leaf); done {
				tree.BackupScores(path, scores)
			}
		case MCTSLeafExpanded, MCTSLeafDepthCapped, MCTSLeafNoLegalActions:
			scores, ok, _ := cfg.Rollout(env, leaf, cfg.RolloutMaxSteps, rng.Uint64())
			if !ok {
				scores = nil
			}
			tree.BackupVisits(path, scores)
		}
	}
}

// TestChanceNodesAccumulateSeveralOutcomes is the defining behaviour: one action
// leads to MANY sampled successors, which is what makes its value an average over
// the transition distribution rather than a single draw.
func TestChanceNodesAccumulateSeveralOutcomes(t *testing.T) {
	env := coinEnv{horizon: 4}
	tree := NewMCTSChanceTree[[]float64, int]([]float64{0, 0})
	cfg := &MCTSConfig[[]float64, int]{
		MaxTreeDepth:    6,
		RolloutMaxSteps: 6,
		Rollout:         UniformRandomRollout[[]float64, int](),
	}
	runChanceIterations(tree, env, cfg, 400)

	root := tree.nodes[0]
	if len(root.children) != 2 {
		t.Fatalf("root should have one chance child per action, got %d", len(root.children))
	}
	for action, child := range root.children {
		chance := tree.nodes[child]
		if !chance.isChance {
			t.Fatalf("root child %d should be a chance node", action)
		}
		if len(chance.children) < 2 {
			t.Errorf("action %d accumulated only %d outcome(s) over %d visits; "+
				"without several the value is still one draw, not an average",
				action, len(chance.children), chance.visits)
		}
		// Distinct draws must actually differ, or the "distribution" is a point
		// mass and the sample seed is not reaching the environment.
		seen := make(map[float64]bool)
		for _, outcome := range chance.children {
			seen[tree.nodes[outcome].state[1]] = true
		}
		if action == 1 && len(seen) < 2 {
			t.Errorf("action %d: %d outcomes but only %d distinct successor states",
				action, len(chance.children), len(seen))
		}
	}
}

// TestChanceWideningRespectsItsBound pins the growth rule. Unbounded widening
// would draw a fresh successor on every visit, so no outcome would ever collect
// enough visits for its average to mean anything.
func TestChanceWideningRespectsItsBound(t *testing.T) {
	env := coinEnv{horizon: 4}
	cfg := &MCTSConfig[[]float64, int]{
		MaxTreeDepth:           6,
		RolloutMaxSteps:        6,
		Rollout:                UniformRandomRollout[[]float64, int](),
		ChanceWideningFactor:   1.0,
		ChanceWideningExponent: 0.5,
	}
	tree := NewMCTSChanceTree[[]float64, int]([]float64{0, 0})
	runChanceIterations(tree, env, cfg, 600)

	for index, node := range tree.nodes {
		if !node.isChance {
			continue
		}
		bound := int(math.Ceil(1.0 * math.Pow(float64(node.visits+1), 0.5)))
		if len(node.children) > bound {
			t.Errorf("chance node %d has %d outcomes after %d visits, above the "+
				"widening bound of %d", index, len(node.children), node.visits, bound)
		}
	}
}

// TestChanceTreeRejectsDeterministicEnvironment checks the search fails loudly
// rather than quietly degrading to a pinned-noise tree, which is the bias it
// exists to remove.
func TestChanceTreeRejectsDeterministicEnvironment(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic when the env cannot resample")
		}
	}()
	tree := NewMCTSChanceTree[TTTState, TTTAction](TTTState{})
	cfg := &MCTSConfig[TTTState, TTTAction]{}
	tree.SelectLeafWithOutcome(&TTTGame{}, cfg, rand.New(rand.NewPCG(1, 2)))
}

// TestChanceTreeAccessors covers the small surface MCTSTreeIteration drives it
// through, including Reset — which the self-play pipeline calls whenever the
// game advances and the old tree no longer applies.
func TestChanceTreeAccessors(t *testing.T) {
	env := coinEnv{horizon: 4}
	tree := NewMCTSChanceTree[[]float64, int]([]float64{0, 0})
	cfg := &MCTSConfig[[]float64, int]{
		MaxTreeDepth:    6,
		RolloutMaxSteps: 6,
		Rollout:         UniformRandomRollout[[]float64, int](),
	}
	if tree.NodeCount() != 1 || tree.Root()[0] != 0 {
		t.Fatalf("fresh tree should be a lone root, got %d nodes rooted at %v",
			tree.NodeCount(), tree.Root())
	}
	runChanceIterations(tree, env, cfg, 200)
	if tree.NodeCount() <= 1 {
		t.Fatal("the tree did not grow")
	}

	visits, wins := tree.RootStatsByLegalIdx(5)
	if len(visits) != 5 || len(wins) != 5 {
		t.Fatalf("stats should be padded to the requested width, got %d/%d",
			len(visits), len(wins))
	}
	total := 0.0
	for _, v := range visits {
		total += v
	}
	if total == 0 {
		t.Error("root actions recorded no visits")
	}
	if visits[2] != 0 || wins[2] != 0 {
		t.Errorf("padding slots should stay zero, got %v/%v", visits[2], wins[2])
	}

	// SelectLeaf is the boolean-shaped wrapper: true only on a real expansion.
	_, _, _, ok := tree.SelectLeaf(env, cfg, rand.New(rand.NewPCG(5, 6)))
	_ = ok

	tree.Reset([]float64{2, 0})
	if tree.NodeCount() != 1 || tree.Root()[0] != 2 {
		t.Fatalf("Reset should leave a lone root at the new state, got %d nodes at %v",
			tree.NodeCount(), tree.Root())
	}
	if visits, _ := tree.RootStatsByLegalIdx(2); visits[0] != 0 || visits[1] != 0 {
		t.Errorf("Reset should clear root statistics, got %v", visits)
	}
}

// TestRunChanceMCTSSearchGuards covers the two ways a search can be asked for
// something it cannot answer.
func TestRunChanceMCTSSearchGuards(t *testing.T) {
	cfg := MCTSConfig[[]float64, int]{Rollout: UniformRandomRollout[[]float64, int]()}

	if _, _, err := RunChanceMCTSSearch[[]float64, int](
		nil, []float64{0, 0}, cfg, 1, 10); err == nil {
		t.Error("expected an error for a nil environment")
	}
	// A terminal root offers no legal actions, so there is nothing to choose.
	if _, _, err := RunChanceMCTSSearch[[]float64, int](
		coinEnv{horizon: 4}, []float64{4, 0}, cfg, 1, 10); err == nil {
		t.Error("expected an error when the root has no legal actions")
	}
}
