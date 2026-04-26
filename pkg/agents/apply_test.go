package agents_test

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/agents"
	"github.com/umbralcalc/stochadex/pkg/general"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func newApplyIterationImpls() []simulator.Iteration {
	return []simulator.Iteration{
		&general.ConstantValuesIteration{},
		&agents.ApplyIteration[agents.TTTState, agents.TTTAction]{
			Env:     &agents.TTTGame{},
			Decoder: agents.TTTDecode,
			Encoder: agents.TTTEncode,
		},
	}
}

func TestApplyIteration(t *testing.T) {
	t.Run(
		"test that the apply partition runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./apply_settings.yaml")
			iterations := newApplyIterationImpls()
			for partitionIndex, iter := range iterations {
				iter.Configure(partitionIndex, settings)
			}
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 9,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			coordinator := simulator.NewPartitionCoordinator(settings, implementations)
			coordinator.Run()

			rows := store.GetValues("apply")
			if len(rows) < 2 {
				t.Fatalf("expected multiple rows, got %d", len(rows))
			}
			// After the first Iterate (rows[1]) at least one cell must be
			// filled (the upstream always emits legal-action idx 0, so the
			// game will progress).
			filled := 0
			for i := 0; i < 9; i++ {
				if rows[1][i] != 0 {
					filled++
				}
			}
			if filled == 0 {
				t.Fatalf("apply partition produced no move at step 1; row=%v", rows[1])
			}
			// By the end of the run the game should have terminated (legal
			// idx 0 plays whichever cell is leftmost-empty each ply, so 9
			// steps is enough to fill the board or end early on a win).
			last, err := agents.TTTDecode(rows[len(rows)-1])
			if err != nil {
				t.Fatalf("decode final row: %v", err)
			}
			if !last.Done {
				t.Fatalf("expected terminal final state, got %+v", last)
			}
		},
	)
	t.Run(
		"test that the apply partition runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./apply_settings.yaml")
			iterations := newApplyIterationImpls()
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.NilOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 9,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("test harness failed: %v", err)
			}
		},
	)
}

// TestApplyIterationStateHistoryMode wires the apply partition to read its
// best-action signal from an upstream partition's state-history row at a
// configurable slot offset, instead of via params_from_upstream. This is
// the wiring used by NewMCTSSelfPlayPartitions to break the apply ↔ search
// dependency cycle. The test verifies the lag-1 read picks up the upstream
// value correctly.
func TestApplyIterationStateHistoryMode(t *testing.T) {
	// Upstream partition row layout: [unused, best_legal_idx]. Slot 1 is
	// what apply will read (BestIdxSlot=1). Constant value = 4 (centre
	// cell on an empty TTT board).
	const upstreamWidth = 2
	const slot = 1
	upstreamInit := []float64{0, 4}

	apply := &agents.ApplyIteration[agents.TTTState, agents.TTTAction]{
		Env:         &agents.TTTGame{},
		Decoder:     agents.TTTDecode,
		Encoder:     agents.TTTEncode,
		BestIdxSlot: slot,
	}

	gen := simulator.NewConfigGenerator()
	store := simulator.NewStateTimeStorage()
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		OutputFunction:       &simulator.StateTimeStorageOutputFunction{Store: store},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 5},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              "upstream",
		Iteration:         &general.ConstantValuesIteration{},
		InitStateValues:   upstreamInit,
		StateHistoryDepth: 1,
		Seed:              0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "apply",
		Iteration: apply,
		ParamsAsPartitions: map[string][]string{
			agents.ApplyParamBestIdxPartition: {"upstream"},
		},
		InitStateValues:   agents.TTTEncode(agents.TTTState{}),
		StateHistoryDepth: 1,
		Seed:              0,
	})
	_ = upstreamWidth
	settings, impl := gen.GenerateConfigs()
	simulator.NewPartitionCoordinator(settings, impl).Run()

	rows := store.GetValues("apply")
	if len(rows) < 2 {
		t.Fatalf("expected multiple rows, got %d", len(rows))
	}
	// At step 1 (first Iterate) apply reads upstream's row 0 = init = best=4.
	// X plays cell 4. Expect cells[4] = 1.
	step1, err := agents.TTTDecode(rows[1])
	if err != nil {
		t.Fatalf("decode row 1: %v", err)
	}
	if step1.Cells[4] != 1 {
		t.Fatalf("expected cell 4 played by X via state-history mode, got cells=%v", step1.Cells)
	}
}

