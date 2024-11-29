package discrete

import (
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/stat/distuv"
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
	values := params.Get("background_rates")
	for i := 1; i < hawkesHistory.StateHistoryDepth; i++ {
		sumValues := hawkesHistory.Values.RawRowView(i - 1)
		floats.Sub(sumValues, hawkesHistory.Values.RawRowView(i))
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
	unitUniformDist *distuv.Uniform
}

func (h *HawkesProcessIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	h.unitUniformDist = &distuv.Uniform{
		Min: 0.0,
		Max: 1.0,
		Src: rand.NewSource(settings.Iterations[partitionIndex].Seed),
	}
}

func (h *HawkesProcessIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	rates := params.Get("intensity")
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		if rates[i] > (rates[i]+
			(1.0/timestepsHistory.NextIncrement))*h.unitUniformDist.Rand() {
			values[i] = stateHistory.Values.At(0, i) + 1.0
		} else {
			values[i] = stateHistory.Values.At(0, i)
		}
	}
	return values
}
