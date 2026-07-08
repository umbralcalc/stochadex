// Package graph derives the partition dependency graph of a stochadex
// simulation statically from its configuration, before any step runs.
//
// It exists to answer two questions cheaply and deterministically:
//
//   - Will this simulation deadlock? A cycle in the within-step
//     params_from_upstream wiring makes the coordinator's per-partition
//     goroutines block on each other's channels forever. InjectCycles finds
//     those cycles up front so a silent hang becomes a clear error.
//   - What depends on what? Build produces the full dependency graph for
//     documentation and visualisation (see Mermaid / DOT).
//
// # Reliability: one edge class is exact, two are hints
//
// The three edge kinds do not have the same relationship to config, and the
// difference is load-bearing:
//
//   - ParamsInject (from params_from_upstream) is engine-enforced wiring. The
//     coordinator builds a channel the consumer blocks on receiving before its
//     Iterate even runs, so the dependency exists whether or not the iteration
//     reads the injected param. These edges are exact — sound and complete —
//     and are the only ones safe to assert on.
//   - CrossHistory (from params_as_partitions) is declared capacity, not
//     wiring. The actual read lives in iteration code, which config cannot see.
//     It over-reports (a configured partition ref the iteration ignores) and
//     under-reports (a cross-read whose index comes from a plain numeric param
//     or a hard-coded literal, with no params_as_partitions entry). Treat it as
//     a may-read hint for visualisation; do not assert on it.
//
// Self-dependencies are not represented as edges at all: every partition is
// assumed to read its own state history each step, so a self edge would be
// universal noise. A partition's history depth is a property of the node, not a
// relationship worth drawing.
//
// The graph is built from a ConfigGenerator, not a Settings, because
// params_as_partitions is flattened into anonymous float params by
// GenerateConfigs and is unrecoverable afterwards.
package graph
