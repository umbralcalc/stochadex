package agents_test

// Direct unit tests for the rollout adapter functions in rollout.go:
// UniformRandomRollout, FromProgress, WinnerToTerminal. These are the
// composable building blocks used by callers wiring custom rollout
// strategies; their edge-case behaviour is contractual.

import (
	"errors"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/agents"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestUniformRandomRolloutTerminatesWithEnvScores(t *testing.T) {
	// X about to win on a "2 in a row, 1 free" position. From the next
	// player's seat (X), there are 7 legal moves; with 50 max steps a
	// uniform rollout reliably reaches a terminal state.
	root := agents.TTTFromGrid([9]int8{1, 1, 0, 2, 2, 0, 0, 0, 0}, 0)
	env := &agents.TTTGame{}
	roll := agents.UniformRandomRollout[agents.TTTState, agents.TTTAction]()
	scores, ok, err := roll(env, root, 50, 1234)
	if err != nil {
		t.Fatalf("rollout error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true on a position that always reaches terminal within 50 steps")
	}
	if len(scores) != 2 {
		t.Fatalf("expected 2-player scores, got len=%d", len(scores))
	}
	sum := scores[0] + scores[1]
	if sum < 0.99 || sum > 1.01 {
		t.Fatalf("expected scores summing to 1 (winner-takes-all or draw), got %v", scores)
	}
}

func TestUniformRandomRolloutTruncatesWithOkFalse(t *testing.T) {
	// Empty board, max steps 0 → can't take any action, returns ok=false.
	env := &agents.TTTGame{}
	roll := agents.UniformRandomRollout[agents.TTTState, agents.TTTAction]()
	scores, ok, err := roll(env, agents.TTTState{}, 0, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false when truncated, got scores=%v", scores)
	}
}

func TestUniformRandomRolloutDeterministicWithSameSeed(t *testing.T) {
	env := &agents.TTTGame{}
	roll := agents.UniformRandomRollout[agents.TTTState, agents.TTTAction]()
	a, _, _ := roll(env, agents.TTTState{}, 50, 42)
	b, _, _ := roll(env, agents.TTTState{}, 50, 42)
	if len(a) != len(b) {
		t.Fatalf("score slice lengths differ across same-seed runs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("rollout not deterministic with same seed: %v vs %v", a, b)
		}
	}
}

func TestWinnerToTerminalOneHotForBinaryWin(t *testing.T) {
	scores := agents.WinnerToTerminal(0, 2, true)
	if len(scores) != 2 || scores[0] != 1 || scores[1] != 0 {
		t.Fatalf("expected one-hot for player 0 winner, got %v", scores)
	}
	scores = agents.WinnerToTerminal(1, 2, true)
	if len(scores) != 2 || scores[0] != 0 || scores[1] != 1 {
		t.Fatalf("expected one-hot for player 1 winner, got %v", scores)
	}
}

func TestWinnerToTerminalReturnsNilWhenNotDone(t *testing.T) {
	if scores := agents.WinnerToTerminal(0, 2, false); scores != nil {
		t.Fatalf("WinnerToTerminal must return nil when done=false, got %v", scores)
	}
}

func TestWinnerToTerminalIgnoresOutOfRangeWinner(t *testing.T) {
	// Negative winner = draw (no slot credited).
	scores := agents.WinnerToTerminal(-1, 2, true)
	if len(scores) != 2 || scores[0] != 0 || scores[1] != 0 {
		t.Fatalf("expected all-zero scores for negative winner, got %v", scores)
	}
	// winner >= players: out-of-range, all zeros.
	scores = agents.WinnerToTerminal(5, 2, true)
	if len(scores) != 2 || scores[0] != 0 || scores[1] != 0 {
		t.Fatalf("expected all-zero scores for out-of-range winner, got %v", scores)
	}
}

func TestFromProgressUsesInnerWhenInnerSucceeds(t *testing.T) {
	// Inner rollout that always returns ok=true with [0.7, 0.3].
	inner := func(env agents.Environment[agents.TTTState, agents.TTTAction], s agents.TTTState, max int, seed uint64) ([]float64, bool, error) {
		return []float64{0.7, 0.3}, true, nil
	}
	progress := func(s agents.TTTState, p int) (float64, bool) {
		t.Fatal("progress should not be called when inner succeeds")
		return 0, false
	}
	wrapped := agents.FromProgress[agents.TTTState, agents.TTTAction](inner, progress)
	env := &agents.TTTGame{}
	scores, ok, err := wrapped(env, agents.TTTState{}, 10, 1)
	if err != nil || !ok {
		t.Fatalf("expected ok=true with inner scores; got ok=%v err=%v", ok, err)
	}
	if scores[0] != 0.7 || scores[1] != 0.3 {
		t.Fatalf("expected inner scores passed through, got %v", scores)
	}
}

