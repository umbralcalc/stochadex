package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/agents"
)

// macroConfigError is runMacroConfig's counterpart for the configs that must
// fail: it returns the expansion error instead of failing the test on it.
func macroConfigError(t *testing.T, yamlText string) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yamlText), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := runMacros(LoadApiRunConfigFromYaml(path))
	return err
}

// TestMCTSSelfPlayMacro drives the whole hook end-to-end through YAML: the macro
// tier resolves an environment from the registry, builds the self-play stack,
// and runs it — no Go toolchain, no main.partitions. The assertion is the sharp
// one: seeded one move from a win, the search has to find it. A macro that wired
// up but searched nothing would still produce rows, so "it ran" is not the test.
//
// The position matters. X's win is at cell 8, which is LAST in legal order
// (empty cells are 1, 2, 4, 5, 8), so a stack that returns a near-arbitrary
// index cannot stumble onto it — an earlier draft of this test used a position
// whose winning cell happened to come first in legal order and passed at one
// simulation per ply, i.e. it asserted nothing about the search.
func TestMCTSSelfPlayMacro(t *testing.T) {
	// X at cells 6 and 7; playing cell 8 completes the bottom row. X to move.
	const cfg = `macros:
- type: mcts_self_play
  name: ttt
  steps: 3
  sims_per_ply: 200
  seed: 99
  env:
    type: tictactoe
    init_grid: [2, 0, 0, 2, 0, 0, 1, 1, 0]
    current_player: 0
`
	out := runMacroConfig(t, cfg)
	rows := out["ttt_apply"]
	if len(rows) < 3 {
		t.Fatalf("expected at least 3 recorded plies, got %d", len(rows))
	}
	// Row 0 is the seeded position; row 1 is the step where search has not yet
	// produced a real best index (the -1 sentinel), so apply holds. Row 2 is the
	// first ply the search actually chose.
	played, err := agents.TTTDecode(rows[2])
	if err != nil {
		t.Fatalf("decode ply 2: %v", err)
	}
	if !played.Done || played.Winner != 0 {
		t.Fatalf("expected the search to play the winning move; got %+v", played)
	}
}

// TestMCTSSelfPlayMacroValidation checks the macro fails loudly and specifically
// on the mistakes a config author actually makes, rather than deadlocking or
// producing an empty run.
func TestMCTSSelfPlayMacroValidation(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		cfg        string
		wantErrHas string
	}{
		{
			name:       "missing name",
			cfg:        "macros:\n- type: mcts_self_play\n  steps: 2\n  env: {type: tictactoe}\n",
			wantErrHas: "needs a name",
		},
		{
			name:       "missing steps",
			cfg:        "macros:\n- type: mcts_self_play\n  name: ttt\n  env: {type: tictactoe}\n",
			wantErrHas: "steps",
		},
		{
			name:       "unregistered environment",
			cfg:        "macros:\n- type: mcts_self_play\n  name: ttt\n  steps: 2\n  env: {type: chess}\n",
			wantErrHas: "unknown type \"chess\"",
		},
		{
			name: "unknown environment field",
			cfg: "macros:\n- type: mcts_self_play\n  name: ttt\n  steps: 2\n" +
				"  env: {type: tictactoe, board: [1, 2]}\n",
			wantErrHas: "unknown field \"board\"",
		},
		{
			name: "out-of-range cell value",
			cfg: "macros:\n- type: mcts_self_play\n  name: ttt\n  steps: 2\n" +
				"  env: {type: tictactoe, init_grid: [9, 0, 0, 0, 0, 0, 0, 0, 0]}\n",
			wantErrHas: "init_grid[0]",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := macroConfigError(t, testCase.cfg)
			if err == nil {
				t.Fatalf("expected an error mentioning %q", testCase.wantErrHas)
			}
			if !strings.Contains(err.Error(), testCase.wantErrHas) {
				t.Errorf("error should mention %q, got: %v", testCase.wantErrHas, err)
			}
		})
	}
}

// TestMCTSSelfPlayMacroRejectsStorageCombination pins the live/against-storage
// split: mcts_self_play produces its own run, so pairing it with a data: block
// and an analysis macro is a config error rather than a silent misread.
func TestMCTSSelfPlayMacroRejectsStorageCombination(t *testing.T) {
	spec := &mctsSelfPlaySpec{}
	if _, _, err := spec.resolve(nil); err == nil {
		t.Fatal("expected the against-storage path to be rejected")
	}
}
