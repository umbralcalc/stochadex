package discrete

import (
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// HawkesProcessIntensityIteration an iteration for a Hawkes process
// self-exciting intensity function.
//
// Usage hints:
//   - Provide: "background_rates" and "hawkes_partition_index" (the partition
//     running the HawkesProcessIteration whose history excites this intensity).
//   - Set ExcitingKernel to the kernel weighting past events by their age.
//
// Update:
//
//	x[i] <- background_rates[i] + sum over k = 2..D of
//	        w(H@1, H@k, t, t@k) * (H@(k-1)[i] - H@k[i])
//	H@k is hawkes_partition_index@k, the counting partition's state k steps back
//	(its index is read once, at Configure). D is that partition's state history
//	depth, t@k the time of its state k steps back (t@1 = t), and w(...) the kernel,
//	which weights each step's count increment by the whole state vectors and times.
type HawkesProcessIntensityIteration struct {
	ExcitingKernel       kernels.IntegrationKernel
	hawkesPartitionIndex int
}

// Configure implements simulator.Iteration.
func (h *HawkesProcessIntensityIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	h.ExcitingKernel.Configure(partitionIndex, settings)
	h.hawkesPartitionIndex = int(
		settings.Iterations[partitionIndex].Params.GetIndex(
			"hawkes_partition_index", 0),
	)
}

// Iterate implements simulator.Iteration.
func (h *HawkesProcessIntensityIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	h.ExcitingKernel.SetParams(params)
	hawkesHistory := stateHistories[h.hawkesPartitionIndex]
	values := params.GetCopy("background_rates")
	for i := 1; i < hawkesHistory.StateHistoryDepth; i++ {
		sumValues := make([]float64, stateHistories[partitionIndex].StateWidth)
		floats.SubTo(
			sumValues,
			hawkesHistory.Values.RawRowView(i-1),
			hawkesHistory.Values.RawRowView(i),
		)
		floats.Scale(
			h.ExcitingKernel.Evaluate(
				hawkesHistory.Values.RawRowView(0),
				hawkesHistory.Values.RawRowView(i),
				timestepsHistory.Values.AtVec(0),
				timestepsHistory.Values.AtVec(i),
			),
			sumValues,
		)
		floats.Add(values, sumValues)
	}
	return values
}

// HawkesProcessIteration defines an iteration for a Hawkes process.
//
// Update:
//
//	x[i] <- x[i] + Bernoulli(intensity[i] / (intensity[i] + 1/dt))
type HawkesProcessIteration struct {
	sampler *rng.Sampler
}

// Configure implements simulator.Iteration.
func (h *HawkesProcessIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	h.sampler = rng.New(settings.Iterations[partitionIndex].Seed)
}

// Iterate implements simulator.Iteration.
func (h *HawkesProcessIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	rates := params.Get("intensity")
	values := stateHistory.GetNextStateRowToUpdate()
	for i := range values {
		if rates[i] > (rates[i]+
			(1.0/timestepsHistory.NextIncrement))*h.sampler.Float64() {
			values[i] += 1.0
		}
	}
	return values
}
