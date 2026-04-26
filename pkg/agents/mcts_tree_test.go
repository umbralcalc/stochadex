package agents_test

// Direct unit tests for the MCTSTree value type. These exercise the tree's
// methods (SelectLeaf, BackupScores, BackupVisits, AdvanceRoot,
// RootStatsByLegalIdx, RootBestLegalIdx, RunOne) independently of the
// stochadex partition machinery. Use the tic-tac-toe environment in
// tictactoe.go as a deterministic, easy-to-reason-about driver.

import (
	"math/rand/v2"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/agents"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestNewMCTSTreeStartsWithJustRoot(t *testing.T) {
	tree := agents.NewMCTSTree[agents.TTTState, agents.TTTAction](agents.TTTState{})
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
	env := &agents.TTTGame{}
	tree := agents.NewMCTSTree[agents.TTTState, agents.TTTAction](agents.TTTState{})
	cfg := &agents.MCTSConfig[agents.TTTState, agents.TTTAction]{}
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
	root := agents.TTTFromGrid([9]int8{1, 1, 1, 2, 2, 0, 0, 0, 0}, 1)
	env := &agents.TTTGame{}
	tree := agents.NewMCTSTree[agents.TTTState, agents.TTTAction](root)
	cfg := &agents.MCTSConfig[agents.TTTState, agents.TTTAction]{}
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
	env := &agents.TTTGame{}
	tree := agents.NewMCTSTree[agents.TTTState, agents.TTTAction](agents.TTTState{})
	cfg := &agents.MCTSConfig[agents.TTTState, agents.TTTAction]{MaxTreeDepth: 1}
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
	env := &agents.TTTGame{}
	tree := agents.NewMCTSTree[agents.TTTState, agents.TTTAction](agents.TTTState{})
	cfg := &agents.MCTSConfig[agents.TTTState, agents.TTTAction]{}
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
	env := &agents.TTTGame{}
	tree := agents.NewMCTSTree[agents.TTTState, agents.TTTAction](agents.TTTState{})
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
	env := &agents.TTTGame{}
	tree := agents.NewMCTSTree[agents.TTTState, agents.TTTAction](agents.TTTState{})
	cfg := &agents.MCTSConfig[agents.TTTState, agents.TTTAction]{}
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
	env := &agents.TTTGame{}
	tree := agents.NewMCTSTree[agents.TTTState, agents.TTTAction](agents.TTTState{})
	cfg := &agents.MCTSConfig[agents.TTTState, agents.TTTAction]{}
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
	env := &agents.TTTGame{}
	tree := agents.NewMCTSTree[agents.TTTState, agents.TTTAction](agents.TTTState{})
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
	env := &agents.TTTGame{}
	tree := agents.NewMCTSTree[agents.TTTState, agents.TTTAction](agents.TTTState{})
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
	env := &agents.TTTGame{}
	tree := agents.NewMCTSTree[agents.TTTState, agents.TTTAction](agents.TTTState{})
	tree.AdvanceRoot(env, 99) // out of range
	if tree.NodeCount() != 1 {
		t.Fatalf("AdvanceRoot with bad idx must be a no-op, got %d nodes", tree.NodeCount())
	}
	if tree.Root().Cells[0] != 0 {
		t.Fatalf("AdvanceRoot with bad idx must not advance state")
	}
}