func TestFromProgressFallsBackToProgressOnTruncation(t *testing.T) {
	// Inner always returns ok=false (truncated, no signal).
	inner := func(env agents.Environment[agents.TTTState, agents.TTTAction], s agents.TTTState, max int, seed uint64) ([]float64, bool, error) {
		return nil, false, nil
	}
	// Progress returns asymmetric values so the result has signal.
	progress := func(s agents.TTTState, p int) (float64, bool) {
		if p == 0 {
			return 0.6, true
		}
		return 0.4, true
	}
	wrapped := agents.FromProgress[agents.TTTState, agents.TTTAction](inner, progress)
	env := &agents.TTTGame{}
	// Use maxSteps=0 so the wrapper's replay loop also terminates without
	// consuming actions; progress is then computed on the supplied state.
	scores, ok, err := wrapped(env, agents.TTTState{}, 0, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected progress fallback to produce ok=true, got scores=%v", scores)
	}
	if scores[0] != 0.6 || scores[1] != 0.4 {
		t.Fatalf("expected progress scores, got %v", scores)
	}
}

func TestFromProgressTreatsAllZeroProgressAsNoSignal(t *testing.T) {
	inner := func(env agents.Environment[agents.TTTState, agents.TTTAction], s agents.TTTState, max int, seed uint64) ([]float64, bool, error) {
		return nil, false, nil
	}
	progress := func(s agents.TTTState, p int) (float64, bool) {
		return 0, true // all-equal-zero — no comparative signal
	}
	wrapped := agents.FromProgress[agents.TTTState, agents.TTTAction](inner, progress)
	env := &agents.TTTGame{}
	_, ok, err := wrapped(env, agents.TTTState{}, 0, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("all-zero progress carries no comparative signal — expected ok=false")
	}
}

func TestFromProgressReturnsOkFalseWhenProgressUnavailable(t *testing.T) {
	inner := func(env agents.Environment[agents.TTTState, agents.TTTAction], s agents.TTTState, max int, seed uint64) ([]float64, bool, error) {
		return nil, false, nil
	}
	progress := func(s agents.TTTState, p int) (float64, bool) {
		return 0, false // no progress available for any player
	}
	wrapped := agents.FromProgress[agents.TTTState, agents.TTTAction](inner, progress)
	env := &agents.TTTGame{}
	_, ok, err := wrapped(env, agents.TTTState{}, 0, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when progress reports unavailable for all players")
	}
}

func TestFromProgressPropagatesInnerError(t *testing.T) {
	innerErr := errors.New("rollout boom")
	inner := func(env agents.Environment[agents.TTTState, agents.TTTAction], s agents.TTTState, max int, seed uint64) ([]float64, bool, error) {
		return nil, false, innerErr
	}
	wrapped := agents.FromProgress[agents.TTTState, agents.TTTAction](inner, nil)
	env := &agents.TTTGame{}
	_, _, err := wrapped(env, agents.TTTState{}, 0, 1)
	if !errors.Is(err, innerErr) {
		t.Fatalf("expected wrapped error to propagate inner error, got %v", err)
	}
}

// newMCTSRolloutIterationImpls builds the [leaf_provider, rollout] iteration
// pair for the tic-tac-toe-driven test.
func newMCTSRolloutIterationImpls() []simulator.Iteration {
	return []simulator.Iteration{
		&general.ConstantValuesIteration{},
		&agents.MCTSRolloutIteration[agents.TTTState, agents.TTTAction]{
			Env: &agents.TTTGame{},
			Cfg: agents.MCTSConfig[agents.TTTState, agents.TTTAction]{
				Rollout: agents.UniformRandomRollout[agents.TTTState, agents.TTTAction](),
			},
			Decoder: agents.TTTDecode,
			Players: 2,
		},
	}
}

