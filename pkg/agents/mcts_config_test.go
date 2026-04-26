package agents

// Internal whitebox tests for MCTSConfig.applyDefaults. Lives in package mcts
// rather than mcts_test because applyDefaults is unexported (it's an
// implementation detail of MCTSTree.RunOne / MCTSTree.SelectLeaf).

import (
	"testing"
)

func TestConfigApplyDefaultsFillsZeroFields(t *testing.T) {
	cfg := MCTSConfig[int, int]{}
	cfg.applyDefaults()
	if cfg.Simulations != MCTSDefaultSimulations {
		t.Errorf("Simulations: got %d want %d", cfg.Simulations, MCTSDefaultSimulations)
	}
	if cfg.RolloutMaxSteps != MCTSDefaultRolloutMaxSteps {
		t.Errorf("RolloutMaxSteps: got %d want %d", cfg.RolloutMaxSteps, MCTSDefaultRolloutMaxSteps)
	}
	if cfg.MaxTreeDepth != MCTSDefaultMaxTreeDepth {
		t.Errorf("MaxTreeDepth: got %d want %d", cfg.MaxTreeDepth, MCTSDefaultMaxTreeDepth)
	}
	if cfg.Exploration != MCTSDefaultExploration {
		t.Errorf("Exploration: got %v want %v", cfg.Exploration, MCTSDefaultExploration)
	}
}

func TestConfigApplyDefaultsPreservesNonZeroFields(t *testing.T) {
	cfg := MCTSConfig[int, int]{
		Simulations:     7,
		RolloutMaxSteps: 11,
		MaxTreeDepth:    13,
		Exploration:     0.5,
	}
	cfg.applyDefaults()
	if cfg.Simulations != 7 {
		t.Errorf("Simulations was overwritten: got %d", cfg.Simulations)
	}
	if cfg.RolloutMaxSteps != 11 {
		t.Errorf("RolloutMaxSteps was overwritten: got %d", cfg.RolloutMaxSteps)
	}
	if cfg.MaxTreeDepth != 13 {
		t.Errorf("MaxTreeDepth was overwritten: got %d", cfg.MaxTreeDepth)
	}
	if cfg.Exploration != 0.5 {
		t.Errorf("Exploration was overwritten: got %v", cfg.Exploration)
	}
}

func TestConfigApplyDefaultsIdempotent(t *testing.T) {
	cfg := MCTSConfig[int, int]{}
	cfg.applyDefaults()
	firstSims := cfg.Simulations
	firstDepth := cfg.MaxTreeDepth
	firstSteps := cfg.RolloutMaxSteps
	firstExploration := cfg.Exploration
	cfg.applyDefaults()
	cfg.applyDefaults()
	if cfg.Simulations != firstSims ||
		cfg.MaxTreeDepth != firstDepth ||
		cfg.RolloutMaxSteps != firstSteps ||
		cfg.Exploration != firstExploration {
		t.Errorf("applyDefaults must be idempotent across the numeric fields")
	}
}

func TestConfigApplyDefaultsTreatsNegativeAsZero(t *testing.T) {
	cfg := MCTSConfig[int, int]{
		Simulations:     -5,
		RolloutMaxSteps: -10,
		MaxTreeDepth:    -1,
		Exploration:     -0.1,
	}
	cfg.applyDefaults()
	if cfg.Simulations != MCTSDefaultSimulations {
		t.Errorf("negative Simulations should be replaced with default")
	}
	if cfg.RolloutMaxSteps != MCTSDefaultRolloutMaxSteps {
		t.Errorf("negative RolloutMaxSteps should be replaced with default")
	}
	if cfg.MaxTreeDepth != MCTSDefaultMaxTreeDepth {
		t.Errorf("negative MaxTreeDepth should be replaced with default")
	}
	if cfg.Exploration != MCTSDefaultExploration {
		t.Errorf("negative Exploration should be replaced with default")
	}
}
