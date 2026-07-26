package api

import (
	"fmt"
	"sort"
	"strings"

	"github.com/umbralcalc/stochadex/pkg/macros"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// The environment registry: the downstream extension point that lets the
// mcts_self_play macro name an agents.Environment from config.
//
// # Why this is a hook and not a data spelling
//
// Every other component family in this engine resolves from a {type: ...} data
// spec because the thing being named is part of the framework's own catalogue —
// a Wiener process, an exponential kernel, a normal likelihood. An
// agents.Environment is not: it is arbitrary decision-process rules (Legal /
// Apply / Terminal), which the repo boundary places downstream, in the project
// that owns the decision layer. Spelling those rules as data would mean growing
// a rules language inside the config, which is a different (and much larger)
// project than this registry — and one that would only serve games.
//
// So the engine ships no environments beyond the tic-tac-toe fixture below, and
// instead lets a downstream module contribute one. The registered builder is
// handed the environment's whole ComponentSpec verbatim and returns finished
// partitions: the fields under `env:` are parsed by the *downstream*, never by
// this package. A card-game project's `{type: cardgame_rules, rules: uno.yaml}`
// is opaque here — the engine passes it through and never learns what a zone or
// a phase is.
//
// # Why the builder returns partitions rather than an environment
//
// macros.NewMCTSSelfPlayPartitions is generic in the environment's state and
// action types, so this package cannot call it: a registry map has to hold a
// concrete type, and erasing S and A to fit one would force an encode/decode
// round-trip through every Apply in the search hot loop. Having the builder
// call the generic constructor itself keeps the typed Environment[S, A] intact
// for its Go callers and keeps this side free of type parameters entirely.
// macros.MCTSSearchSettings carries the config-stated half across that boundary.

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
