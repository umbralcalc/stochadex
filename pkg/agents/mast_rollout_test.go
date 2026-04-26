package agents_test

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/agents"
	"github.com/umbralcalc/stochadex/pkg/general"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func newMASTRolloutImpls() []simulator.Iteration {
	return []simulator.Iteration{
		&general.ConstantValuesIteration{}, // leaf_provider
		&general.ConstantValuesIteration{}, // agg_provider
		&agents.MASTRolloutIteration[agents.TTTState, agents.TTTAction]{
			Env: &agents.TTTGame{},
			Cfg: agents.MCTSConfig[agents.TTTState, agents.TTTAction]{
				RolloutMaxSteps: 30,
			},
			Decoder:  agents.TTTDecode,
			KeyToIdx: tttKeyToIdx,
			MaxKeys:  9,
			MaxPath:  9,
			Players:  2,
		},
	}
}

// tttKeyToIdx maps a tic-tac-toe action to its cell index. Cell index is
// already a bounded int in [0, 9), making it the natural action key.
func tttKeyToIdx(a agents.TTTAction) int { return int(a) }

func TestMASTRolloutIteration(t *testing.T) {
	t.Run(
		"test that the MAST rollout partition runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./mast_rollout_settings.yaml")
			iterations := newMASTRolloutImpls()
			for partitionIndex, iter := range iterations {
				iter.Configure(partitionIndex, settings)
			}
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 20,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			coordinator := simulator.NewPartitionCoordinator(settings, implementations)
			coordinator.Run()

			rows := store.GetValues("rollout")
			if len(rows) == 0 {
				t.Fatal("no rollout rows recorded")
			}
			validCount := 0
			for _, r := range rows {
				if r[agents.MASTRolloutOkOffset(2)] == 1 {
					validCount++
					sum := r[0] + r[1]
					if sum < 0.99 || sum > 1.01 {
						t.Fatalf("score vector with ok=1 must sum to ~1, got %v", r[:3])
					}
					if r[agents.MASTRolloutNumPathOffset(2)] == 0 {
						t.Fatalf("expected non-zero num_path with ok=1, got %v", r)
					}
				}
			}
			if validCount == 0 {
				t.Fatal("no rollout produced ok=1")
			}
		},
	)
	t.Run(
		"test that the MAST rollout partition runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./mast_rollout_settings.yaml")
			iterations := newMASTRolloutImpls()
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.NilOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 12,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("test harness failed: %v", err)
			}
		},
	)
}

func TestMASTRolloutRowLayout(t *testing.T) {
	if got, want := agents.MASTRolloutRowWidth(2, 5), 2+2+10; got != want {
		t.Fatalf("MASTRolloutRowWidth(2,5): got %d want %d", got, want)
	}
	if agents.MASTRolloutScoresOffset(0) != 0 {
		t.Fatalf("scores offset 0: got %d", agents.MASTRolloutScoresOffset(0))
	}
	if agents.MASTRolloutOkOffset(2) != 2 {
		t.Fatalf("ok offset for P=2: got %d want 2", agents.MASTRolloutOkOffset(2))
	}
	if agents.MASTRolloutNumPathOffset(2) != 3 {
		t.Fatalf("num_path offset for P=2: got %d want 3", agents.MASTRolloutNumPathOffset(2))
	}
	if agents.MASTRolloutPathOffset(2) != 4 {
		t.Fatalf("path offset for P=2: got %d want 4", agents.MASTRolloutPathOffset(2))
	}
}

