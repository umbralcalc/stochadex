package agents

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ApplyIteration advances the environment by one ply per stochadex outer
// step using a best-action signal supplied either via params_from_upstream
// (within-step) OR via params_as_partitions (lagged read of an upstream
// partition's state-history row). The partition row is the encoded
// current game state; one outer step decodes the row, applies the chosen
// legal action, and writes the encoded post-move state back.
//
// Row layout (width = StateWidth):
//   row[0 .. StateWidth-1]   encoded current game state.
//
// # Two read modes
//
//  1. Direct param mode (ApplyParamBestIdx): set
//     ParamsFromUpstream[ApplyParamBestIdx] to read the best-action index
//     directly within the same step. Used when the upstream partition is
//     not in a self-referential cycle with apply.
//
//  2. State-history mode (ApplyParamBestIdxPartition + BestIdxSlot): set
//     ParamsAsPartitions[ApplyParamBestIdxPartition] to the upstream
//     partition's name, and set BestIdxSlot to the offset of the
//     best-action index within that partition's row. Apply will read the
//     PREVIOUS step's row 0 of that partition each step. Used to break
//     the apply ↔ search dependency cycle in NewMCTSSelfPlayPartitions:
//     search reads apply within-step (so apply does not wait on search),
//     and apply reads search lagged by one step (so search does not wait
//     on apply). The 1-step lag aligns correctly because at step N apply
//     applies the best_idx that search produced at step N-1 for apply's
//     state at step N-1 — which is the same state apply currently holds
//     (apply only advances when it applies a move).
//
// State-history mode takes priority if both are configured.
//
// Warm fields (must be set before Configure):
//   - Env: the typed Environment[S, A] to advance.
//   - Decoder, Encoder: codec for the game state.
//
// Optional warm field:
//   - BestIdxSlot: row offset of the best-action index within the
//     upstream partition's row when using state-history mode.
type ApplyIteration[S any, A any] struct {
	Env          Environment[S, A]
	Decoder      func([]float64) (S, error)
	Encoder      func(S) []float64
	BestIdxSlot  int

	cur         S
	lastWritten []float64
}

// Param key for direct (within-step) best-action input. The value should
// be a 1-element slice — the legal-action index. A negative value (e.g.
// the -1 sentinel) leaves the state unchanged.
const ApplyParamBestIdx = "best_legal_idx"

// Param key for state-history (lag-1) best-action input. The value (set
// via ParamsAsPartitions) is a 1-element slice containing the partition
// index of the upstream partition whose row[BestIdxSlot] holds the
// best-action index.
const ApplyParamBestIdxPartition = "best_idx_partition"

// Configure implements simulator.Iteration.
func (m *ApplyIteration[S, A]) Configure(partitionIndex int, settings *simulator.Settings) {
	if m.Env == nil {
		panic("agents.ApplyIteration: Env required")
	}
	if m.Decoder == nil {
		panic("agents.ApplyIteration: Decoder required")
	}
	if m.Encoder == nil {
		panic("agents.ApplyIteration: Encoder required")
	}
	is := settings.Iterations[partitionIndex]
	cur, err := m.Decoder(is.InitStateValues)
	if err != nil {
		panic("agents.ApplyIteration: decoder failed: " + err.Error())
	}
	m.cur = cur
	enc := m.Encoder(cur)
	m.lastWritten = make([]float64, len(enc))
	copy(m.lastWritten, enc)
}

// Iterate implements simulator.Iteration.
func (m *ApplyIteration[S, A]) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	_ = timestepsHistory
	sh := stateHistories[partitionIndex]
	row := sh.CopyStateRow(0)
	if !floatsEqual(row, m.lastWritten) {
		decoded, err := m.Decoder(row)
		if err != nil {
			return row
		}
		m.cur = decoded
	}
	if _, done := m.Env.Terminal(m.cur); done {
		return row
	}
	bestIdx := -1
	// State-history mode takes priority if both keys are present.
	if upstreamSlice, ok := params.GetOk(ApplyParamBestIdxPartition); ok && len(upstreamSlice) > 0 {
		upstreamIdx := int(upstreamSlice[0])
		if upstreamIdx >= 0 && upstreamIdx < len(stateHistories) {
			upstreamRow := stateHistories[upstreamIdx].Values.RawRowView(0)
			if m.BestIdxSlot >= 0 && m.BestIdxSlot < len(upstreamRow) {
				bestIdx = int(upstreamRow[m.BestIdxSlot])
			}
		}
	} else if idxSlice, ok := params.GetOk(ApplyParamBestIdx); ok && len(idxSlice) > 0 {
		bestIdx = int(idxSlice[0])
	}
	if bestIdx < 0 {
		return row
	}
	leg := m.Env.Legal(m.cur)
	if bestIdx >= len(leg) {
		return row
	}
	ns, err := m.Env.Apply(m.cur, leg[bestIdx])
	if err != nil {
		return row
	}
	out := m.Encoder(ns)
	if cap(m.lastWritten) < len(out) {
		m.lastWritten = make([]float64, len(out))
	} else {
		m.lastWritten = m.lastWritten[:len(out)]
	}
	copy(m.lastWritten, out)
	m.cur = ns
	return out
}

// floatsEqual reports whether a and b are bitwise-identical. Used by
// ApplyIteration to detect when the row has been overwritten by another
// partition since the last Iterate so the cached typed state can be
// invalidated.
func floatsEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
