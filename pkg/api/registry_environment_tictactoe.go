package api

import (
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/agents"
	"github.com/umbralcalc/stochadex/pkg/macros"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// The one environment the engine registers itself. Tic-tac-toe is already this
// repo's canonical Environment fixture (see agents.TTTGame): small enough to be
// obvious, sharp enough that a broken search fails a "win in one" assertion.
// Registering it here is what makes the mcts_self_play macro runnable from the
// stock CLI with no downstream module — which is what keeps the whole hook
// covered by an end-to-end config in engine CI, and gives cfg/ a worked example.
//
// It is a fixture, not a domain model: it is deliberately the only environment
// the engine ships, and real decision rules belong downstream (see the registry
// note in registry_environment.go).
func init() {
	RegisterEnvironment("tictactoe", buildTicTacToeEnvironment)
}

// buildTicTacToeEnvironment is the reference EnvironmentBuilder. It shows the
// intended shape for a downstream one: parse your own fields out of the spec,
// fill the typed half of the self-play spec, apply the config-stated search
// settings over the top, and hand back the partitions.
//
// Fields (all optional):
//
//	init_grid:      9 cell values, 0 empty / 1 X / 2 O (default: empty board)
//	current_player: 0 for X to move, 1 for O (default: 0)
func buildTicTacToeEnvironment(
	spec simulator.ComponentSpec,
	settings macros.MCTSSearchSettings,
) ([]*simulator.PartitionConfig, error) {
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

	selfPlay := macros.MCTSSelfPlaySpec[agents.TTTState, agents.TTTAction]{
		Env: &agents.TTTGame{},
		Cfg: agents.MCTSConfig[agents.TTTState, agents.TTTAction]{
			Rollout: agents.UniformRandomRollout[agents.TTTState, agents.TTTAction](),
		},
		InitState: initState,
		Decoder:   agents.TTTDecode,
		Encoder:   agents.TTTEncode,
		// The environment's own shape — never config's to state, because a
		// mismatch here is a silently misshapen row rather than a loud error.
		MaxLegalActions: 9,
		StateWidth:      agents.TTTWidth,
		Players:         2,
		// A per-environment default the config may override.
		SimsPerPly: 50,
	}
	macros.ApplyMCTSSearchSettings(&selfPlay, settings)

	return macros.NewMCTSSelfPlayPartitions(selfPlay), nil
}
