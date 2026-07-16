package graph

import (
	"fmt"
	"sort"
	"strings"
)

// Mermaid renders the graph as a Mermaid flowchart. Mermaid is chosen as the
// default because it renders inline in GitHub-flavoured markdown (e.g. a model
// card) with no external tooling.
//
// Edge styling encodes the reliability distinction from the package doc:
//   - solid arrow  (-->)  : ParamsInject, the exact within-step wiring.
//   - dashed arrow (-.->) : CrossHistory, the may-read hint.
//
// CrossHistory dependencies are reads of a partition's *past* committed state,
// not of its live within-step output. So that the rendered graph stays a DAG,
// such an edge does not point back at the live producer node; instead each
// producer read this way gets a distinct, differently coloured past-copy node
// bearing the same name, and the lag edge originates there. Unrolling the
// one-step lag into its own node means a safe mutual-lag cycle (the documented
// cycle-breaking pattern) no longer draws as a cycle — only a genuine
// within-step deadlock does, and those partitions are highlighted so the hazard
// is visible at a glance.
//
// Self-dependencies are assumed universal and not drawn (see package doc).
func (g *Graph) Mermaid() string {
	var b strings.Builder
	b.WriteString("flowchart TB\n")
	for i, name := range g.Names {
		fmt.Fprintf(&b, "  n%d[%q]\n", i, name)
	}
	// Declare a past-copy node for every partition read as past state, before
	// any edges reference it, so node ids are stable and grouped.
	pastSources := g.crossHistorySources()
	for _, src := range pastSources {
		fmt.Fprintf(&b, "  n%dpast[%q]\n", src, g.Names[src])
	}
	for _, e := range g.Edges {
		switch e.Kind {
		case ParamsInject:
			fmt.Fprintf(&b, "  n%d -->|%s| n%d\n", e.Source, mermaidLabel(e.Param), e.Target)
		case CrossHistory:
			fmt.Fprintf(&b, "  n%dpast -.->|%s| n%d\n", e.Source, mermaidLabel(e.Param), e.Target)
		}
	}
	if len(pastSources) > 0 {
		b.WriteString("  classDef pastcopy fill:#d8e6f3,stroke:#4a7ba6,color:#000;\n")
		for _, src := range pastSources {
			fmt.Fprintf(&b, "  class n%dpast pastcopy;\n", src)
		}
	}
	cycles := g.InjectCycles()
	if len(cycles) > 0 {
		b.WriteString("  classDef deadlock fill:#f88,stroke:#900,color:#000;\n")
		seen := make(map[int]bool)
		for _, comp := range cycles {
			for _, v := range comp {
				if !seen[v] {
					fmt.Fprintf(&b, "  class n%d deadlock;\n", v)
					seen[v] = true
				}
			}
		}
	}
	return b.String()
}

// DOT renders the graph in Graphviz DOT for users who want a rendered image
// (stochadex-graph --format dot ... | dot -Tsvg). CrossHistory reads of past
// state render as differently coloured past-copy nodes, exactly as in Mermaid,
// so the drawn graph is a DAG (see the Mermaid doc for the rationale).
func (g *Graph) DOT() string {
	var b strings.Builder
	b.WriteString("digraph simulation {\n")
	b.WriteString("  rankdir=LR;\n")
	b.WriteString("  node [shape=box];\n")
	for i, name := range g.Names {
		fmt.Fprintf(&b, "  n%d [label=%q];\n", i, name)
	}
	pastSources := g.crossHistorySources()
	for _, src := range pastSources {
		fmt.Fprintf(&b,
			"  n%dpast [label=%q, style=filled, fillcolor=\"#d8e6f3\"];\n",
			src, g.Names[src])
	}
	for _, e := range g.Edges {
		switch e.Kind {
		case ParamsInject:
			fmt.Fprintf(&b, "  n%d -> n%d [label=%q];\n", e.Source, e.Target, e.Param)
		case CrossHistory:
			fmt.Fprintf(&b, "  n%dpast -> n%d [label=%q, style=dashed];\n", e.Source, e.Target, e.Param)
		}
	}
	seen := make(map[int]bool)
	for _, comp := range g.InjectCycles() {
		for _, v := range comp {
			if !seen[v] {
				fmt.Fprintf(&b, "  n%d [style=filled, fillcolor=\"#ff8888\"];\n", v)
				seen[v] = true
			}
		}
	}
	b.WriteString("}\n")
	return b.String()
}

// crossHistorySources returns, in ascending index order, the distinct partition
// indices that are read as past state via a CrossHistory edge. Each gets one
// past-copy node in the rendered graph, shared by all its lag consumers.
func (g *Graph) crossHistorySources() []int {
	seen := make(map[int]bool)
	srcs := make([]int, 0)
	for _, e := range g.Edges {
		if e.Kind == CrossHistory && !seen[e.Source] {
			seen[e.Source] = true
			srcs = append(srcs, e.Source)
		}
	}
	sort.Ints(srcs)
	return srcs
}

// mermaidLabel escapes a Mermaid edge label. Partition and param names in
// stochadex are simple identifiers, but quotes and pipes would break the
// flowchart syntax, so they are neutralised defensively.
func mermaidLabel(s string) string {
	r := strings.NewReplacer(`"`, "#quot;", "|", "#124;")
	return r.Replace(s)
}
