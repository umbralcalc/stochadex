package graph

import (
	"reflect"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// partition is a terse constructor for a wiring-only PartitionConfig. The
// Iteration is left nil: Build reads only names, depths and wiring, never the
// implementation.
func partition(
	name string,
	depth int,
	upstream map[string]simulator.NamedUpstreamConfig,
	asPartitions map[string][]string,
) *simulator.PartitionConfig {
	return &simulator.PartitionConfig{
		Name:               name,
		InitStateValues:    []float64{0.0},
		StateHistoryDepth:  depth,
		ParamsFromUpstream: upstream,
		ParamsAsPartitions: asPartitions,
	}
}

func newGen(configs ...*simulator.PartitionConfig) *simulator.ConfigGenerator {
	gen := simulator.NewConfigGenerator()
	for _, c := range configs {
		gen.SetPartition(c)
	}
	return gen
}

func hasEdge(g *Graph, want Edge) bool {
	for _, e := range g.Edges {
		if e == want {
			return true
		}
	}
	return false
}

func TestBuildEdges(t *testing.T) {
	// source (depth 5)
	//   -> middle via within-step inject
	//   -> sink via cross-history read
	gen := newGen(
		partition("source", 5, nil, nil),
		partition("middle", 1,
			map[string]simulator.NamedUpstreamConfig{
				"driver": {Upstream: "source", Indices: []int{0}},
			}, nil),
		partition("sink", 1, nil,
			map[string][]string{"other": {"source"}}),
	)
	g := Build(gen)

	if !reflect.DeepEqual(g.Names, []string{"source", "middle", "sink"}) {
		t.Fatalf("unexpected names: %v", g.Names)
	}

	inject := Edge{Source: 0, Target: 1, SourceName: "source", TargetName: "middle",
		Kind: ParamsInject, Param: "driver", Window: 0}
	if !hasEdge(g, inject) {
		t.Errorf("missing inject edge; got %+v", g.Edges)
	}

	cross := Edge{Source: 0, Target: 2, SourceName: "source", TargetName: "sink",
		Kind: CrossHistory, Param: "other", Window: 5}
	if !hasEdge(g, cross) {
		t.Errorf("missing cross-history edge (window should be source's depth 5); got %+v", g.Edges)
	}

	// Self-dependencies are assumed universal and never emitted as edges, not
	// even for a partition with history depth > 1.
	for _, e := range g.Edges {
		if e.Source == e.Target {
			t.Errorf("unexpected self edge: %+v", e)
		}
	}
}

func TestInjectCyclesDetectsDeadlock(t *testing.T) {
	// a and b inject into each other within-step: a genuine deadlock.
	gen := newGen(
		partition("a", 1,
			map[string]simulator.NamedUpstreamConfig{"x": {Upstream: "b"}}, nil),
		partition("b", 1,
			map[string]simulator.NamedUpstreamConfig{"y": {Upstream: "a"}}, nil),
	)
	g := Build(gen)

	if !g.HasDeadlock() {
		t.Fatal("expected a deadlock cycle to be detected")
	}
	cycles := g.InjectCycles()
	if len(cycles) != 1 || !reflect.DeepEqual(cycles[0], []int{0, 1}) {
		t.Errorf("expected one cycle {0,1}, got %v", cycles)
	}
}

func TestSelfInjectIsDeadlock(t *testing.T) {
	// A partition injecting into itself within-step deadlocks on its own channel.
	gen := newGen(
		partition("a", 1,
			map[string]simulator.NamedUpstreamConfig{"x": {Upstream: "a"}}, nil),
	)
	g := Build(gen)
	if !g.HasDeadlock() {
		t.Fatal("expected self-inject to be flagged as a deadlock")
	}
}

func TestLagBreaksCycle(t *testing.T) {
	// a depends on b within-step (inject b -> a); b reads a's history (cross
	// a -> b). The dependency is cyclic, but the lag edge breaks the deadlock,
	// so the inject subgraph is acyclic and InjectCycles must be empty.
	gen := newGen(
		partition("a", 1,
			map[string]simulator.NamedUpstreamConfig{"in": {Upstream: "b"}},
			nil),
		partition("b", 1, nil,
			map[string][]string{"a_ref": {"a"}}),
	)
	g := Build(gen)

	if g.HasDeadlock() {
		t.Errorf("cycle routed through a lag edge must not be a deadlock; cycles=%v", g.InjectCycles())
	}
}

func TestBuildDeterministic(t *testing.T) {
	build := func() *Graph {
		return Build(newGen(
			partition("p", 2, nil,
				map[string][]string{"a": {"q"}, "b": {"q"}, "c": {"q"}}),
			partition("q", 3,
				map[string]simulator.NamedUpstreamConfig{
					"m": {Upstream: "q_src"}, "n": {Upstream: "q_src"},
				}, nil),
			partition("q_src", 1, nil, nil),
		))
	}
	if !reflect.DeepEqual(build(), build()) {
		t.Error("Build is not deterministic across runs")
	}
}

func TestRenderContains(t *testing.T) {
	gen := newGen(
		partition("a", 1,
			map[string]simulator.NamedUpstreamConfig{"x": {Upstream: "b"}}, nil),
		partition("b", 2, nil,
			map[string][]string{"a_ref": {"a"}}),
	)
	g := Build(gen)

	mermaid := g.Mermaid()
	// The cross-history read of "a" originates from a past-copy node (n0past),
	// not the live n0, and that copy is classed for its distinct colour.
	for _, want := range []string{
		"flowchart TB", `n0["a"]`, "-->|x|",
		`n0past["a"]`, "n0past -.->|a_ref|", "classDef pastcopy", "class n0past pastcopy",
	} {
		if !strings.Contains(mermaid, want) {
			t.Errorf("mermaid output missing %q:\n%s", want, mermaid)
		}
	}

	dot := g.DOT()
	for _, want := range []string{
		"digraph simulation", `label="a"`, "style=dashed",
		"n0past [label=\"a\"", "n0past -> n1",
	} {
		if !strings.Contains(dot, want) {
			t.Errorf("dot output missing %q:\n%s", want, dot)
		}
	}
}

func TestPastCopyMakesMutualLagAcyclic(t *testing.T) {
	// a injects into b within-step; b reads a's past state (a mutual dependency
	// broken by the lag). The rendered graph must keep the live producer node
	// (n0) free of the lag edge — that edge originates from the past-copy node —
	// so the drawing has no cycle.
	gen := newGen(
		partition("a", 1, nil,
			map[string][]string{"b_past": {"b"}}),
		partition("b", 1,
			map[string]simulator.NamedUpstreamConfig{"a_now": {Upstream: "a"}}, nil),
	)
	g := Build(gen)

	mermaid := g.Mermaid()
	// The lag read of b must leave from n1past, never from the live n1.
	if !strings.Contains(mermaid, "n1past -.->|b_past| n0") {
		t.Errorf("expected lag edge from past-copy node; got:\n%s", mermaid)
	}
	if strings.Contains(mermaid, "n1 -.->") {
		t.Errorf("live producer node must not carry the lag edge:\n%s", mermaid)
	}
}
