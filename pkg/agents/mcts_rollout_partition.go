package agents

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// MCTSRolloutPartition runs one rollout per stochadex step. It reads the leaf
// state to roll out from via params_from_upstream (typically wired to a
// MCTSTreePartition's leaf_state slot via Indices) and outputs a per-player
// score vector plus an ok flag in its own row.
//
// Row layout (width = Players + 1):
//   row[0 .. Players-1]   per-player [0,1] scores from the rollout
//   row[Players]          ok flag (1 if the rollout produced valid scores,
//                         0 otherwise — the upstream MCTSTreePartition uses
//                         this to decide whether to apply backupScores or
//                         backupVisits with no signal)
//
// Use MCTSRolloutRowWidth(P) to size InitStateValues / state_width.
//
// Stateless across steps — each Iterate is one independent rollout. Swap
// in FromProgress, WinnerToTerminal, or any custom MCTSRolloutFn via Cfg.Rollout
// without touching the partition wiring.
//
// Warm fields (must be set before Configure):
//   - Env: the typed Environment[S, A] to roll out against.
//   - Cfg: must have Rollout set (the rollout function to invoke).
//   - Decoder: decodes the leaf_state from the upstream-supplied
//     []float64 back into the typed S.
//   - Players: P, the per-player score vector length.
type MCTSRolloutPartition[S any, A any] struct {
	Env     Environment[S, A]
	Cfg     MCTSConfig[S, A]
	Decoder func([]float64) (S, error)
	Players int

	seed uint64
}

// Param key used by MCTSRolloutPartition.Iterate to read the leaf state from
// an upstream MCTSTreePartition via params_from_upstream. The slice should
// be of length StateWidth followed by a single has_leaf flag (matching
// MCTSTreePartition's row layout: leaf_state then has_leaf). Use Indices on
// the NamedUpstreamConfig to slice out the leaf_state + has_leaf section
// of the tree's row.
const MCTSRolloutParamLeaf = "leaf"

// MCTSRolloutRowWidth returns the required InitStateValues / StateWidth for a
// MCTSRolloutPartition with the given player count.
func MCTSRolloutRowWidth(players int) int {
	return players + 1
}

// Configure implements simulator.Iteration.
func (m *MCTSRolloutPartition[S, A]) Configure(partitionIndex int, settings *simulator.Settings) {
	if m.Env == nil {
		panic("agents.MCTSRolloutPartition: Env required")
	}
	if m.Decoder == nil {
		panic("agents.MCTSRolloutPartition: Decoder required")
	}
	if m.Players <= 0 {
		panic("agents.MCTSRolloutPartition: Players must be > 0")
	}
	is := settings.Iterations[partitionIndex]
	if is.StateWidth != MCTSRolloutRowWidth(m.Players) {
		panic("agents.MCTSRolloutPartition: StateWidth must equal MCTSRolloutRowWidth(Players)")
	}
	m.seed = is.Seed
}

// Iterate implements simulator.Iteration.
func (m *MCTSRolloutPartition[S, A]) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	out := make([]float64, MCTSRolloutRowWidth(m.Players))
	cfg := m.Cfg
	cfg.applyDefaults()
	if cfg.Rollout == nil {
		// No rollout function configured: write zeros + ok=0 so the
		// downstream tree partition treats this as no-signal.
		return out
	}
	leafSlice, ok := params.GetOk(MCTSRolloutParamLeaf)
	if !ok {
		return out
	}
	// Layout: [leaf_state(W), has_leaf(1)]. Reject if has_leaf == 0.
	if len(leafSlice) < 2 {
		return out
	}
	width := len(leafSlice) - 1
	hasLeaf := leafSlice[width]
	if hasLeaf == 0 {
		return out
	}
	encoded := make([]float64, width)
	copy(encoded, leafSlice[:width])
	leaf, err := m.Decoder(encoded)
	if err != nil {
		return out
	}
	step := uint64(timestepsHistory.CurrentStepNumber)
	seed := m.seed ^ step ^ uint64(partitionIndex+1)*0x9e3779b97f4a7c15
	scores, rolloutOK, rolloutErr := cfg.Rollout(m.Env, leaf, cfg.RolloutMaxSteps, seed)
	if rolloutErr != nil || !rolloutOK {
		return out
	}
	if len(scores) != m.Players {
		// Misconfigured env: refuse to write a malformed row rather than
		// silently truncate.
		return out
	}
	copy(out, scores)
	out[m.Players] = 1
	_ = stateHistories // not consumed
	return out
}