// TestMASTRolloutEmitsZerosWhenHasLeafFalse verifies the upstream
// has_leaf=0 sentinel produces an all-zero scores+ok+path row.
func TestMASTRolloutEmitsZerosWhenHasLeafFalse(t *testing.T) {
	emptyLeaf := agents.TTTEncode(agents.TTTState{})
	leafSlice := append(append([]float64{}, emptyLeaf...), 0) // has_leaf=0

	roll := &agents.MASTRolloutIteration[agents.TTTState, agents.TTTAction]{
		Env:      &agents.TTTGame{},
		Cfg:      agents.MCTSConfig[agents.TTTState, agents.TTTAction]{RolloutMaxSteps: 30},
		Decoder:  agents.TTTDecode,
		KeyToIdx: tttKeyToIdx,
		MaxKeys:  9,
		MaxPath:  9,
		Players:  2,
	}
	leafIndices := make([]int, len(leafSlice))
	for i := range leafIndices {
		leafIndices[i] = i
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
		Name:              "leaf",
		Iteration:         &general.ConstantValuesIteration{},
		InitStateValues:   leafSlice,
		StateHistoryDepth: 1,
		Seed:              0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "rollout",
		Iteration: roll,
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			agents.MCTSRolloutParamLeaf: {Upstream: "leaf", Indices: leafIndices},
		},
		InitStateValues:   make([]float64, agents.MASTRolloutRowWidth(2, 9)),
		StateHistoryDepth: 1,
		Seed:              0,
	})
	settings, impl := gen.GenerateConfigs()
	simulator.NewPartitionCoordinator(settings, impl).Run()

	rows := store.GetValues("rollout")
	for i, r := range rows {
		for j, v := range r {
			if v != 0 {
				t.Fatalf("row %d slot %d: expected zero with has_leaf=false, got %v", i, j, v)
			}
		}
	}
}

// TestMASTRolloutBiasesByAggregates wires a non-uniform aggregates row
// that strongly favours a single key (cell 4 = the centre). With low tau
// and many rollouts, the emitted paths should bias toward visiting cell
// 4 early.
func TestMASTRolloutBiasesByAggregates(t *testing.T) {
	emptyLeaf := agents.TTTEncode(agents.TTTState{})
	leafSlice := append(append([]float64{}, emptyLeaf...), 1)

	aggRow := make([]float64, agents.MASTAggregationRowWidth(9))
	for k := 0; k < 9; k++ {
		aggRow[agents.MASTAggregationCountSlot(k)] = 100
		if k == 4 {
			aggRow[agents.MASTAggregationSumSlot(k)] = 100
		}
	}

	roll := &agents.MASTRolloutIteration[agents.TTTState, agents.TTTAction]{
		Env:      &agents.TTTGame{},
		Cfg:      agents.MCTSConfig[agents.TTTState, agents.TTTAction]{RolloutMaxSteps: 30},
		Decoder:  agents.TTTDecode,
		KeyToIdx: tttKeyToIdx,
		MaxKeys:  9,
		MaxPath:  9,
		Players:  2,
		Tau:      0.1,
	}

	gen := simulator.NewConfigGenerator()
	store := simulator.NewStateTimeStorage()
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		OutputFunction:       &simulator.StateTimeStorageOutputFunction{Store: store},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 50},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	leafIdx := make([]int, len(leafSlice))
	for i := range leafIdx {
		leafIdx[i] = i
	}
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              "leaf",
		Iteration:         &general.ConstantValuesIteration{},
		InitStateValues:   leafSlice,
		StateHistoryDepth: 1,
		Seed:              0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              "agg",
		Iteration:         &general.ConstantValuesIteration{},
		InitStateValues:   aggRow,
		StateHistoryDepth: 1,
		Seed:              0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "rollout",
		Iteration: roll,
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			agents.MCTSRolloutParamLeaf: {Upstream: "leaf", Indices: leafIdx},
		},
		ParamsAsPartitions: map[string][]string{
			agents.MASTAggregationParamPartition: {"agg"},
		},
		InitStateValues:   make([]float64, agents.MASTRolloutRowWidth(2, 9)),
		StateHistoryDepth: 1,
		Seed:              42,
	})
	settings, impl := gen.GenerateConfigs()
	simulator.NewPartitionCoordinator(settings, impl).Run()

	rows := store.GetValues("rollout")
	pathOff := agents.MASTRolloutPathOffset(2)
	fourFirst := 0
	totalValid := 0
	for _, r := range rows {
		if r[agents.MASTRolloutOkOffset(2)] != 1 {
			continue
		}
		totalValid++
		if int(r[pathOff]) == 4 {
			fourFirst++
		}
	}
	if totalValid == 0 {
		t.Fatal("no valid rollouts produced")
	}
	ratio := float64(fourFirst) / float64(totalValid)
	if ratio < 0.5 {
		t.Fatalf("expected MAST bias toward cell 4 (>50%% of first picks); got %d/%d = %.2f", fourFirst, totalValid, ratio)
	}
}
