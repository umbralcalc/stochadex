package observations

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// DynamicMaskStateObservationIteration uses non-zeros in the masking partition to
// only observe a masked subset of the state values, replacing masked values with
// specified NaN value.
type DynamicMaskStateObservationIteration struct {
	nanValue float64
}

func (d *DynamicMaskStateObservationIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	d.nanValue = settings.Params[partitionIndex]["nan_value"][0]
}

func (d *DynamicMaskStateObservationIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	outputValues := make([]float64, 0)
	maskValues := params["mask_values"]
	stateValues := params["values_to_observe"]
	for i, maskValue := range maskValues {
		if maskValue == 0 {
			outputValues = append(outputValues, d.nanValue)
			continue
		}
		outputValues = append(outputValues, stateValues[i])
	}
	return outputValues
}
