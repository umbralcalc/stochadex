package agents_test

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/agents"
	"github.com/umbralcalc/stochadex/pkg/agents/agentstest"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// newMCTSTreeIterationIteration constructs a MCTSTreeIteration wired for tic-tac-toe.
// Used by both t.Run blocks and the analysis-level integration tests.
func newMCTSTreeIterationIteration() *agents.MCTSTreeIteration[agentstest.TTTState, agentstest.TTTAction] {
	return &agents.MCTSTreeIteration[agentstest.TTTState, agentstest.TTTAction]{
		Env: &agentstest.TTTGame{},
		Cfg: agents.MCTSConfig[agentstest.TTTState, agentstest.TTTAction]{
			// Rollout fn left nil — MCTSTreeIteration selects + backs up only.
			// Scores are supplied by an upstream MCTSRolloutIteration.
		},
		Decoder:         agentstest.TTTDecode,
		Encoder:         agentstest.TTTEncode,
		MaxLegalActions: 9,
		StateWidth:      agentstest.TTTWidth,
		Players:         2,
	}
}

func TestMCTSTreeIteration(t *testing.T) {
	t.Run(
		"test that the tree partition runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./mcts_tree_iteration_settings.yaml")
			iterations := []simulator.Iteration{newMCTSTreeIterationIteration()}
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
			settings := simulator.LoadSettingsFromYaml("./mcts_tree_iteration_settings.yaml")
			iterations := []simulator.Iteration{newMCTSTreeIterationIteration()}
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
	tree := newMCTSTreeIterationIteration()
	gen := simulator.NewConfigGenerator()

	// Provider partition emits a constant encoded TTT state in slots
	// matching agents.MCTSTreeParamRootState (length = StateWidth = 10). Updating
	// its row mid-run is the standard way to push a "new root" signal
	// downstream; we use init values that DIFFER from the tree's init root
	// (which is the empty board with X to move).
	const provName = "root_provider"
	provInit := agentstest.TTTEncode(agentstest.TTTFromGrid(
		[9]int8{1, 0, 2, 0, 0, 0, 0, 0, 0}, 0,
	))

	rootIndices := make([]int, agentstest.TTTWidth)
	for i := 0; i < agentstest.TTTWidth; i++ {
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
			row := make([]float64, agents.MCTSTreeRowWidth(agentstest.TTTWidth, 9))
			// MCTSTree's init still encodes the empty board at the leaf-state slot
			// (it'll be reset on first iter once root_state arrives).
			copy(row[agents.MCTSTreeRowLeafStateOffset:], agentstest.TTTEncode(agentstest.TTTState{}))
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
	tree := newMCTSTreeIterationIteration()
	gen := simulator.NewConfigGenerator()
	store := simulator.NewStateTimeStorage()

	// X has already won across the top row.
	terminal := agentstest.TTTFromGrid([9]int8{1, 1, 1, 2, 2, 0, 0, 0, 0}, 1)
	rowInit := make([]float64, agents.MCTSTreeRowWidth(agentstest.TTTWidth, 9))
	copy(rowInit[agents.MCTSTreeRowLeafStateOffset:], agentstest.TTTEncode(terminal))

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
	hasLeafSlot := agents.MCTSTreeRowHasLeafOffset(agentstest.TTTWidth)
	for i, r := range rows {
		if i == 0 {
			continue // initial row, before any Iterate
		}
		if r[hasLeafSlot] != 0 {
			t.Fatalf("step %d: expected has_leaf=0 at terminal root, got %v", i, r[hasLeafSlot])
		}
	}
}
