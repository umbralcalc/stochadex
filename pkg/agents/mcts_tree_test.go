package agents_test

// Direct unit tests for the MCTSTree value type. These exercise the tree's
// methods (SelectLeaf, BackupScores, BackupVisits, AdvanceRoot,
// RootStatsByLegalIdx, RootBestLegalIdx, RunOne) independently of the
// stochadex partition machinery. Use the shared tic-tac-toe environment
// in pkg/mcts/mctstest as a deterministic, easy-to-reason-about driver.

import (
	"math/rand/v2"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/agents"
	"github.com/umbralcalc/stochadex/pkg/agents/agentstest"
)

func TestNewMCTSTreeStartsWithJustRoot(t *testing.T) {
	tree := agents.NewMCTSTree[agentstest.TTTState, agentstest.TTTAction](agentstest.TTTState{})
	if tree.NodeCount() != 1 {
		t.Fatalf("expected 1 node (root only), got %d", tree.NodeCount())
	}
	root := tree.Root()
	for i, c := range root.Cells {
		if c != 0 {
			t.Fatalf("expected empty root, got cell[%d]=%d", i, c)
		}
	}
}

func TestSelectLeafExpandsRootChildOnEmptyTree(t *testing.T) {
	env := &agentstest.TTTGame{}
	tree := agents.NewMCTSTree[agentstest.TTTState, agentstest.TTTAction](agentstest.TTTState{})
	cfg := &agents.MCTSConfig[agentstest.TTTState, agentstest.TTTAction]{}
	rng := rand.New(rand.NewPCG(1, 2))

	path, _, leafIdx, ok := tree.SelectLeaf(env, cfg, rng)
	if !ok {
		t.Fatal("expected ok=true on a fresh tree")
	}
	if len(path) != 1 {
		t.Fatalf("expected single-step path on first selection, got %v", path)
	}
	if path[0] != leafIdx {
		t.Fatalf("path tail should equal leafIdx, got path=%v leafIdx=%d", path, leafIdx)
	}
	if tree.NodeCount() != 2 {
		t.Fatalf("expected 2 nodes after expansion (root + new leaf), got %d", tree.NodeCount())
	}
}

func TestSelectLeafReturnsOkFalseWhenRootIsTerminal(t *testing.T) {
	// X has already won; the root is terminal.
	root := agentstest.TTTFromGrid([9]int8{1, 1, 1, 2, 2, 0, 0, 0, 0}, 1)
	env := &agentstest.TTTGame{}
	tree := agents.NewMCTSTree[agentstest.TTTState, agentstest.TTTAction](root)
	cfg := &agents.MCTSConfig[agentstest.TTTState, agentstest.TTTAction]{}
	rng := rand.New(rand.NewPCG(1, 2))

	_, _, _, ok := tree.SelectLeaf(env, cfg, rng)
	if ok {
		t.Fatal("expected ok=false when root is terminal")
	}
	if tree.NodeCount() != 1 {
		t.Fatalf("terminal SelectLeaf must not grow the tree, got %d nodes", tree.NodeCount())
	}
}

func TestSelectLeafReturnsOkFalseAtDepthCap(t *testing.T) {
	env := &agentstest.TTTGame{}
	tree := agents.NewMCTSTree[agentstest.TTTState, agentstest.TTTAction](agentstest.TTTState{})
	cfg := &agents.MCTSConfig[agentstest.TTTState, agentstest.TTTAction]{MaxTreeDepth: 1}
	rng := rand.New(rand.NewPCG(1, 2))

	// First SelectLeaf: expands one root child; depth reached after path
	// length 1. ok=true since we expand.
	_, _, _, ok := tree.SelectLeaf(env, cfg, rng)
	if !ok {
		t.Fatal("expected ok=true on first SelectLeaf with depth cap 1")
	}
	// Second SelectLeaf: walks down to that child. depth becomes 1, which
	// equals MaxTreeDepth. Returns ok=false (no expansion past the cap).
	_, _, _, ok = tree.SelectLeaf(env, cfg, rng)
	if ok {
		t.Fatal("expected ok=false at depth cap")
	}
}

