package api

import (
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/agents"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Tic-tac-toe is the only environment the engine registers, and a fixture rather
// than a domain model: it keeps the hook covered end to end in CI and gives cfg/
// a worked example. Real decision rules belong downstream.
func init() {
	RegisterEnvironment("tictactoe", buildTicTacToeEnvironment)
}

// buildTicTacToeEnvironment is the reference EnvironmentBuilder. It shows the
// intended shape for a downstream one: parse your own fields out of the spec,
// and hand back NewMCTSEnvironment of the typed environment.
//
// Fields (all optional):
//
//	init_grid:      9 cell values, 0 empty / 1 X / 2 O (default: empty board)
//	current_player: 0 for X to move, 1 for O (default: 0)
func buildTicTacToeEnvironment(spec simulator.ComponentSpec) (MCTSEnvironment, error) {
	initState := agents.TTTState{}
	var grid [9]int8
	currentPlayer := 0
	haveGrid := false

	for key, value := range spec.Fields {
		switch key {
		case "init_grid":
			raw, ok := value.([]interface{})
			if !ok || len(raw) != 9 {
				return nil, fmt.Errorf("init_grid must be a list of 9 cell values, got %v", value)
			}
			for i, element := range raw {
				cell, ok := element.(int)
				if !ok || cell < 0 || cell > 2 {
					return nil, fmt.Errorf(
						"init_grid[%d] must be 0 (empty), 1 (X) or 2 (O), got %v", i, element)
				}
				grid[i] = int8(cell)
			}
			haveGrid = true
		case "current_player":
			player, ok := value.(int)
			if !ok || (player != 0 && player != 1) {
				return nil, fmt.Errorf("current_player must be 0 or 1, got %v", value)
			}
			currentPlayer = player
		default:
			return nil, fmt.Errorf("unknown field %q", key)
		}
	}
	if haveGrid || currentPlayer != 0 {
		initState = agents.TTTFromGrid(grid, currentPlayer)
	}

	return NewMCTSEnvironment(MCTSEnvironmentSpec[agents.TTTState, agents.TTTAction]{
		Env:             &agents.TTTGame{},
		InitState:       initState,
		Decoder:         agents.TTTDecode,
		Encoder:         agents.TTTEncode,
		MaxLegalActions: 9,
		StateWidth:      agents.TTTWidth,
		Players:         2,
		// A per-environment default the config may override.
		SimsPerPly: 50,
	})
}
