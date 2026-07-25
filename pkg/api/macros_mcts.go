package api

import (
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/macros"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// mcts_self_play is a LIVE macro, like evolution_strategy_optimisation: its
// partitions form a self-contained self-play loop run as a fresh simulation
// rather than an analysis against pre-recorded storage.
//
// It is also the one macro whose central component is not resolved from the
// data registries. Its `env:` spec goes to the environment registry instead
// (registry_environment.go), which explains why the engine can run this macro
// over rules it has never heard of.
type mctsSelfPlaySpec struct {
	macroTypeField `yaml:",inline"`
	// Env names a registered environment and carries that environment's own
	// fields, which this package passes through without interpreting.
	Env simulator.ComponentSpec `yaml:"env"`
	// Name prefixes the generated partitions: "<name>_apply" holds the encoded
	// state after each ply and is normally what you read output from;
	// "<name>_search" holds the embedded search.
	Name string `yaml:"name"`
	// Steps is the number of outer plies to run.
	Steps int `yaml:"steps"`
	// SimsPerPly and the UCT knobs below are all optional: an unset value keeps
	// whatever default the registered environment chose.
	SimsPerPly      int     `yaml:"sims_per_ply,omitempty"`
	Simulations     int     `yaml:"simulations,omitempty"`
	Exploration     float64 `yaml:"exploration,omitempty"`
	MaxTreeDepth    int     `yaml:"max_tree_depth,omitempty"`
	RolloutMaxSteps int     `yaml:"rollout_max_steps,omitempty"`
	Seed            uint64  `yaml:"seed,omitempty"`
	Timestep        float64 `yaml:"timestep,omitempty"`
}

// resolve reports that this is a live macro (it does not run against storage).
func (s *mctsSelfPlaySpec) resolve(
	*simulator.StateTimeStorage,
) ([]*simulator.PartitionConfig, map[string]int, error) {
	return nil, nil, fmt.Errorf(
		"mcts_self_play is a live macro; it runs its own simulation and must " +
			"not be combined with against-storage macros")
}

func (s *mctsSelfPlaySpec) resolveLive(
	*simulator.StateTimeStorage,
) ([]*simulator.PartitionConfig, int, float64, error) {
	if s.Name == "" {
		return nil, 0, 0, fmt.Errorf("mcts_self_play needs a name: (it prefixes the partition names)")
	}
	if s.Steps <= 0 {
		return nil, 0, 0, fmt.Errorf("mcts_self_play needs steps: > 0 (the number of plies to play)")
	}
	if s.Env.IsZero() {
		return nil, 0, 0, fmt.Errorf(
			"mcts_self_play needs an env: {type: ...} naming a registered environment")
	}
	partitions, err := ResolveEnvironment(s.Env, macros.MCTSSearchSettings{
		Name:            s.Name,
		SimsPerPly:      s.SimsPerPly,
		Simulations:     s.Simulations,
		Exploration:     s.Exploration,
		MaxTreeDepth:    s.MaxTreeDepth,
		RolloutMaxSteps: s.RolloutMaxSteps,
		Seed:            s.Seed,
	})
	if err != nil {
		return nil, 0, 0, err
	}
	timestep := s.Timestep
	if timestep == 0 {
		timestep = 1.0
	}
	return partitions, s.Steps, timestep, nil
}
