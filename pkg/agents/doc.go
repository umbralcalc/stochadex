// Package agents provides decision-making agents that operate over a generic
// Environment[S, A] interface. The package is intended to host any agent built
// on the same environment framework — currently it ships MCTS (UCT) as the
// only agent, with MAST as an optional rollout strategy on top.
//
// Per-player terminal scores are []float64 in [0,1] (the established stochadex
// value convention). Codecs (encoder/decoder for S into the stochadex row's
// []float64) are supplied by the caller as function fields on each partition
// — this package does not depend on any encoding protocol.
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
//   - Self-play: compose ApplyPartition + an embedded search sim
//     (analysis.NewMCTSSelfPlayPartitions wires this up)
//   - Per-simulation telemetry: run MCTSTreePartition + MCTSRolloutPartition
//     directly without the apply layer, then read the tree's row
//   - YAML / pkg/api use: ship non-generic façade types per environment family
//     that bake in the type parameters, since generic types in YAML iteration
//     strings are awkward
package agents