// TestApplyIterationDoesNotMoveAtTerminal verifies that apply detects an
// already-terminal current state (game over) and skips the move even when
// the upstream signals a valid index.
func TestApplyIterationDoesNotMoveAtTerminal(t *testing.T) {
	// Seed apply with a position where X has already won.
	terminalInit := agents.TTTEncode(agents.TTTFromGrid(
		[9]int8{1, 1, 1, 2, 2, 0, 0, 0, 0}, 1,
	))
	apply := &agents.ApplyIteration[agents.TTTState, agents.TTTAction]{
		Env:     &agents.TTTGame{},
		Decoder: agents.TTTDecode,
		Encoder: agents.TTTEncode,
	}
	gen := simulator.NewConfigGenerator()
	store := simulator.NewStateTimeStorage()
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		OutputFunction:       &simulator.StateTimeStorageOutputFunction{Store: store},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 3},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              "idx",
		Iteration:         &general.ConstantValuesIteration{},
		InitStateValues:   []float64{0}, // claim "play legal[0]"
		StateHistoryDepth: 1,
		Seed:              0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "apply",
		Iteration: apply,
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			agents.ApplyParamBestIdx: {Upstream: "idx", Indices: []int{0}},
		},
		InitStateValues:   terminalInit,
		StateHistoryDepth: 1,
		Seed:              0,
	})
	settings, impl := gen.GenerateConfigs()
	simulator.NewPartitionCoordinator(settings, impl).Run()

	rows := store.GetValues("apply")
	for i, r := range rows {
		got, err := agents.TTTDecode(r)
		if err != nil {
			t.Fatalf("decode row %d: %v", i, err)
		}
		if !got.Done {
			t.Fatalf("row %d: expected terminal state to be preserved, got %+v", i, got)
		}
	}
}

// TestApplyIterationIgnoresOutOfRangeIdx verifies that an upstream
// best_legal_idx outside the legal-action range is treated as a no-op
// rather than panicking or producing a bogus move.
func TestApplyIterationIgnoresOutOfRangeIdx(t *testing.T) {
	apply := &agents.ApplyIteration[agents.TTTState, agents.TTTAction]{
		Env:     &agents.TTTGame{},
		Decoder: agents.TTTDecode,
		Encoder: agents.TTTEncode,
	}
	gen := simulator.NewConfigGenerator()
	store := simulator.NewStateTimeStorage()
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		OutputFunction:       &simulator.StateTimeStorageOutputFunction{Store: store},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 3},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              "idx",
		Iteration:         &general.ConstantValuesIteration{},
		InitStateValues:   []float64{99}, // way out of range (only 9 legal moves on empty board)
		StateHistoryDepth: 1,
		Seed:              0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "apply",
		Iteration: apply,
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			agents.ApplyParamBestIdx: {Upstream: "idx", Indices: []int{0}},
		},
		InitStateValues:   agents.TTTEncode(agents.TTTState{}),
		StateHistoryDepth: 1,
		Seed:              0,
	})
	settings, impl := gen.GenerateConfigs()
	simulator.NewPartitionCoordinator(settings, impl).Run()

	rows := store.GetValues("apply")
	for i, r := range rows {
		got, _ := agents.TTTDecode(r)
		filled := 0
		for _, c := range got.Cells {
			if c != 0 {
				filled++
			}
		}
		if filled != 0 {
			t.Fatalf("row %d: out-of-range idx should leave board untouched, got %d cells filled", i, filled)
		}
	}
}

// TestApplyIterationSkipsOnNegativeIdx verifies that the -1 sentinel
// (meaning "search hasn't produced a real best yet") is treated as
// no-op.
func TestApplyIterationSkipsOnNegativeIdx(t *testing.T) {
	apply := &agents.ApplyIteration[agents.TTTState, agents.TTTAction]{
		Env:     &agents.TTTGame{},
		Decoder: agents.TTTDecode,
		Encoder: agents.TTTEncode,
	}
	gen := simulator.NewConfigGenerator()
	store := simulator.NewStateTimeStorage()
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		OutputFunction:       &simulator.StateTimeStorageOutputFunction{Store: store},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 3},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              "idx",
		Iteration:         &general.ConstantValuesIteration{},
		InitStateValues:   []float64{-1},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "apply",
		Iteration: apply,
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			agents.ApplyParamBestIdx: {Upstream: "idx", Indices: []int{0}},
		},
		InitStateValues:   agents.TTTEncode(agents.TTTState{}),
		StateHistoryDepth: 1,
		Seed:              0,
	})
	settings, impl := gen.GenerateConfigs()
	simulator.NewPartitionCoordinator(settings, impl).Run()

	rows := store.GetValues("apply")
	last, _ := agents.TTTDecode(rows[len(rows)-1])
	for i, c := range last.Cells {
		if c != 0 {
			t.Fatalf("expected board untouched with idx=-1; cell[%d]=%d", i, c)
		}
	}
}
