package agents_test

// Tests for the one-shot RunMCTSSearch helper. RunMCTSSearch is the
// non-partition-driven entry point: builds a MCTSTree, runs N simulations
// from a fixed root, returns the most-visited legal action plus per-edge
// stats. Useful for "what's the best move from this state?" queries
// outside any stochadex coordinator.

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/agents"
)

func TestRunMCTSSearchPicksWinningMove(t *testing.T) {
	// X at 0, 1; cell 2 wins for X. X to move.
	root := agents.TTTFromGrid([9]int8{1, 1, 0, 0, 2, 0, 0, 2, 0}, 0)
	env := &agents.TTTGame{}
	cfg := agents.MCTSConfig[agents.TTTState, agents.TTTAction]{Simulations: 200}
	best, stats, err := agents.RunMCTSSearch[agents.TTTState, agents.TTTAction](env, root, cfg, 42, 200)
	if err != nil {
		t.Fatalf("RunMCTSSearch: %v", err)
	}
	if best != agents.TTTAction(2) {
		t.Fatalf("expected winning move 2, got %d (stats=%v)", best, stats)
	}
}

func TestRunMCTSSearchBlocksOpponent(t *testing.T) {
	// X at 0, 1; if O does not block at 2, X wins. O to move.
	root := agents.TTTFromGrid([9]int8{1, 1, 0, 2, 0, 0, 0, 0, 0}, 1)
	env := &agents.TTTGame{}
	cfg := agents.MCTSConfig[agents.TTTState, agents.TTTAction]{Simulations: 400}
	best, stats, err := agents.RunMCTSSearch[agents.TTTState, agents.TTTAction](env, root, cfg, 7, 400)
	if err != nil {
		t.Fatalf("RunMCTSSearch: %v", err)
	}
	if best != agents.TTTAction(2) {
		t.Fatalf("expected blocking move 2, got %d (stats=%v)", best, stats)
	}
}

func TestRunMCTSSearchRejectsEmptyEnv(t *testing.T) {
	cfg := agents.MCTSConfig[agents.TTTState, agents.TTTAction]{}
	_, _, err := agents.RunMCTSSearch[agents.TTTState, agents.TTTAction](nil, agents.TTTState{}, cfg, 0, 1)
	if err == nil {
		t.Fatal("expected RunMCTSSearch to reject nil env")
	}
}

func TestRunMCTSSearchRejectsTerminalRoot(t *testing.T) {
	// Position with no legal moves (X already won).
	root := agents.TTTFromGrid([9]int8{1, 1, 1, 2, 2, 0, 0, 0, 0}, 1)
	env := &agents.TTTGame{}
	cfg := agents.MCTSConfig[agents.TTTState, agents.TTTAction]{}
	_, _, err := agents.RunMCTSSearch[agents.TTTState, agents.TTTAction](env, root, cfg, 0, 1)
	if err == nil {
		t.Fatal("expected RunMCTSSearch to reject terminal root")
	}
}

func TestRunMCTSSearchMCTSEdgeStatsReportPerLegalCounts(t *testing.T) {
	// X needs to win at cell 2. With 200 sims the winning child must
	// dominate visits. RunMCTSSearch returns per-legal stats — the winning
	// move's stats must show the most visits and a positive mean.
	root := agents.TTTFromGrid([9]int8{1, 1, 0, 0, 2, 0, 0, 2, 0}, 0)
	env := &agents.TTTGame{}
	cfg := agents.MCTSConfig[agents.TTTState, agents.TTTAction]{Simulations: 200}
	_, stats, err := agents.RunMCTSSearch[agents.TTTState, agents.TTTAction](env, root, cfg, 42, 200)
	if err != nil {
		t.Fatalf("RunMCTSSearch: %v", err)
	}
	if len(stats) == 0 {
		t.Fatal("RunMCTSSearch returned empty stats slice")
	}
	totalVisits := 0
	bestVisits := 0
	bestAction := agents.TTTAction(-1)
	for _, s := range stats {
		totalVisits += s.Visits
		if s.Visits > bestVisits {
			bestVisits = s.Visits
			bestAction = s.Action
		}
	}
	if totalVisits == 0 {
		t.Fatal("expected nonzero total visits across edge stats")
	}
	if bestAction != agents.TTTAction(2) {
		t.Fatalf("expected most-visited action to be the winning move 2, got %d", bestAction)
	}
}

func TestRunMCTSSearchUsesUniformRolloutByDefault(t *testing.T) {
	// Cfg.Rollout left nil; RunMCTSSearch must default to uniform random
	// rollouts rather than refusing to run.
	root := agents.TTTState{}
	env := &agents.TTTGame{}
	cfg := agents.MCTSConfig[agents.TTTState, agents.TTTAction]{} // no Rollout
	_, stats, err := agents.RunMCTSSearch[agents.TTTState, agents.TTTAction](env, root, cfg, 1, 50)
	if err != nil {
		t.Fatalf("RunMCTSSearch should work with default rollout: %v", err)
	}
	if len(stats) == 0 {
		t.Fatal("expected at least one expanded edge after sims with default rollout")
	}
}