func TestAdvanceRootPreservesSubtreeWhenAvailable(t *testing.T) {
	env := &agents.TTTGame{}
	tree := agents.NewMCTSTree[agents.TTTState, agents.TTTAction](agents.TTTState{})
	cfg := &agents.MCTSConfig[agents.TTTState, agents.TTTAction]{
		Simulations: 200,
		Rollout:     agents.UniformRandomRollout[agents.TTTState, agents.TTTAction](),
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
	tree := agents.NewMCTSTree[agents.TTTState, agents.TTTAction](agents.TTTState{})
	if _, ok := tree.RootBestLegalIdx(); ok {
		t.Fatal("expected RootBestLegalIdx to return ok=false on a fresh tree")
	}
}

func TestRunOneGrowsTreeAcrossSims(t *testing.T) {
	env := &agents.TTTGame{}
	tree := agents.NewMCTSTree[agents.TTTState, agents.TTTAction](agents.TTTState{})
	cfg := &agents.MCTSConfig[agents.TTTState, agents.TTTAction]{
		Simulations: 50,
		Rollout:     agents.UniformRandomRollout[agents.TTTState, agents.TTTAction](),
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

// newMCTSTreeIteration constructs a MCTSTreeIteration wired for tic-tac-toe.
// Used by both t.Run blocks and the analysis-level integration tests.
func newMCTSTreeIteration() *agents.MCTSTreeIteration[agents.TTTState, agents.TTTAction] {
	return &agents.MCTSTreeIteration[agents.TTTState, agents.TTTAction]{
		Env: &agents.TTTGame{},
		Cfg: agents.MCTSConfig[agents.TTTState, agents.TTTAction]{
			// Rollout fn left nil — MCTSTreeIteration selects + backs up only.
			// Scores are supplied by an upstream MCTSRolloutIteration.
		},
		Decoder:         agents.TTTDecode,
		Encoder:         agents.TTTEncode,
		MaxLegalActions: 9,
		StateWidth:      agents.TTTWidth,
		Players:         2,
	}
}

func TestMCTSTreeIteration(t *testing.T) {
	t.Run(
		"test that the tree partition runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./mcts_tree_settings.yaml")
			iterations := []simulator.Iteration{newMCTSTreeIteration()}
			for partitionIndex, iter := range iterations {
				iter.Configure(partitionIndex, settings)
			}
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 50,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			coordinator := simulator.NewPartitionCoordinator(settings, implementations)
			coordinator.Run()

			// Without an upstream rollout supplying scores, MCTSTreeIteration
			// uses no-signal-tolerant backups: tree must still grow because
			// every selection counts as a visit.
			rows := store.GetValues("tree")
			if len(rows) == 0 {
				t.Fatal("no rows recorded")
			}
			final := rows[len(rows)-1]
			// Best root idx must have been chosen by the end of the run.
			if final[agents.MCTSTreeRowBestRootIdx] < 0 {
				t.Fatalf("expected best_root_idx >= 0 after 50 steps, row=%v", final)
			}
		},
	)
	t.Run(
		"test that the tree partition runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./mcts_tree_settings.yaml")
			iterations := []simulator.Iteration{newMCTSTreeIteration()}
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.NilOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 30,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("test harness failed: %v", err)
			}
		},
	)
}

// TestMCTSTreeIterationRowLayout pins the row-layout offsets and width so a
// future change can't silently shift slots that downstream wiring depends
// on. Documented in TreeRow* constants and MCTSTreeRowWidth.
func TestMCTSTreeIterationRowLayout(t *testing.T) {
	const W = 10
	const K = 9
	if got, want := agents.MCTSTreeRowWidth(W, K), 1+W+1+2*K; got != want {
		t.Fatalf("MCTSTreeRowWidth(%d,%d): got %d want %d", W, K, got, want)
	}
	if agents.MCTSTreeRowBestRootIdx != 0 {
		t.Fatalf("MCTSTreeRowBestRootIdx must be 0, got %d", agents.MCTSTreeRowBestRootIdx)
	}
	if agents.MCTSTreeRowLeafStateOffset != 1 {
		t.Fatalf("MCTSTreeRowLeafStateOffset must be 1, got %d", agents.MCTSTreeRowLeafStateOffset)
	}
	if got, want := agents.MCTSTreeRowHasLeafOffset(W), 1+W; got != want {
		t.Fatalf("MCTSTreeRowHasLeafOffset(%d): got %d want %d", W, got, want)
	}
	if got, want := agents.MCTSTreeRowVisitsOffset(W), 2+W; got != want {
		t.Fatalf("MCTSTreeRowVisitsOffset(%d): got %d want %d", W, got, want)
	}
	if got, want := agents.MCTSTreeRowWinsOffset(W, K), 2+W+K; got != want {
		t.Fatalf("MCTSTreeRowWinsOffset(%d,%d): got %d want %d", W, K, got, want)
	}
}

