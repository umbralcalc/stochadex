package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/agents"
	"github.com/umbralcalc/stochadex/pkg/macros"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// TestRegisterEnvironment covers the downstream-environment hook: a module
// layered above pkg/api contributes an agents.Environment spelling without the
// engine importing it, or knowing anything about the rules it encodes.
func TestRegisterEnvironment(t *testing.T) {
	var gotSpec simulator.ComponentSpec
	var gotSettings macros.MCTSSearchSettings
	RegisterEnvironment("test_environment", func(
		spec simulator.ComponentSpec,
		settings macros.MCTSSearchSettings,
	) ([]*simulator.PartitionConfig, error) {
		gotSpec = spec
		gotSettings = settings
		selfPlay := macros.MCTSSelfPlaySpec[agents.TTTState, agents.TTTAction]{
			Env: &agents.TTTGame{},
			Cfg: agents.MCTSConfig[agents.TTTState, agents.TTTAction]{
				Rollout: agents.UniformRandomRollout[agents.TTTState, agents.TTTAction](),
			},
			Decoder:         agents.TTTDecode,
			Encoder:         agents.TTTEncode,
			MaxLegalActions: 9,
			StateWidth:      agents.TTTWidth,
			Players:         2,
			SimsPerPly:      11,
		}
		macros.ApplyMCTSSearchSettings(&selfPlay, settings)
		return macros.NewMCTSSelfPlayPartitions(selfPlay), nil
	})

	t.Run("a registered environment is dispatched, with its whole spec", func(t *testing.T) {
		partitions, err := ResolveEnvironment(
			simulator.ComponentSpec{
				Type:   "test_environment",
				Fields: map[string]interface{}{"rules": "uno.yaml", "players": 3},
			},
			macros.MCTSSearchSettings{Name: "game", Simulations: 25},
		)
		if err != nil {
			t.Fatalf("ResolveEnvironment: %v", err)
		}
		if len(partitions) != 2 {
			t.Fatalf("expected the apply + search partitions, got %d", len(partitions))
		}
		// The engine must hand the environment's own fields through untouched —
		// that is the whole point: it never parses a downstream's rules format.
		if gotSpec.Type != "test_environment" ||
			gotSpec.Fields["rules"] != "uno.yaml" || gotSpec.Fields["players"] != 3 {
			t.Errorf("spec not passed through verbatim: %+v", gotSpec)
		}
		if gotSettings.Name != "game" || gotSettings.Simulations != 25 {
			t.Errorf("search settings not passed through: %+v", gotSettings)
		}
		if partitions[0].Name != "game_apply" {
			t.Errorf("config's name: should drive partition naming, got %q", partitions[0].Name)
		}
	})

	t.Run("an unknown environment names the registered ones", func(t *testing.T) {
		_, err := ResolveEnvironment(
			simulator.ComponentSpec{Type: "no_such_environment"},
			macros.MCTSSearchSettings{Name: "x"},
		)
		if err == nil {
			t.Fatal("expected an error for an unknown environment")
		}
		// The likely cause of this error is a binary that did not link the
		// registering module, so the message has to show what it *did* link.
		if !strings.Contains(err.Error(), "no_such_environment") ||
			!strings.Contains(err.Error(), "tictactoe") {
			t.Errorf("error should name the unknown type and the registered ones, got: %v", err)
		}
	})

	t.Run("a builder error is wrapped with the environment name", func(t *testing.T) {
		RegisterEnvironment("failing_environment", func(
			simulator.ComponentSpec, macros.MCTSSearchSettings,
		) ([]*simulator.PartitionConfig, error) {
			return nil, fmt.Errorf("bad ruleset")
		})
		_, err := ResolveEnvironment(
			simulator.ComponentSpec{Type: "failing_environment"},
			macros.MCTSSearchSettings{Name: "x"},
		)
		if err == nil || !strings.Contains(err.Error(), "failing_environment") ||
			!strings.Contains(err.Error(), "bad ruleset") {
			t.Errorf("expected the builder error wrapped with its name, got: %v", err)
		}
	})

	t.Run("a builder returning no partitions is an error, not a silent no-op", func(t *testing.T) {
		RegisterEnvironment("empty_environment", func(
			simulator.ComponentSpec, macros.MCTSSearchSettings,
		) ([]*simulator.PartitionConfig, error) {
			return nil, nil
		})
		_, err := ResolveEnvironment(
			simulator.ComponentSpec{Type: "empty_environment"},
			macros.MCTSSearchSettings{Name: "x"},
		)
		if err == nil || !strings.Contains(err.Error(), "no partitions") {
			t.Errorf("expected an empty-partitions error, got: %v", err)
		}
	})

	t.Run("registering the same name twice panics", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("expected a panic on duplicate registration")
			}
		}()
		RegisterEnvironment("test_environment", func(
			simulator.ComponentSpec, macros.MCTSSearchSettings,
		) ([]*simulator.PartitionConfig, error) {
			return nil, nil
		})
	})
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