func TestBackupScoresCreditsActorAlongPath(t *testing.T) {
	env := &agentstest.TTTGame{}
	tree := agents.NewMCTSTree[agentstest.TTTState, agentstest.TTTAction](agentstest.TTTState{})
	cfg := &agents.MCTSConfig[agentstest.TTTState, agentstest.TTTAction]{}
	rng := rand.New(rand.NewPCG(1, 2))

	path, _, _, ok := tree.SelectLeaf(env, cfg, rng)
	if !ok {
		t.Fatal("expected ok=true")
	}
	// The expanded leaf was created from the empty board with X to move
	// (player 0). Backup with score [1, 0] (X wins) must credit the leaf.
	tree.BackupScores(path, []float64{1.0, 0.0})

	visits, wins := tree.RootStatsByLegalIdx(9)
	nonzeroVisits := 0
	nonzeroWins := 0
	for i := 0; i < 9; i++ {
		if visits[i] != 0 {
			nonzeroVisits++
			if visits[i] != 1 {
				t.Fatalf("expected exactly 1 visit at slot %d, got %v", i, visits[i])
			}
		}
		if wins[i] != 0 {
			nonzeroWins++
			if wins[i] != 1.0 {
				t.Fatalf("expected exactly 1.0 win at slot %d, got %v", i, wins[i])
			}
		}
	}
	if nonzeroVisits != 1 || nonzeroWins != 1 {
		t.Fatalf("expected one visit and one win; got %d visits, %d wins", nonzeroVisits, nonzeroWins)
	}
}

func TestBackupScoresIgnoresEmptyPath(t *testing.T) {
	env := &agentstest.TTTGame{}
	tree := agents.NewMCTSTree[agentstest.TTTState, agentstest.TTTAction](agentstest.TTTState{})
	tree.BackupScores(nil, []float64{1, 0})
	tree.BackupScores([]int{}, []float64{1, 0})
	visits, _ := tree.RootStatsByLegalIdx(9)
	for i, v := range visits {
		if v != 0 {
			t.Fatalf("empty-path backup should not modify tree, got visits[%d]=%v", i, v)
		}
	}
	_ = env
}

func TestBackupScoresIgnoresEmptyScores(t *testing.T) {
	env := &agentstest.TTTGame{}
	tree := agents.NewMCTSTree[agentstest.TTTState, agentstest.TTTAction](agentstest.TTTState{})
	cfg := &agents.MCTSConfig[agentstest.TTTState, agentstest.TTTAction]{}
	rng := rand.New(rand.NewPCG(1, 2))
	path, _, _, ok := tree.SelectLeaf(env, cfg, rng)
	if !ok {
		t.Fatal("expected ok=true")
	}
	tree.BackupScores(path, nil)
	visits, _ := tree.RootStatsByLegalIdx(9)
	for i, v := range visits {
		if v != 0 {
			t.Fatalf("empty-scores backup should not modify tree, got visits[%d]=%v", i, v)
		}
	}
}

func TestBackupVisitsIncrementsWithoutWinCredit(t *testing.T) {
	env := &agentstest.TTTGame{}
	tree := agents.NewMCTSTree[agentstest.TTTState, agentstest.TTTAction](agentstest.TTTState{})
	cfg := &agents.MCTSConfig[agentstest.TTTState, agentstest.TTTAction]{}
	rng := rand.New(rand.NewPCG(1, 2))

	path, _, _, ok := tree.SelectLeaf(env, cfg, rng)
	if !ok {
		t.Fatal("expected ok=true")
	}
	// Pass nil scores: visit should still count (no-signal-tolerant).
	tree.BackupVisits(path, nil)

	visits, wins := tree.RootStatsByLegalIdx(9)
	nonzeroVisits := 0
	for i := 0; i < 9; i++ {
		if visits[i] != 0 {
			nonzeroVisits++
		}
		if wins[i] != 0 {
			t.Fatalf("BackupVisits with nil scores must not credit wins, got wins[%d]=%v", i, wins[i])
		}
	}
	if nonzeroVisits != 1 {
		t.Fatalf("expected exactly one visit credited, got %d", nonzeroVisits)
	}
}

func TestRootStatsByLegalIdxPadsToMaxK(t *testing.T) {
	env := &agentstest.TTTGame{}
	tree := agents.NewMCTSTree[agentstest.TTTState, agentstest.TTTAction](agentstest.TTTState{})
	// Exercise different K values with no expansions: all should return
	// zero-padded slices of the requested length.
	for _, k := range []int{1, 9, 50} {
		visits, wins := tree.RootStatsByLegalIdx(k)
		if len(visits) != k || len(wins) != k {
			t.Fatalf("RootStatsByLegalIdx(%d): got len(visits)=%d len(wins)=%d", k, len(visits), len(wins))
		}
		for i := 0; i < k; i++ {
			if visits[i] != 0 || wins[i] != 0 {
				t.Fatalf("RootStatsByLegalIdx(%d): expected zeros, got visits[%d]=%v wins[%d]=%v", k, i, visits[i], i, wins[i])
			}
		}
	}
	_ = env
}

