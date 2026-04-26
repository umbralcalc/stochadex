package agents_test

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/agents"
	"github.com/umbralcalc/stochadex/pkg/agents/agentstest"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// newMCTSRolloutPartitionImpls builds the [leaf_provider, rollout] iteration
// pair for the tic-tac-toe-driven test.
func newMCTSRolloutPartitionImpls() []simulator.Iteration {
	return []simulator.Iteration{
		&general.ConstantValuesIteration{},
		&agents.MCTSRolloutPartition[agentstest.TTTState, agentstest.TTTAction]{
			Env: &agentstest.TTTGame{},
			Cfg: agents.MCTSConfig[agentstest.TTTState, agentstest.TTTAction]{
				Rollout: agents.UniformRandomRollout[agentstest.TTTState, agentstest.TTTAction](),
			},
			Decoder: agentstest.TTTDecode,
			Players: 2,
		},
	}
}

func TestMCTSRolloutPartition(t *testing.T) {
	t.Run(
		"test that the rollout partition runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./mcts_rollout_partition_settings.yaml")
			iterations := newMCTSRolloutPartitionImpls()
			for partitionIndex, iter := range iterations {
				iter.Configure(partitionIndex, settings)
			}
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 30,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			coordinator := simulator.NewPartitionCoordinator(settings, implementations)
			coordinator.Run()

			rows := store.GetValues("rollout")
			if len(rows) == 0 {
				t.Fatal("no rollout rows recorded")
			}
			// At least one rollout in the run must have produced ok=1 with
			// a per-player score vector summing to ~1 (one player wins, draw
			// splits 0.5 / 0.5; either way scores[0] + scores[1] is 1.0).
			validCount := 0
			for _, r := range rows {
				if r[2] == 1 {
					validCount++
					sum := r[0] + r[1]
					if sum < 0.99 || sum > 1.01 {
						t.Fatalf("score vector with ok=1 must sum to ~1, got %v", r)
					}
				}
			}
			if validCount == 0 {
				t.Fatal("no rollout produced ok=1 — rollout partition didn't run")
			}
		},
	)
	t.Run(
		"test that the rollout partition runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./mcts_rollout_partition_settings.yaml")
			iterations := newMCTSRolloutPartitionImpls()
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.NilOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 20,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("test harness failed: %v", err)
			}
		},
	)
}

// runRolloutWithLeaf wires a constant leaf-provider partition emitting
// the given [encoded_leaf, has_leaf] slice into an upstream of a rollout
// partition, runs steps, and returns the rollout partition's recorded rows.
// Helper for the various rollout-edge-case tests below.
func runRolloutWithLeaf(t *testing.T, leafSlice []float64, rollout *agents.MCTSRolloutPartition[agentstest.TTTState, agentstest.TTTAction], steps int) [][]float64 {
	t.Helper()
	width := len(leafSlice)
	indices := make([]int, width)
	for i := 0; i < width; i++ {
		indices[i] = i
	}
	gen := simulator.NewConfigGenerator()
	store := simulator.NewStateTimeStorage()
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		OutputFunction:       &simulator.StateTimeStorageOutputFunction{Store: store},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: steps},
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
		Iteration: rollout,
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			agents.MCTSRolloutParamLeaf: {Upstream: "leaf", Indices: indices},
		},
		InitStateValues:   make([]float64, agents.MCTSRolloutRowWidth(rollout.Players)),
		StateHistoryDepth: 1,
		Seed:              0,
	})
	settings, impl := gen.GenerateConfigs()
	simulator.NewPartitionCoordinator(settings, impl).Run()
	return store.GetValues("rollout")
}

// TestMCTSRolloutPartitionEmitsZerosWhenHasLeafFalse pins the contract that
// the upstream-supplied has_leaf=0 sentinel produces an all-zero
// scores+ok row, signalling no signal to the downstream tree partition.
func TestMCTSRolloutPartitionEmitsZerosWhenHasLeafFalse(t *testing.T) {
	emptyLeaf := agentstest.TTTEncode(agentstest.TTTState{})
	leafSlice := append(append([]float64{}, emptyLeaf...), 0) // has_leaf = 0
	roll := &agents.MCTSRolloutPartition[agentstest.TTTState, agentstest.TTTAction]{
		Env: &agentstest.TTTGame{},
		Cfg: agents.MCTSConfig[agentstest.TTTState, agentstest.TTTAction]{
			Rollout: agents.UniformRandomRollout[agentstest.TTTState, agentstest.TTTAction](),
		},
		Decoder: agentstest.TTTDecode,
		Players: 2,
	}
	rows := runRolloutWithLeaf(t, leafSlice, roll, 5)
	for i, r := range rows {
		// Every row must be all-zero (has_leaf=0 → ok=0 path).
		for j, v := range r {
			if v != 0 {
				t.Fatalf("row %d slot %d: expected zero with has_leaf=false, got %v", i, j, v)
			}
		}
	}
}

