package agents_test

// Direct unit tests for the rollout adapter functions in rollout.go:
// UniformRandomRollout, FromProgress, WinnerToTerminal. These are the
// composable building blocks used by callers wiring custom rollout
// strategies; their edge-case behaviour is contractual.

import (
	"errors"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/agents"
	"github.com/umbralcalc/stochadex/pkg/agents/agentstest"
)

func TestUniformRandomRolloutTerminatesWithEnvScores(t *testing.T) {
	// X about to win on a "2 in a row, 1 free" position. From the next
	// player's seat (X), there are 7 legal moves; with 50 max steps a
	// uniform rollout reliably reaches a terminal state.
	root := agentstest.TTTFromGrid([9]int8{1, 1, 0, 2, 2, 0, 0, 0, 0}, 0)
	env := &agentstest.TTTGame{}
	roll := agents.UniformRandomRollout[agentstest.TTTState, agentstest.TTTAction]()
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
	env := &agentstest.TTTGame{}
	roll := agents.UniformRandomRollout[agentstest.TTTState, agentstest.TTTAction]()
	scores, ok, err := roll(env, agentstest.TTTState{}, 0, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false when truncated, got scores=%v", scores)
	}
}

func TestUniformRandomRolloutDeterministicWithSameSeed(t *testing.T) {
	env := &agentstest.TTTGame{}
	roll := agents.UniformRandomRollout[agentstest.TTTState, agentstest.TTTAction]()
	a, _, _ := roll(env, agentstest.TTTState{}, 50, 42)
	b, _, _ := roll(env, agentstest.TTTState{}, 50, 42)
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
	inner := func(env agents.Environment[agentstest.TTTState, agentstest.TTTAction], s agentstest.TTTState, max int, seed uint64) ([]float64, bool, error) {
		return []float64{0.7, 0.3}, true, nil
	}
	progress := func(s agentstest.TTTState, p int) (float64, bool) {
		t.Fatal("progress should not be called when inner succeeds")
		return 0, false
	}
	wrapped := agents.FromProgress[agentstest.TTTState, agentstest.TTTAction](inner, progress)
	env := &agentstest.TTTGame{}
	scores, ok, err := wrapped(env, agentstest.TTTState{}, 10, 1)
	if err != nil || !ok {
		t.Fatalf("expected ok=true with inner scores; got ok=%v err=%v", ok, err)
	}
	if scores[0] != 0.7 || scores[1] != 0.3 {
		t.Fatalf("expected inner scores passed through, got %v", scores)
	}
}

func TestFromProgressFallsBackToProgressOnTruncation(t *testing.T) {
	// Inner always returns ok=false (truncated, no signal).
	inner := func(env agents.Environment[agentstest.TTTState, agentstest.TTTAction], s agentstest.TTTState, max int, seed uint64) ([]float64, bool, error) {
		return nil, false, nil
	}
	// Progress returns asymmetric values so the result has signal.
	progress := func(s agentstest.TTTState, p int) (float64, bool) {
		if p == 0 {
			return 0.6, true
		}
		return 0.4, true
	}
	wrapped := agents.FromProgress[agentstest.TTTState, agentstest.TTTAction](inner, progress)
	env := &agentstest.TTTGame{}
	// Use maxSteps=0 so the wrapper's replay loop also terminates without
	// consuming actions; progress is then computed on the supplied state.
	scores, ok, err := wrapped(env, agentstest.TTTState{}, 0, 1)
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
	inner := func(env agents.Environment[agentstest.TTTState, agentstest.TTTAction], s agentstest.TTTState, max int, seed uint64) ([]float64, bool, error) {
		return nil, false, nil
	}
	progress := func(s agentstest.TTTState, p int) (float64, bool) {
		return 0, true // all-equal-zero — no comparative signal
	}
	wrapped := agents.FromProgress[agentstest.TTTState, agentstest.TTTAction](inner, progress)
	env := &agentstest.TTTGame{}
	_, ok, err := wrapped(env, agentstest.TTTState{}, 0, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("all-zero progress carries no comparative signal — expected ok=false")
	}
}

func TestFromProgressReturnsOkFalseWhenProgressUnavailable(t *testing.T) {
	inner := func(env agents.Environment[agentstest.TTTState, agentstest.TTTAction], s agentstest.TTTState, max int, seed uint64) ([]float64, bool, error) {
		return nil, false, nil
	}
	progress := func(s agentstest.TTTState, p int) (float64, bool) {
		return 0, false // no progress available for any player
	}
	wrapped := agents.FromProgress[agentstest.TTTState, agentstest.TTTAction](inner, progress)
	env := &agentstest.TTTGame{}
	_, ok, err := wrapped(env, agentstest.TTTState{}, 0, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when progress reports unavailable for all players")
	}
}

func TestFromProgressPropagatesInnerError(t *testing.T) {
	innerErr := errors.New("rollout boom")
	inner := func(env agents.Environment[agentstest.TTTState, agentstest.TTTAction], s agentstest.TTTState, max int, seed uint64) ([]float64, bool, error) {
		return nil, false, innerErr
	}
	wrapped := agents.FromProgress[agentstest.TTTState, agentstest.TTTAction](inner, nil)
	env := &agentstest.TTTGame{}
	_, _, err := wrapped(env, agentstest.TTTState{}, 0, 1)
	if !errors.Is(err, innerErr) {
		t.Fatalf("expected wrapped error to propagate inner error, got %v", err)
	}
}