func TestMCTSRolloutIteration(t *testing.T) {
	t.Run(
		"test that the rollout partition runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./mcts_rollout_settings.yaml")
			iterations := newMCTSRolloutIterationImpls()
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
			settings := simulator.LoadSettingsFromYaml("./mcts_rollout_settings.yaml")
			iterations := newMCTSRolloutIterationImpls()
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
func runRolloutWithLeaf(t *testing.T, leafSlice []float64, rollout *agents.MCTSRolloutIteration[agents.TTTState, agents.TTTAction], steps int) [][]float64 {
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

// TestMCTSRolloutIterationEmitsZerosWhenHasLeafFalse pins the contract that
// the upstream-supplied has_leaf=0 sentinel produces an all-zero
// scores+ok row, signalling no signal to the downstream tree partition.
func TestMCTSRolloutIterationEmitsZerosWhenHasLeafFalse(t *testing.T) {
	emptyLeaf := agents.TTTEncode(agents.TTTState{})
	leafSlice := append(append([]float64{}, emptyLeaf...), 0) // has_leaf = 0
	roll := &agents.MCTSRolloutIteration[agents.TTTState, agents.TTTAction]{
		Env: &agents.TTTGame{},
		Cfg: agents.MCTSConfig[agents.TTTState, agents.TTTAction]{
			Rollout: agents.UniformRandomRollout[agents.TTTState, agents.TTTAction](),
		},
		Decoder: agents.TTTDecode,
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

// TestMCTSRolloutIterationEmitsZerosWhenCfgRolloutNil verifies the safety
// path: a misconfigured Cfg with no Rollout function emits zeros rather
// than panicking.
func TestMCTSRolloutIterationEmitsZerosWhenCfgRolloutNil(t *testing.T) {
	emptyLeaf := agents.TTTEncode(agents.TTTState{})
	leafSlice := append(append([]float64{}, emptyLeaf...), 1) // has_leaf = 1
	roll := &agents.MCTSRolloutIteration[agents.TTTState, agents.TTTAction]{
		Env:     &agents.TTTGame{},
		Cfg:     agents.MCTSConfig[agents.TTTState, agents.TTTAction]{}, // no Rollout
		Decoder: agents.TTTDecode,
		Players: 2,
	}
	rows := runRolloutWithLeaf(t, leafSlice, roll, 3)
	for _, r := range rows {
		if r[2] != 0 {
			t.Fatalf("expected ok=0 when Cfg.Rollout is nil, got %v", r)
		}
	}
}

// TestMCTSRolloutIterationRejectsScoresWithWrongLength verifies the safety
// path: a rollout function returning a score vector of the wrong length
// (Players=2 but rollout returns 3 floats) is rejected with ok=0 rather
// than emitting a malformed row.
func TestMCTSRolloutIterationRejectsScoresWithWrongLength(t *testing.T) {
	bogus := func(env agents.Environment[agents.TTTState, agents.TTTAction], s agents.TTTState, max int, seed uint64) ([]float64, bool, error) {
		return []float64{0.3, 0.3, 0.4}, true, nil // 3 players for a 2-player env
	}
	emptyLeaf := agents.TTTEncode(agents.TTTState{})
	leafSlice := append(append([]float64{}, emptyLeaf...), 1)
	roll := &agents.MCTSRolloutIteration[agents.TTTState, agents.TTTAction]{
		Env:     &agents.TTTGame{},
		Cfg:     agents.MCTSConfig[agents.TTTState, agents.TTTAction]{Rollout: bogus},
		Decoder: agents.TTTDecode,
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

// TestMCTSRolloutIterationPropagatesOkOnValidScores is the positive
// counterpart: when the rollout returns valid scores summing to 1, the
// row faithfully reflects them with ok=1.
func TestMCTSRolloutIterationPropagatesOkOnValidScores(t *testing.T) {
	canned := func(env agents.Environment[agents.TTTState, agents.TTTAction], s agents.TTTState, max int, seed uint64) ([]float64, bool, error) {
		return []float64{0.6, 0.4}, true, nil
	}
	emptyLeaf := agents.TTTEncode(agents.TTTState{})
	leafSlice := append(append([]float64{}, emptyLeaf...), 1)
	roll := &agents.MCTSRolloutIteration[agents.TTTState, agents.TTTAction]{
		Env:     &agents.TTTGame{},
		Cfg:     agents.MCTSConfig[agents.TTTState, agents.TTTAction]{Rollout: canned},
		Decoder: agents.TTTDecode,
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
