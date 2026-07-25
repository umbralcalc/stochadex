// Package agents provides decision-making agents that operate over a generic
// Environment[S, A] interface. The package is intended to host any agent built
// on the same environment framework. Currently it ships MCTS (UCT) as the
// only agent, with MAST as an optional rollout strategy on top.
//
// Per-player terminal scores are []float64 in [0,1] (the established stochadex
// value convention). Codecs (encoder/decoder for S into the stochadex row's
// []float64) are supplied by the caller as function fields on each partition.
// This package does not depend on any encoding protocol.
//
// Key Features:
//   - Generic Environment[S, A] interface (Legal/Apply/Terminal/Actor/Players)
//   - MCTS decomposed as three partitions: tree (selection + backup),
//     rollout (one playout per step), and apply (state advancer)
//   - Pluggable rollout functions (UniformRandomRollout, FromProgress,
//     WinnerToTerminal)
//   - MAST as an optional rollout strategy: a learning rollout policy backed
//     by a separate aggregation partition holding running per-action-key
//     (count, sum) state
//   - Cycle-breaking by mixing within-step params_from_upstream with lag-1
//     state-history reads, so tree ↔ rollout and apply ↔ search both compose
//     without deadlock
//   - One-shot helper RunMCTSSearch for ad-hoc searches outside a coordinator
//
// Usage Patterns:
//   - Self-play: compose ApplyIteration + an embedded search sim
//     (macros.NewMCTSSelfPlayPartitions wires this up)
//   - Per-simulation telemetry: run MCTSTreeIteration + MCTSRolloutIteration
//     directly without the apply layer, then read the tree's row
//   - YAML / pkg/api use: register an environment with api.RegisterEnvironment
//     and name it from the mcts_self_play macro's env: spec. The registered
//     builder keeps the type parameters on the Go side — it fills the typed half
//     of a macros.MCTSSelfPlaySpec and applies the config-stated half
//     (macros.MCTSSearchSettings) over the top — so no generic type ever has to
//     be spelled in YAML.
package agents
