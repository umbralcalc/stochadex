package api

import (
	"fmt"
	"sort"
	"strings"

	"github.com/umbralcalc/stochadex/pkg/macros"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// The environment registry lets the mcts_self_play macro name an
// agents.Environment from config.
//
// It is a hook rather than a {type: ...} data spec because an environment is
// arbitrary decision rules (Legal / Apply / Terminal), which the repo boundary
// places downstream. Spelling those as data would mean a rules language inside
// the config. The registered builder receives the whole ComponentSpec verbatim
// and returns finished partitions, so the fields under `env:` are parsed by the
// downstream and never by this package.
//
// The builder returns partitions rather than an environment because
// macros.NewMCTSSelfPlayPartitions is generic in S and A: a registry map holds a
// concrete type, and erasing the parameters would force an encode/decode round
// trip through every Apply in the search hot loop. macros.MCTSSearchSettings
// carries the config-stated half across the boundary.

// EnvironmentBuilder constructs the partitions of an MCTS self-play stack from
// a downstream environment's data spec plus the search settings the config
// stated. Implementations fill the typed half of a macros.MCTSSelfPlaySpec,
// call macros.ApplyMCTSSearchSettings with the settings, and return
// macros.NewMCTSSelfPlayPartitions of the result.
type EnvironmentBuilder func(
	spec simulator.ComponentSpec,
	settings macros.MCTSSearchSettings,
) ([]*simulator.PartitionConfig, error)

// environmentBuilders holds every registered environment spelling. Unlike the
// iteration registry there is no separate core map to shadow: the engine claims
// only the "tictactoe" fixture name, registered through this same entry point.
var environmentBuilders = map[string]EnvironmentBuilder{}

// RegisterEnvironment adds an environment spelling usable as the mcts_self_play
// macro's `env:` type. Call it from an init function in the module that owns
// the environment; the engine only sees it if the binary links that module, so
// a downstream ships its own CLI (as cmd/stochadex does for the ONNX partition).
//
// Panics on a duplicate name, so two modules cannot silently claim one spelling.
func RegisterEnvironment(typeName string, build EnvironmentBuilder) {
	if _, exists := environmentBuilders[typeName]; exists {
		panic("api: duplicate environment registration " + typeName)
	}
	environmentBuilders[typeName] = build
}

// RegisteredEnvironments returns the registered spellings in sorted order, for
// error messages and for a downstream to check what its binary has linked.
func RegisteredEnvironments() []string {
	names := make([]string, 0, len(environmentBuilders))
	for name := range environmentBuilders {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ResolveEnvironment dispatches an `env:` spec to its registered builder.
func ResolveEnvironment(
	spec simulator.ComponentSpec,
	settings macros.MCTSSearchSettings,
) ([]*simulator.PartitionConfig, error) {
	build, ok := environmentBuilders[spec.Type]
	if !ok {
		return nil, fmt.Errorf(
			"environment: unknown type %q (registered: %s). An environment is "+
				"supplied by the downstream module that owns the decision rules and "+
				"registered with api.RegisterEnvironment — the engine ships only the "+
				"tictactoe fixture. If you registered one, check the binary links "+
				"that module",
			spec.Type,
			strings.Join(RegisteredEnvironments(), ", "),
		)
	}
	partitions, err := build(spec, settings)
	if err != nil {
		return nil, fmt.Errorf("environment %q: %w", spec.Type, err)
	}
	if len(partitions) == 0 {
		return nil, fmt.Errorf(
			"environment %q: builder returned no partitions", spec.Type)
	}
	return partitions, nil
}
