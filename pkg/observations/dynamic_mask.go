package observations

import (
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// DynamicMaskStateObservationIteration uses non-zeros in the masking partition to
// only observe a masked subset of the state values, replacing masked values with
// specified NaN value.
type DynamicMaskStateObservationIteration struct {
	nanValue           float64
	partitionToObserve int
	maskPartition      int
}

func (d *DynamicMaskStateObservationIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	d.nanValue = settings.OtherParams[partitionIndex].
		FloatParams["nan_value"][0]
	d.maskPartition = int(settings.OtherParams[partitionIndex].
		IntParams["mask_partition"][0])
	d.partitionToObserve = int(settings.OtherParams[partitionIndex].
		IntParams["partition_to_observe"][0])
}

func (d *DynamicMaskStateObservationIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	outputValues := make([]float64, 0)
	maskValues := params.FloatParams["partition_"+strconv.Itoa(d.maskPartition)]
	stateValues := params.FloatParams["partition_"+strconv.Itoa(d.partitionToObserve)]
	for i, maskValue := range maskValues {
		if maskValue == 0 {
			outputValues = append(outputValues, d.nanValue)
			continue
		}
		outputValues = append(outputValues, stateValues[i])
	}
	return outputValues
}
