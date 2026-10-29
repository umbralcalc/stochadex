package macros_test

// End-to-end integration tests for the MCTS analysis helper. The helper
// builds Architecture K: an outer apply partition + an outer search
// partition (EmbeddedSimulationRunIteration wrapping a MCTSTreeIteration +
// MCTSRolloutIteration pipeline). The tests drive the full stack against the
// shared tic-tac-toe fixture and assert on the OUTER apply partition's
// recorded rows — which is the user-visible game state evolving over
// time.

import (
	"fmt"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/agents"
	"github.com/umbralcalc/stochadex/pkg/macros"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

func ttSpec(initState agents.TTTState, simsPerPly int, seed uint64) macros.MCTSSelfPlaySpec[agents.TTTState, agents.TTTAction] {
	return macros.MCTSSelfPlaySpec[agents.TTTState, agents.TTTAction]{
		Name: "ttt",
		Env:  &agents.TTTGame{},
		Cfg: agents.MCTSConfig[agents.TTTState, agents.TTTAction]{
			Rollout: agents.UniformRandomRollout[agents.TTTState, agents.TTTAction](),
		},
		InitState:       initState,
		Decoder:         agents.TTTDecode,
		Encoder:         agents.TTTEncode,
		SimsPerPly:      simsPerPly,
		MaxLegalActions: 9,
		StateWidth:      agents.TTTWidth,
		Players:         2,
		Seed:            seed,
	}
}

func runOuter(t *testing.T, spec macros.MCTSSelfPlaySpec[agents.TTTState, agents.TTTAction], outerSteps int) [][]float64 {
	t.Helper()
	parts := macros.NewMCTSSelfPlayPartitions[agents.TTTState, agents.TTTAction](spec)
	gen := simulator.NewConfigGenerator()
	store := simulator.NewStateTimeStorage()
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		OutputFunction:       &simulator.StateTimeStorageOutputFunction{Store: store},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: outerSteps},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	for _, p := range parts {
		gen.SetPartition(p)
	}
	settings, impl := gen.GenerateConfigs()
	simulator.NewPartitionCoordinator(settings, impl).Run()
	return store.GetValues(spec.Name + "_apply")
}

func TestSelfPlayPartitionsPlayGameToTermination(t *testing.T) {
	rows := runOuter(t, ttSpec(agents.TTTState{}, 30, 7), 12)
	if len(rows) < 2 {
		t.Fatalf("expected multiple rows, got %d", len(rows))
	}
	last, err := agents.TTTDecode(rows[len(rows)-1])
	if err != nil {
		t.Fatalf("decode final row: %v", err)
	}
	if !last.Done {
		t.Fatalf("expected terminal final state after 12 outer steps, got %+v", last)
	}
	filled := 0
	for _, c := range last.Cells {
		if c != 0 {
			filled++
		}
	}
	if filled < 5 {
		t.Fatalf("final position has too few moves played: %d cells filled, state=%+v", filled, last)
	}
}

// TestSelfPlayPartitionsFindWinFromForcedPosition seeds the apply
// partition with a "win in one" position. The first step is the
// initialisation step where search runs but apply skips (best_idx = -1
// sentinel). The second step is where apply consumes search's first real
// best_idx and plays the winning move.
func TestSelfPlayPartitionsFindWinFromForcedPosition(t *testing.T) {
	// X has two in a row at 0, 1; cell 2 wins. X to move.
	init := agents.TTTFromGrid([9]int8{1, 1, 0, 0, 2, 0, 0, 2, 0}, 0)
	rows := runOuter(t, ttSpec(init, 200, 99), 3)
	if len(rows) < 3 {
		t.Fatalf("expected at least 3 rows, got %d", len(rows))
	}
	// Row 0 = init (recorded by EveryStepOutputCondition before any
	// Iterate). Row 1 = after step 1 (apply skipped because search hadn't
	// produced a real best_idx yet). Row 2 = after step 2 (apply played
	// the winning move).
	step2, err := agents.TTTDecode(rows[2])
	if err != nil {
		t.Fatalf("decode row 2: %v", err)
	}
	if !step2.Done || step2.Winner != 0 {
		t.Fatalf("expected X to win on step 2; got %+v", step2)
	}
}

