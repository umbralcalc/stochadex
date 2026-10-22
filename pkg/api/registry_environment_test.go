package api

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/agents"
	"github.com/umbralcalc/stochadex/pkg/macros"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// tttEnvironment is a registered environment for these tests: tic-tac-toe,
// with a self-play default of 11 simulations per ply.
func tttEnvironment() (MCTSEnvironment, error) {
	return NewMCTSEnvironment(MCTSEnvironmentSpec[agents.TTTState, agents.TTTAction]{
		Env: &agents.TTTGame{}, Decoder: agents.TTTDecode, Encoder: agents.TTTEncode,
		MaxLegalActions: 9, StateWidth: agents.TTTWidth, Players: 2, SimsPerPly: 11,
	})
}

// TestRegisterEnvironment covers the downstream-environment hook: a module
// layered above pkg/api contributes an agents.Environment spelling without the
// engine importing it, or knowing anything about the rules it encodes.
func TestRegisterEnvironment(t *testing.T) {
	var gotSpec simulator.ComponentSpec
	RegisterEnvironment("test_environment", func(spec simulator.ComponentSpec) (MCTSEnvironment, error) {
		gotSpec = spec
		return tttEnvironment()
	})

	t.Run("a registered environment is built from its whole spec", func(t *testing.T) {
		environment, err := ResolveEnvironment(simulator.ComponentSpec{
			Type:   "test_environment",
			Fields: map[string]interface{}{"rules": "uno.yaml", "players": 3},
		})
		if err != nil {
			t.Fatalf("ResolveEnvironment: %v", err)
		}
		// The engine must hand the environment's own fields through untouched —
		// that is the whole point: it never parses a downstream's rules format.
		if gotSpec.Type != "test_environment" ||
			gotSpec.Fields["rules"] != "uno.yaml" || gotSpec.Fields["players"] != 3 {
			t.Errorf("spec not passed through verbatim: %+v", gotSpec)
		}
		partitions := environment.selfPlay(macros.MCTSSearchSettings{Name: "game"})
		if len(partitions) != 2 || partitions[0].Name != "game_apply" {
			t.Fatalf("expected the config-named apply + search partitions, got %d", len(partitions))
		}
	})

	t.Run("an unknown environment names the registered ones", func(t *testing.T) {
		_, err := ResolveEnvironment(simulator.ComponentSpec{Type: "no_such_environment"})
		// The likely cause of this error is a binary that did not link the
		// registering module, so the message has to show what it *did* link.
		if err == nil || !strings.Contains(err.Error(), "no_such_environment") ||
			!strings.Contains(err.Error(), "tictactoe") {
			t.Errorf("error should name the unknown type and the registered ones, got: %v", err)
		}
	})

	t.Run("a builder error is wrapped with the environment name", func(t *testing.T) {
		RegisterEnvironment("failing_environment", func(simulator.ComponentSpec) (MCTSEnvironment, error) {
			return nil, fmt.Errorf("bad ruleset")
		})
		_, err := ResolveEnvironment(simulator.ComponentSpec{Type: "failing_environment"})
		if err == nil || !strings.Contains(err.Error(), `environment "failing_environment": bad ruleset`) {
			t.Errorf("expected the builder error wrapped with its name, got: %v", err)
		}
	})

	t.Run("a builder returning no environment is an error, not a silent no-op", func(t *testing.T) {
		RegisterEnvironment("empty_environment", func(simulator.ComponentSpec) (MCTSEnvironment, error) {
			return nil, nil
		})
		_, err := ResolveEnvironment(simulator.ComponentSpec{Type: "empty_environment"})
		if err == nil || !strings.Contains(err.Error(), "builder returned no environment") {
			t.Errorf("expected a no-environment error, got: %v", err)
		}
	})

	t.Run("registering a taken name, or the engine's simulation, panics", func(t *testing.T) {
		for _, name := range []string{"test_environment", "simulation"} {
			func() {
				defer func() {
					if recover() == nil {
						t.Errorf("registering %q should panic", name)
					}
				}()
				RegisterEnvironment(name, func(simulator.ComponentSpec) (MCTSEnvironment, error) {
					return tttEnvironment()
				})
			}()
		}
	})
}

func init() {
	RegisterEnvironment("eleven_sims_environment", func(simulator.ComponentSpec) (MCTSEnvironment, error) {
		return tttEnvironment()
	})
}

// countedBuilds counts how often counted_environment is built.
var countedBuilds atomic.Int64

