package discrete

import (
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// HawkesProcessIntensityIteration an iteration for a Hawkes process
// self-exciting intensity function.
type HawkesProcessIntensityIteration struct {
	excitingKernel       kernels.IntegrationKernel
	hawkesPartitionIndex int
}

func (h *HawkesProcessIntensityIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	h.excitingKernel.Configure(partitionIndex, settings)
	h.hawkesPartitionIndex = int(
		settings.Iterations[partitionIndex].Params.GetIndex(
			"hawkes_partition_index", 0),
	)
}

func (h *HawkesProcessIntensityIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	h.excitingKernel.SetParams(params)
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
			h.excitingKernel.Evaluate(
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

// NewHawkesProcessIntensityIteration creates a new
// HawkesProcessIntensityIteration given a partition index
// for the Hawkes process itself.
func NewHawkesProcessIntensityIteration(
	excitingKernel kernels.IntegrationKernel,
	hawkesPartitionIndex int,
) *HawkesProcessIntensityIteration {
	return &HawkesProcessIntensityIteration{
		excitingKernel:       excitingKernel,
		hawkesPartitionIndex: hawkesPartitionIndex,
	}
}

// HawkesProcessIteration defines an iteration for a Hawkes process.
type HawkesProcessIteration struct {
	sampler *rng.Sampler
}

func (h *HawkesProcessIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	h.sampler = rng.New(settings.Iterations[partitionIndex].Seed)
}

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
