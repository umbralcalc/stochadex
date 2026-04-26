package agents

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// MASTAggregationPartition is a stochadex iteration that maintains
// running (count, sum) pairs per action-key index for MAST (Move-Average
// Sampling Technique). Each step it reads a variable-length update
// batch from an upstream rollout partition via params_from_upstream and
// applies the (key_idx, reward) increments to its row.
//
// Row layout (width = 2 * MaxKeys):
//
//	row[2*k]     count for key k
//	row[2*k+1]   sum   for key k
//
// Use MASTAggregationRowWidth(K) to compute state_width.
//
// # Update batch format
//
// The upstream batch is a single []float64 with the layout
//
//	[num_updates, key_idx_0, reward_0, key_idx_1, reward_1, ...]
//
// where num_updates is the number of valid (key_idx, reward) pairs in
// the slice. MASTRolloutPartition emits exactly this layout in its
// row's path-suffix slots; wire MASTAggregationPartition's
// params_from_upstream (key MASTAggregationParamUpdates) to those slots.
//
// Out-of-range key indices and updates beyond the slice's declared
// num_updates are silently dropped.
//
// # Read access
//
// Downstream samplers (e.g. MASTRolloutPartition) read the aggregates
// via state-history mode (lag-1) using params_as_partitions. See
// MASTAggregationParamPartition for the canonical key.
type MASTAggregationPartition[A any] struct {
	MaxKeys int

	counts []float64
	sums   []float64
}

// MASTAggregationParamUpdates is the params_from_upstream key used to
// read the variable-length update batch from an upstream rollout
// partition.
const MASTAggregationParamUpdates = "mast_updates"

// MASTAggregationParamPartition is the params_as_partitions key used by
// downstream samplers to learn this partition's index for state-history
// reads. The value is a 1-element slice containing the partition index.
const MASTAggregationParamPartition = "mast_aggregates_partition"

// MASTAggregationRowWidth returns the required state_width for an
// MASTAggregationPartition with the given key bound.
func MASTAggregationRowWidth(maxKeys int) int { return 2 * maxKeys }

// MASTAggregationCountSlot returns the row offset of the count for key k.
func MASTAggregationCountSlot(k int) int { return 2 * k }

// MASTAggregationSumSlot returns the row offset of the sum for key k.
func MASTAggregationSumSlot(k int) int { return 2*k + 1 }

// Configure implements simulator.Iteration.
func (m *MASTAggregationPartition[A]) Configure(partitionIndex int, settings *simulator.Settings) {
	if m.MaxKeys <= 0 {
		panic("agents.MASTAggregationPartition: MaxKeys must be > 0")
	}
	is := settings.Iterations[partitionIndex]
	if is.StateWidth != MASTAggregationRowWidth(m.MaxKeys) {
		panic("agents.MASTAggregationPartition: StateWidth must equal MASTAggregationRowWidth(MaxKeys)")
	}
	m.counts = make([]float64, m.MaxKeys)
	m.sums = make([]float64, m.MaxKeys)
	// Seed counts/sums from any non-zero init values so the harness
	// re-run produces identical outputs (no statefulness residue).
	for k := 0; k < m.MaxKeys; k++ {
		m.counts[k] = is.InitStateValues[MASTAggregationCountSlot(k)]
		m.sums[k] = is.InitStateValues[MASTAggregationSumSlot(k)]
	}
}

// Iterate implements simulator.Iteration.
func (m *MASTAggregationPartition[A]) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	_ = stateHistories
	_ = timestepsHistory
	if updates, ok := params.GetOk(MASTAggregationParamUpdates); ok && len(updates) >= 1 {
		num := int(updates[0])
		for i := 0; i < num; i++ {
			off := 1 + 2*i
			if off+1 >= len(updates) {
				break
			}
			k := int(updates[off])
			r := updates[off+1]
			if k < 0 || k >= m.MaxKeys {
				continue
			}
			m.counts[k]++
			m.sums[k] += r
		}
	}
	row := make([]float64, MASTAggregationRowWidth(m.MaxKeys))
	for k := 0; k < m.MaxKeys; k++ {
		row[MASTAggregationCountSlot(k)] = m.counts[k]
		row[MASTAggregationSumSlot(k)] = m.sums[k]
	}
	return row
}

// MASTMeanForKey reads the running mean reward for key k from a row in
// the MASTAggregationPartition's layout. Returns (0, 0) when the key
// has not been observed. Used by samplers that have read the partition's
// row via params_as_partitions.
func MASTMeanForKey(row []float64, k int) (mean float64, count int) {
	if k < 0 || 2*k+1 >= len(row) {
		return 0, 0
	}
	c := row[MASTAggregationCountSlot(k)]
	if c <= 0 {
		return 0, 0
	}
	return row[MASTAggregationSumSlot(k)] / c, int(c)
}