// TestMCTSRolloutPartitionEmitsZerosWhenCfgRolloutNil verifies the safety
// path: a misconfigured Cfg with no Rollout function emits zeros rather
// than panicking.
func TestMCTSRolloutPartitionEmitsZerosWhenCfgRolloutNil(t *testing.T) {
	emptyLeaf := agentstest.TTTEncode(agentstest.TTTState{})
	leafSlice := append(append([]float64{}, emptyLeaf...), 1) // has_leaf = 1
	roll := &agents.MCTSRolloutPartition[agentstest.TTTState, agentstest.TTTAction]{
		Env:     &agentstest.TTTGame{},
		Cfg:     agents.MCTSConfig[agentstest.TTTState, agentstest.TTTAction]{}, // no Rollout
		Decoder: agentstest.TTTDecode,
		Players: 2,
	}
	rows := runRolloutWithLeaf(t, leafSlice, roll, 3)
	for _, r := range rows {
		if r[2] != 0 {
			t.Fatalf("expected ok=0 when Cfg.Rollout is nil, got %v", r)
		}
	}
}

// TestMCTSRolloutPartitionRejectsScoresWithWrongLength verifies the safety
// path: a rollout function returning a score vector of the wrong length
// (Players=2 but rollout returns 3 floats) is rejected with ok=0 rather
// than emitting a malformed row.
func TestMCTSRolloutPartitionRejectsScoresWithWrongLength(t *testing.T) {
	bogus := func(env agents.Environment[agentstest.TTTState, agentstest.TTTAction], s agentstest.TTTState, max int, seed uint64) ([]float64, bool, error) {
		return []float64{0.3, 0.3, 0.4}, true, nil // 3 players for a 2-player env
	}
	emptyLeaf := agentstest.TTTEncode(agentstest.TTTState{})
	leafSlice := append(append([]float64{}, emptyLeaf...), 1)
	roll := &agents.MCTSRolloutPartition[agentstest.TTTState, agentstest.TTTAction]{
		Env:     &agentstest.TTTGame{},
		Cfg:     agents.MCTSConfig[agentstest.TTTState, agentstest.TTTAction]{Rollout: bogus},
		Decoder: agentstest.TTTDecode,
		Players: 2,
	}
	rows := runRolloutWithLeaf(t, leafSlice, roll, 3)
	for i, r := range rows {
		if len(r) != 3 {
			t.Fatalf("row %d width: got %d want 3", i, len(r))
		}
		if r[2] != 0 {
			t.Fatalf("row %d: expected ok=0 on misconfigured rollout, got %v", i, r)
		}
	}
}

// TestMCTSRolloutPartitionPropagatesOkOnValidScores is the positive
// counterpart: when the rollout returns valid scores summing to 1, the
// row faithfully reflects them with ok=1.
func TestMCTSRolloutPartitionPropagatesOkOnValidScores(t *testing.T) {
	canned := func(env agents.Environment[agentstest.TTTState, agentstest.TTTAction], s agentstest.TTTState, max int, seed uint64) ([]float64, bool, error) {
		return []float64{0.6, 0.4}, true, nil
	}
	emptyLeaf := agentstest.TTTEncode(agentstest.TTTState{})
	leafSlice := append(append([]float64{}, emptyLeaf...), 1)
	roll := &agents.MCTSRolloutPartition[agentstest.TTTState, agentstest.TTTAction]{
		Env:     &agentstest.TTTGame{},
		Cfg:     agents.MCTSConfig[agentstest.TTTState, agentstest.TTTAction]{Rollout: canned},
		Decoder: agentstest.TTTDecode,
		Players: 2,
	}
	rows := runRolloutWithLeaf(t, leafSlice, roll, 3)
	// Row 0 is the init (zeros). Subsequent rows must reflect the canned
	// scores.
	for i := 1; i < len(rows); i++ {
		if rows[i][0] != 0.6 || rows[i][1] != 0.4 || rows[i][2] != 1 {
			t.Fatalf("row %d: expected [0.6 0.4 1], got %v", i, rows[i])
		}
	}
}