func init() {
	RegisterEnvironment("counted_environment", func(simulator.ComponentSpec) (MCTSEnvironment, error) {
		countedBuilds.Add(1)
		return tttEnvironment()
	})
}

// TestEnvironmentsAreSharedWithinARun: partitions naming the same environment
// spec search one environment, as mcts_self_play's do, since an environment
// may keep state its partitions must agree on.
func TestEnvironmentsAreSharedWithinARun(t *testing.T) {
	twin := strings.ReplaceAll(readFile(t, "testdata/selfplay/ttt_twin.yaml"),
		"{type: tictactoe, init_grid: [0, 0, 0, 0, 0, 0, 0, 0, 0], current_player: 0}", "{type: counted_environment}")
	builds := func(yaml string) int64 {
		t.Helper()
		before := countedBuilds.Load()
		if _, err := LoadConfig(writeConfigPath(t, yaml)); err != nil {
			t.Fatal(err)
		}
		return countedBuilds.Load() - before
	}

	t.Run("the apply, and the tree and rollout of its nested search, share one", func(t *testing.T) {
		if n := builds(twin); n != 1 {
			t.Errorf("built %d environments for one spec in one run, want 1", n)
		}
	})

	t.Run("so do an apply and a search stated as a top-level embedded: run", func(t *testing.T) {
		yaml := `main:
  partitions:
  - {name: ttt_apply, iteration: {type: mcts_apply, environment: {type: counted_environment}}, params_as_partitions: {best_idx_partition: [ttt_search]}, init_state_values: [0, 0, 0, 0, 0, 0, 0, 0, 0, 0], state_history_depth: 1, seed: 1}
  - {name: ttt_search, params: {burn_in_steps: [0]}, init_state_values: [-1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0], state_history_depth: 1, seed: 1}
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 1}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
embedded:
- name: ttt_search
  partitions:
  - {name: ttt_tree, iteration: {type: mcts_tree, environment: {type: counted_environment}}, params_as_partitions: {rollout_scores_partition: [ttt_rollout]}, init_state_values: [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0], state_history_depth: 1, seed: 1}
  - {name: ttt_rollout, iteration: {type: mcts_rollout, environment: {type: counted_environment}}, params_from_upstream: {leaf: {upstream: ttt_tree, indices: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11]}}, init_state_values: [0, 0, 0], state_history_depth: 1, seed: 2}
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 5}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`
		if n := builds(yaml); n != 1 {
			t.Errorf("built %d environments for one spec in one run, want 1", n)
		}
	})

	t.Run("a different spec is a different environment", func(t *testing.T) {
		yaml := strings.Replace(twin, "{type: mcts_rollout, environment: *env}",
			"{type: mcts_rollout, environment: {type: counted_environment, variant: 2}}", 1)
		if n := builds(yaml); n != 2 {
			t.Errorf("built %d environments for two specs, want 2", n)
		}
	})

	t.Run("each load is its own run, as each ensemble member is", func(t *testing.T) {
		if n := builds(twin) + builds(twin); n != 2 {
			t.Errorf("built %d environments over two loads, want 2", n)
		}
	})

	t.Run("each replica of a replicated run has its own", func(t *testing.T) {
		yaml := `main:
  partitions:
  - name: replicas
    iteration:
      type: replicated_simulation_run
      replicas: 3
      run:
        partitions:
        - {name: a, iteration: {type: mcts_apply, environment: {type: counted_environment}}, params_as_partitions: {best_idx_partition: [b]}, init_state_values: [0, 0, 0, 0, 0, 0, 0, 0, 0, 0], state_history_depth: 1, seed: 1}
        - {name: b, iteration: {type: constant_values}, params: {}, init_state_values: [-1], state_history_depth: 1, seed: 0}
        simulation:
          termination_condition: {type: number_of_steps, max_steps: 1}
          timestep_function: {type: constant, stepsize: 1.0}
          init_time_value: 0.0
      output_partition: b
    params: {}
    init_state_values: [0, 0, 0]
    state_history_depth: 1
    seed: 0
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 1}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`
		if n := builds(yaml); n != 3 {
			t.Errorf("built %d environments for three concurrent replicas, want 3", n)
		}
	})

	t.Run("ResolveIteration resolves a run of its own", func(t *testing.T) {
		before := countedBuilds.Load()
		for range 2 {
			if _, err := ResolveIteration(simulator.ComponentSpec{Type: "mcts_apply",
				Fields: map[string]interface{}{"environment": map[interface{}]interface{}{"type": "counted_environment"}}}); err != nil {
				t.Fatal(err)
			}
		}
		if n := countedBuilds.Load() - before; n != 2 {
			t.Errorf("two standalone resolutions built %d environments, want 2", n)
		}
	})
}

