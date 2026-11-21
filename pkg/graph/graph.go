package graph

import (
	"sort"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// EdgeKind distinguishes the dependency types between partitions.
type EdgeKind int

const (
	// ParamsInject is a within-step dependency declared by params_from_upstream:
	// the source's this-step output is piped into the target's params before the
	// target iterates. It imposes a computation order and deadlocks if cyclic.
	ParamsInject EdgeKind = iota
	// CrossHistory is a lag>=1 dependency declared by params_as_partitions: the
	// target may read the source partition's previous-step state history.
	CrossHistory
	// HistoryRead is a lag>=1 dependency an iteration states in its own spec:
	// the target reads the source partition's committed state, one step behind
	// or further (an expression's upstreams, for example). Unlike CrossHistory
	// it is the iteration's own declaration of what it reads, so it is exact.
	HistoryRead
)

// String returns a stable snake_case label for the edge kind.
func (k EdgeKind) String() string {
	switch k {
	case ParamsInject:
		return "params_inject"
	case CrossHistory:
		return "cross_history"
	case HistoryRead:
		return "history_read"
	default:
		return "unknown"
	}
}

// Reliable reports whether this edge kind is exact (engine-enforced wiring, or
// an iteration's own statement of what it reads) rather than declared capacity
// (a may-read hint). ParamsInject and HistoryRead edges are reliable, and only
// they should be asserted on.
func (k EdgeKind) Reliable() bool { return k == ParamsInject || k == HistoryRead }

// Lagged reports whether this edge kind reads the source's past committed
// state rather than its this-step output. A lagged edge imposes no computation
// order and can never deadlock.
func (k EdgeKind) Lagged() bool { return k == CrossHistory || k == HistoryRead }

// HistoryReader is implemented by an iteration whose spec states which other
// partitions' committed state it reads. HistoryReads maps the name the
// iteration reads each by (an expression's upstream alias) to that partition's
// name. Build draws each as a HistoryRead edge.
type HistoryReader interface {
	HistoryReads() map[string]string
}

// Edge is a directed dependency: the Target partition depends on the Source.
type Edge struct {
	Source, Target int
	// SourceName and TargetName are the partition names for the indices above.
	SourceName, TargetName string
	Kind                   EdgeKind
	// Param is the name that carries the dependency: the params_from_upstream or
	// params_as_partitions key, or the name a HistoryReader reads the source by.
	Param string
	// Window is the maximum readable lag: the source partition's state history
	// depth for a CrossHistory or HistoryRead edge, 0 for a within-step
	// ParamsInject edge.
	Window int
}

// Graph is the dependency graph of a simulation: partition names in index order
// plus the derived edges. It is a pure function of the configuration.
type Graph struct {
	Names []string
	Edges []Edge
}

// Build derives the dependency graph from a ConfigGenerator. It is
// deterministic — edges are emitted in a stable order (by target index, then
// kind, then sorted param key) — and performs no simulation run. It calls no
// method of a partition's iteration but HistoryReads.
func Build(gen *simulator.ConfigGenerator) *Graph {
	names := gen.PartitionNames()
	index := make(map[string]int, len(names))
	for i, name := range names {
		index[name] = i
	}
	g := &Graph{Names: names}
	for target, name := range names {
		cfg := gen.GetPartition(name)

		// ParamsInject: exact within-step wiring from params_from_upstream.
		for _, param := range sortedKeys(cfg.ParamsFromUpstream) {
			up := cfg.ParamsFromUpstream[param]
			src, ok := index[up.Upstream]
			if !ok {
				continue
			}
			g.Edges = append(g.Edges, Edge{
				Source:     src,
				Target:     target,
				SourceName: up.Upstream,
				TargetName: name,
				Kind:       ParamsInject,
				Param:      param,
				Window:     0,
			})
		}

		// CrossHistory: declared may-read capacity from params_as_partitions.
		for _, param := range sortedKeys(cfg.ParamsAsPartitions) {
			for _, ref := range cfg.ParamsAsPartitions[param] {
				src, ok := index[ref]
				if !ok || src == target {
					// Self references are assumed universal and never drawn as
					// edges (see package doc), so skip a params_as_partitions
					// entry that points a partition at itself.
					continue
				}
				g.Edges = append(g.Edges, Edge{
					Source:     src,
					Target:     target,
					SourceName: ref,
					TargetName: name,
					Kind:       CrossHistory,
					Param:      param,
					Window:     gen.GetPartition(ref).StateHistoryDepth,
				})
			}
		}

		// HistoryRead: the lagged reads the iteration's own spec states.
		reader, ok := cfg.Iteration.(HistoryReader)
		if !ok {
			continue
		}
		reads := reader.HistoryReads()
		for _, param := range sortedKeys(reads) {
			src, ok := index[reads[param]]
			if !ok || src == target {
				continue
			}
			g.Edges = append(g.Edges, Edge{
				Source:     src,
				Target:     target,
				SourceName: reads[param],
				TargetName: name,
				Kind:       HistoryRead,
				Param:      param,
				Window:     gen.GetPartition(reads[param]).StateHistoryDepth,
			})
		}
	}
	return g
}

// sortedKeys returns the keys of a string-keyed map in sorted order so that
// edge emission is deterministic regardless of Go's map iteration order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
