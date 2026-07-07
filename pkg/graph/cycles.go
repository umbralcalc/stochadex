package graph

import "sort"

// InjectCycles returns the cyclic strongly-connected components of the
// ParamsInject subgraph, each as a sorted slice of partition indices. A
// non-empty result means the simulation will deadlock under the default and
// persistent-worker execution strategies: the within-step params channels form
// a receive cycle in which no partition can produce the value another is
// blocked waiting for.
//
// Only ParamsInject edges are considered. A dependency cycle that routes
// through a lag edge (a CrossHistory read, or a partition's assumed read of its
// own history) is safe — the lag consumer reads the previous step's committed
// value — and is the documented cycle-breaking pattern, so it is intentionally
// excluded.
func (g *Graph) InjectCycles() [][]int {
	n := len(g.Names)
	adj := make([][]int, n)
	selfLoop := make([]bool, n)
	for _, e := range g.Edges {
		if e.Kind != ParamsInject {
			continue
		}
		if e.Source == e.Target {
			selfLoop[e.Source] = true
			continue
		}
		adj[e.Source] = append(adj[e.Source], e.Target)
	}
	for i := range adj {
		sort.Ints(adj[i])
	}

	// Tarjan's strongly-connected-components algorithm. A component is a cycle
	// if it has more than one node, or a single node with a self-loop.
	const unvisited = -1
	idx := make([]int, n)
	low := make([]int, n)
	onStack := make([]bool, n)
	for i := range idx {
		idx[i] = unvisited
	}
	var stack []int
	var counter int
	var cycles [][]int

	var strongConnect func(v int)
	strongConnect = func(v int) {
		idx[v] = counter
		low[v] = counter
		counter++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range adj[v] {
			switch {
			case idx[w] == unvisited:
				strongConnect(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			case onStack[w]:
				if idx[w] < low[v] {
					low[v] = idx[w]
				}
			}
		}
		if low[v] != idx[v] {
			return
		}
		var comp []int
		for {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[w] = false
			comp = append(comp, w)
			if w == v {
				break
			}
		}
		if len(comp) > 1 || selfLoop[comp[0]] {
			sort.Ints(comp)
			cycles = append(cycles, comp)
		}
	}

	for v := 0; v < n; v++ {
		if idx[v] == unvisited {
			strongConnect(v)
		}
	}
	return cycles
}

// HasDeadlock reports whether the ParamsInject subgraph contains any cycle,
// i.e. whether the simulation would deadlock under channel-based execution.
func (g *Graph) HasDeadlock() bool {
	return len(g.InjectCycles()) > 0
}