func TestRegisteredEnvironmentsAsComponents(t *testing.T) {
	t.Run("tic-tac-toe self-play, written out over the registered environment, matches the macro exactly", func(t *testing.T) {
		macro, twin := runPair(t, readFile(t, "../../cfg/example_mcts_config.yaml"),
			readFile(t, "testdata/selfplay/ttt_twin.yaml"))
		assertStoragesEqual(t, "twin vs macro", twin.Storage, macro.Storage)
		// Ten plies of self-play fill the board: a perfect-play draw has no
		// empty cell left.
		final := twin.Storage.GetValues("ttt_apply")[10]
		for cell := 0; cell < 9; cell++ {
			if final[cell] == 0 {
				t.Errorf("cell %d is still empty after ten plies: %v", cell, final[:9])
			}
		}
	})

	t.Run("an environment's own sims per ply applies when the config states none", func(t *testing.T) {
		selfPlay := func(sims string) [][]float64 {
			result, err := RunToStorage(writeConfig(t, "macros:\n- type: mcts_self_play\n  name: g\n"+
				"  steps: 3\n  seed: 5\n"+sims+"  env: {type: eleven_sims_environment}\n"))
			if err != nil {
				t.Fatal(err)
			}
			return result.Storage.GetValues("g_search")
		}
		unstated, eleven, fifty := selfPlay(""), selfPlay("  sims_per_ply: 11\n"), selfPlay("  sims_per_ply: 50\n")
		if !equalRows(unstated, eleven) || equalRows(unstated, fifty) {
			t.Error("an unstated sims_per_ply should be the environment's 11")
		}
	})

	t.Run("a registered environment takes the mcts_* settings", func(t *testing.T) {
		twin := readFile(t, "testdata/selfplay/ttt_twin.yaml")
		plain, err := RunToStorage(writeConfig(t, twin))
		if err != nil {
			t.Fatal(err)
		}
		tuned, err := RunToStorage(writeConfig(t, strings.NewReplacer(
			"{type: mcts_tree, environment: *env}", "{type: mcts_tree, environment: *env, exploration: 0.1, max_tree_depth: 2}",
			"{type: mcts_rollout, environment: *env}", "{type: mcts_rollout, environment: *env, rollout_max_steps: 1}",
		).Replace(twin)))
		if err != nil {
			t.Fatal(err)
		}
		if equalRows(plain.Storage.GetValues("ttt_search"), tuned.Storage.GetValues("ttt_search")) {
			t.Error("the search settings changed nothing")
		}
	})
}

func TestNewMCTSEnvironment(t *testing.T) {
	valid := MCTSEnvironmentSpec[agents.TTTState, agents.TTTAction]{
		Env: &agents.TTTGame{}, Decoder: agents.TTTDecode, Encoder: agents.TTTEncode,
		MaxLegalActions: 9, StateWidth: agents.TTTWidth, Players: 2,
	}
	cases := map[string]func(*MCTSEnvironmentSpec[agents.TTTState, agents.TTTAction]){
		"an MCTS environment needs Env":                 func(s *MCTSEnvironmentSpec[agents.TTTState, agents.TTTAction]) { s.Env = nil },
		"an MCTS environment needs Encoder and Decoder": func(s *MCTSEnvironmentSpec[agents.TTTState, agents.TTTAction]) { s.Decoder = nil },
		"needs MaxLegalActions, StateWidth and Players > 0, got 0, 10 and 2": func(s *MCTSEnvironmentSpec[agents.TTTState, agents.TTTAction]) {
			s.MaxLegalActions = 0
		},
		"needs MaxLegalActions, StateWidth and Players > 0, got 9, 0 and 2": func(s *MCTSEnvironmentSpec[agents.TTTState, agents.TTTAction]) {
			s.StateWidth = 0
		},
		"needs MaxLegalActions, StateWidth and Players > 0, got 9, 10 and 0": func(s *MCTSEnvironmentSpec[agents.TTTState, agents.TTTAction]) {
			s.Players = 0
		},
	}
	for want, breakSpec := range cases {
		spec := valid
		breakSpec(&spec)
		if _, err := NewMCTSEnvironment(spec); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("expected an error containing %q, got %v", want, err)
		}
	}
	if spec := valid; spec.Encoder != nil {
		spec.Encoder = nil
		if _, err := NewMCTSEnvironment(spec); err == nil {
			t.Error("a missing Encoder should be refused")
		}
	}
	environment, err := NewMCTSEnvironment(valid)
	if err != nil {
		t.Fatal(err)
	}
	if rollout := environment.rolloutIteration(0).(*agents.MCTSRolloutIteration[agents.TTTState, agents.TTTAction]); rollout.Cfg.Rollout == nil {
		t.Error("an environment with no rollout should play out uniformly at random")
	}
}

