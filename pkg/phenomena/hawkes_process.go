package phenomena

import (
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// HawkesProcessExcitingKernel defines an interface that must be implemented
// for an exciting kernel of the Hawkes process.
type HawkesProcessExcitingKernel interface {
	Evaluate(
		params *simulator.OtherParams,
		currentTime float64,
		somePreviousTime float64,
		stateElement int,
	) float64
}

// HawkesProcessIntensityIteration an iteration for a Hawkes process
// self-exciting intensity function.
type HawkesProcessIntensityIteration struct {
	excitingKernel       HawkesProcessExcitingKernel
	hawkesPartitionIndex int
}

func (h *HawkesProcessIntensityIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	h.hawkesPartitionIndex = int(
		settings.OtherParams[partitionIndex].
			IntParams["hawkes_partition_index"][0],
	)
}

func (h *HawkesProcessIntensityIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	hawkesHistory := stateHistories[h.hawkesPartitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		values[i] = params.FloatParams["background_rates"][i]
		for j := 1; j < hawkesHistory.StateHistoryDepth; j++ {
			values[i] += (hawkesHistory.Values.At(j, i) -
				hawkesHistory.Values.At(j-1, i)) * h.excitingKernel.Evaluate(
				params,
				timestepsHistory.Values.AtVec(j),
				timestepsHistory.Values.AtVec(j-1),
				i,
			)
		}
	}
	return values
}

// NewHawkesProcessIntensityIteration creates a new
// HawkesProcessIntensityIteration given a partition index
// for the Hawkes process itself.
func NewHawkesProcessIntensityIteration(
	excitingKernel HawkesProcessExcitingKernel,
	hawkesPartitionIndex int,
) *HawkesProcessIntensityIteration {
	return &HawkesProcessIntensityIteration{
		excitingKernel:       excitingKernel,
		hawkesPartitionIndex: hawkesPartitionIndex,
	}
}

// HawkesProcessIteration defines an iteration for a Hawkes process.
type HawkesProcessIteration struct {
	unitUniformDist         *distuv.Uniform
	intensityPartitionIndex int
}

func (h *HawkesProcessIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	h.unitUniformDist = &distuv.Uniform{
		Min: 0.0,
		Max: 1.0,
		Src: rand.NewSource(settings.Seeds[partitionIndex]),
	}
	h.intensityPartitionIndex = int(
		settings.OtherParams[partitionIndex].
			IntParams["intensity_partition_index"][0],
	)
}

func (h *HawkesProcessIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	rates := params.FloatParams["partition_"+strconv.Itoa(h.intensityPartitionIndex)]
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