func TestAdvanceRootResetsWhenSubtreeMissing(t *testing.T) {
	env := &agentstest.TTTGame{}
	tree := agents.NewMCTSTree[agentstest.TTTState, agentstest.TTTAction](agentstest.TTTState{})
	// No simulations run, so root has no expanded children yet.
	tree.AdvanceRoot(env, 0)
	if tree.NodeCount() != 1 {
		t.Fatalf("expected fresh tree (1 node) after AdvanceRoot with no subtree, got %d", tree.NodeCount())
	}
	root := tree.Root()
	if root.Cells[0] != 1 {
		t.Fatalf("expected new root to have X at cell 0 (legal[0] applied to empty board), got cells=%v", root.Cells)
	}
}

func TestAdvanceRootIgnoresOutOfRangeIdx(t *testing.T) {
	env := &agentstest.TTTGame{}
	tree := agents.NewMCTSTree[agentstest.TTTState, agentstest.TTTAction](agentstest.TTTState{})
	tree.AdvanceRoot(env, 99) // out of range
	if tree.NodeCount() != 1 {
		t.Fatalf("AdvanceRoot with bad idx must be a no-op, got %d nodes", tree.NodeCount())
	}
	if tree.Root().Cells[0] != 0 {
		t.Fatalf("AdvanceRoot with bad idx must not advance state")
	}
}

func TestAdvanceRootPreservesSubtreeWhenAvailable(t *testing.T) {
	env := &agentstest.TTTGame{}
	tree := agents.NewMCTSTree[agentstest.TTTState, agentstest.TTTAction](agentstest.TTTState{})
	cfg := &agents.MCTSConfig[agentstest.TTTState, agentstest.TTTAction]{
		Simulations: 200,
		Rollout:     agents.UniformRandomRollout[agentstest.TTTState, agentstest.TTTAction](),
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < cfg.Simulations; i++ {
		tree.RunOne(env, cfg, rng)
	}
	bestI, ok := tree.RootBestLegalIdx()
	if !ok {
		t.Fatal("expected best legal idx after sims")
	}

	beforeNodes := tree.NodeCount()
	tree.AdvanceRoot(env, bestI)
	afterNodes := tree.NodeCount()

	if afterNodes >= beforeNodes {
		t.Fatalf("AdvanceRoot should shrink tree (kept only one subtree); before=%d after=%d", beforeNodes, afterNodes)
	}
	if afterNodes <= 1 {
		t.Fatalf("AdvanceRoot collapsed tree to root only — subtree was lost; got %d nodes", afterNodes)
	}
	// New root state must be the post-move state for legal[bestI] applied
	// to the empty board.
	rootCells := tree.Root().Cells
	if rootCells[bestI] != 1 {
		t.Fatalf("expected new root with X at cell %d (the played move), got cells=%v", bestI, rootCells)
	}
}

func TestRootBestLegalIdxFalseOnUnvisited(t *testing.T) {
	tree := agents.NewMCTSTree[agentstest.TTTState, agentstest.TTTAction](agentstest.TTTState{})
	if _, ok := tree.RootBestLegalIdx(); ok {
		t.Fatal("expected RootBestLegalIdx to return ok=false on a fresh tree")
	}
}

func TestRunOneGrowsTreeAcrossSims(t *testing.T) {
	env := &agentstest.TTTGame{}
	tree := agents.NewMCTSTree[agentstest.TTTState, agentstest.TTTAction](agentstest.TTTState{})
	cfg := &agents.MCTSConfig[agentstest.TTTState, agentstest.TTTAction]{
		Simulations: 50,
		Rollout:     agents.UniformRandomRollout[agentstest.TTTState, agentstest.TTTAction](),
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < cfg.Simulations; i++ {
		tree.RunOne(env, cfg, rng)
	}
	if tree.NodeCount() <= 1 {
		t.Fatalf("expected tree to grow past root over %d sims, got %d nodes", cfg.Simulations, tree.NodeCount())
	}
	if _, ok := tree.RootBestLegalIdx(); !ok {
		t.Fatal("expected RootBestLegalIdx ok=true after sims")
	}
}