// TestChanceNodesFollowTheEnvironment: a search averages over sampled
// successors only for an environment that can sample them, and pinned_noise
// turns that off.
func TestChanceNodesFollowTheEnvironment(t *testing.T) {
	deterministic, err := tttEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]interface{}{}
	for key, value := range mctsFixtureEnvironment {
		if key != "type" {
			fields[key] = value
		}
	}
	stochastic, err := resolveMCTSEnvironment(simulator.ComponentSpec{Type: "simulation", Fields: fields})
	if err != nil {
		t.Fatal(err)
	}
	chance := func(environment MCTSEnvironment, pinned bool) bool {
		switch tree := environment.tree(mctsTreeSettings{pinnedNoise: pinned}).(type) {
		case *agents.MCTSTreeIteration[agents.TTTState, agents.TTTAction]:
			return tree.ChanceNodes
		case *agents.MCTSTreeIteration[[]float64, int]:
			return tree.ChanceNodes
		}
		t.Fatal("unexpected tree type")
		return false
	}
	if chance(deterministic, false) || chance(deterministic, true) {
		t.Error("deterministic rules have no successors to sample, so no chance nodes")
	}
	if !chance(stochastic, false) || chance(stochastic, true) {
		t.Error("a forward model's search samples successors unless its noise is pinned")
	}
}

// TestApplyMCTSSearchSettingsOverrideSemantics pins the override-if-set rule the
// registry depends on: a registered environment carries per-ruleset defaults, and
// a config overrides only the knobs it actually states. Getting this backwards
// would silently reset every unstated knob to zero.
func TestApplyMCTSSearchSettingsOverrideSemantics(t *testing.T) {
	base := func() macros.MCTSSelfPlaySpec[agents.TTTState, agents.TTTAction] {
		spec := macros.MCTSSelfPlaySpec[agents.TTTState, agents.TTTAction]{
			Name:       "builder_default",
			SimsPerPly: 11,
			Seed:       3,
		}
		spec.Cfg.Simulations = 7
		spec.Cfg.Exploration = 0.5
		return spec
	}

	t.Run("unset settings leave the builder's defaults alone", func(t *testing.T) {
		spec := base()
		macros.ApplyMCTSSearchSettings(&spec, macros.MCTSSearchSettings{Name: "from_config"})
		if spec.SimsPerPly != 11 || spec.Seed != 3 ||
			spec.Cfg.Simulations != 7 || spec.Cfg.Exploration != 0.5 {
			t.Errorf("builder defaults were clobbered: %+v cfg=%+v", spec, spec.Cfg)
		}
		if spec.Name != "from_config" {
			t.Errorf("name should always come from config, got %q", spec.Name)
		}
	})

	t.Run("stated settings win", func(t *testing.T) {
		spec := base()
		macros.ApplyMCTSSearchSettings(&spec, macros.MCTSSearchSettings{
			Name:            "from_config",
			SimsPerPly:      99,
			Seed:            42,
			Simulations:     100,
			Exploration:     1.4,
			MaxTreeDepth:    5,
			RolloutMaxSteps: 30,
		})
		if spec.SimsPerPly != 99 || spec.Seed != 42 || spec.Cfg.Simulations != 100 ||
			spec.Cfg.Exploration != 1.4 || spec.Cfg.MaxTreeDepth != 5 ||
			spec.Cfg.RolloutMaxSteps != 30 {
			t.Errorf("config settings did not take effect: %+v cfg=%+v", spec, spec.Cfg)
		}
	})

	t.Run("an empty name leaves the builder's name", func(t *testing.T) {
		spec := base()
		macros.ApplyMCTSSearchSettings(&spec, macros.MCTSSearchSettings{SimsPerPly: 2})
		if spec.Name != "builder_default" {
			t.Errorf("empty name should not blank the builder's, got %q", spec.Name)
		}
	})
}