// TestMCTSTreeIterationResetsOnRootStateChange exercises the load-bearing
// outer-piped root_state mechanism: when an upstream partition pushes a
// new game state into MCTSTreeParamRootState, the tree must rebuild from the
// new root rather than keep accumulating against the old one. Without
// this, self-play would search a stale position every ply after the first.
func TestMCTSTreeIterationResetsOnRootStateChange(t *testing.T) {
	tree := newMCTSTreeIteration()
	gen := simulator.NewConfigGenerator()

	// Provider partition emits a constant encoded TTT state in slots
	// matching agents.MCTSTreeParamRootState (length = StateWidth = 10). Updating
	// its row mid-run is the standard way to push a "new root" signal
	// downstream; we use init values that DIFFER from the tree's init root
	// (which is the empty board with X to move).
	const provName = "root_provider"
	provInit := agents.TTTEncode(agents.TTTFromGrid(
		[9]int8{1, 0, 2, 0, 0, 0, 0, 0, 0}, 0,
	))

	rootIndices := make([]int, agents.TTTWidth)
	for i := 0; i < agents.TTTWidth; i++ {
		rootIndices[i] = i
	}

	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.NilOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 5},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              provName,
		Iteration:         &general.ConstantValuesIteration{},
		InitStateValues:   provInit,
		StateHistoryDepth: 1,
		Seed:              0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "tree",
		Iteration: tree,
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			agents.MCTSTreeParamRootState: {Upstream: provName, Indices: rootIndices},
		},
		InitStateValues: func() []float64 {
			row := make([]float64, agents.MCTSTreeRowWidth(agents.TTTWidth, 9))
			// MCTSTree's init still encodes the empty board at the leaf-state slot
			// (it'll be reset on first iter once root_state arrives).
			copy(row[agents.MCTSTreeRowLeafStateOffset:], agents.TTTEncode(agents.TTTState{}))
			return row
		}(),
		StateHistoryDepth: 1,
		Seed:              0,
	})
	settings, impl := gen.GenerateConfigs()
	simulator.NewPartitionCoordinator(settings, impl).Run()

	// After running, the tree's internal root should be the provider's
	// state (a partially-filled board), NOT the empty board it was
	// configured with.
	gotRoot := tree.MCTSTree().Root()
	if gotRoot.Cells[0] != 1 || gotRoot.Cells[2] != 2 {
		t.Fatalf("tree did not reset to upstream root_state; root cells=%v", gotRoot.Cells)
	}
}

// TestMCTSTreeIterationTerminalRootEmitsHasLeafFalse verifies that when the
// tree's root is already terminal (game over), SelectLeaf returns ok=false
// and the partition emits has_leaf=0 — signalling downstream rollouts to
// skip.
func TestMCTSTreeIterationTerminalRootEmitsHasLeafFalse(t *testing.T) {
	tree := newMCTSTreeIteration()
	gen := simulator.NewConfigGenerator()
	store := simulator.NewStateTimeStorage()

	// X has already won across the top row.
	terminal := agents.TTTFromGrid([9]int8{1, 1, 1, 2, 2, 0, 0, 0, 0}, 1)
	rowInit := make([]float64, agents.MCTSTreeRowWidth(agents.TTTWidth, 9))
	copy(rowInit[agents.MCTSTreeRowLeafStateOffset:], agents.TTTEncode(terminal))

	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		OutputFunction:       &simulator.StateTimeStorageOutputFunction{Store: store},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 5},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              "tree",
		Iteration:         tree,
		InitStateValues:   rowInit,
		StateHistoryDepth: 1,
		Seed:              0,
	})
	settings, impl := gen.GenerateConfigs()
	simulator.NewPartitionCoordinator(settings, impl).Run()

	rows := store.GetValues("tree")
	hasLeafSlot := agents.MCTSTreeRowHasLeafOffset(agents.TTTWidth)
	for i, r := range rows {
		if i == 0 {
			continue // initial row, before any Iterate
		}
		if r[hasLeafSlot] != 0 {
			t.Fatalf("step %d: expected has_leaf=0 at terminal root, got %v", i, r[hasLeafSlot])
		}
	}
}
