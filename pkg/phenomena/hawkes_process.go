package phenomena

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// HawkesProcessExcitingKernel defines an interface that must be implemented
// for an exciting kernel of the Hawkes process.
type HawkesProcessExcitingKernel interface {
	Evaluate(
		otherParams *simulator.OtherParams,
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

func (h *HawkesProcessIntensityIteration) Iterate(
	otherParams *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	hawkesHistory := stateHistories[h.hawkesPartitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		values[i] = otherParams.FloatParams["background_rates"][i]
		for j := 1; j < hawkesHistory.StateHistoryDepth; j++ {
			values[i] += (hawkesHistory.Values.At(j, i) -
				hawkesHistory.Values.At(j-1, i)) * h.excitingKernel.Evaluate(
				otherParams,
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

func (h *HawkesProcessIteration) Iterate(
	otherParams *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	rateHistory := stateHistories[h.intensityPartitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		if rateHistory.Values.At(0, i) > (rateHistory.Values.At(0, i)+
			(1.0/timestepsHistory.NextIncrement))*h.unitUniformDist.Rand() {
			values[i] = stateHistory.Values.At(0, i) + 1.0
		} else {
			values[i] = stateHistory.Values.At(0, i)
		}
	}
	return values
}

// NewHawkesProcessIteration creates a new HawkesProcessIteration given a
// seed and a partition index for the rate process.
func NewHawkesProcessIteration(
	seed uint64,
	intensityPartitionIndex int,
) *HawkesProcessIteration {
	return &HawkesProcessIteration{
		unitUniformDist: &distuv.Uniform{
			Min: 0.0,
			Max: 1.0,
			Src: rand.NewSource(seed),
		},
		intensityPartitionIndex: intensityPartitionIndex,
	}
}