func TestSelfPlayPartitionsRunWithHarnesses(t *testing.T) {
	parts := macros.NewMCTSSelfPlayPartitions[agents.TTTState, agents.TTTAction](
		ttSpec(agents.TTTState{}, 20, 11),
	)
	gen := simulator.NewConfigGenerator()
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.NilOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 9},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	for _, p := range parts {
		gen.SetPartition(p)
	}
	settings, impl := gen.GenerateConfigs()
	if err := simulator.RunWithHarnesses(settings, impl); err != nil {
		t.Fatalf("RunWithHarnesses: %v", err)
	}
}

func TestNewMCTSSelfPlayPartitionsPanicsOnMissingCodec(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on missing encoder/decoder")
		}
	}()
	_ = macros.NewMCTSSelfPlayPartitions[agents.TTTState, agents.TTTAction](
		macros.MCTSSelfPlaySpec[agents.TTTState, agents.TTTAction]{
			Name:            "ttt",
			Env:             &agents.TTTGame{},
			MaxLegalActions: 9,
			StateWidth:      agents.TTTWidth,
			Players:         2,
		},
	)
}

// TestMCTSSelfPlayLayout checks the layout against the row format stated by
// hand: a tree row is [best_root_idx, leaf state (10), has_leaf, visits (9),
// wins (9)], a rollout row one score per player and a flag, and the search row
// the two concatenated.
func TestMCTSSelfPlayLayout(t *testing.T) {
	root := agents.TTTState{Current: 1}
	root.Cells[4] = 1
	encoded := []float64{0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	tree := append(append([]float64{0}, encoded...), make([]float64, 19)...)
	search := append(append(append([]float64{-1}, encoded...), make([]float64, 19)...), 0, 0, 0)

	t.Run("an unset search budget takes 50 simulations per ply", func(t *testing.T) {
		layout := macros.NewMCTSSelfPlayLayout(ttSpec(root, 0, 6))
		if layout.SimsPerPly != 50 {
			t.Errorf("SimsPerPly = %d, want 50", layout.SimsPerPly)
		}
		for name, pair := range map[string][2][]float64{
			"ApplyInit":   {layout.ApplyInit, encoded},
			"TreeInit":    {layout.TreeInit, tree},
			"RolloutInit": {layout.RolloutInit, {0, 0, 0}},
			"SearchInit":  {layout.SearchInit, search},
		} {
			if !floats.Equal(pair[0], pair[1]) {
				t.Errorf("%s = %v, want %v", name, pair[0], pair[1])
			}
		}
		if got := fmt.Sprint(layout.LeafIndices); got != "[1 2 3 4 5 6 7 8 9 10 11]" {
			t.Errorf("LeafIndices = %s", got)
		}
		if got := fmt.Sprint(layout.RootStateIndices); got != "[0 1 2 3 4 5 6 7 8 9]" {
			t.Errorf("RootStateIndices = %s", got)
		}
		if layout.BestIdxSlot != 0 || layout.Seed != 6 || layout.RolloutSeed != 7 {
			t.Errorf("BestIdxSlot, Seed, RolloutSeed = %d, %d, %d; want 0, 6, 7",
				layout.BestIdxSlot, layout.Seed, layout.RolloutSeed)
		}
	})

	t.Run("a set search budget stands", func(t *testing.T) {
		if got := macros.NewMCTSSelfPlayLayout(ttSpec(root, 12, 6)).SimsPerPly; got != 12 {
			t.Errorf("SimsPerPly = %d, want 12", got)
		}
	})
}
