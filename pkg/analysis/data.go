package analysis

import (
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// IndexRange holds upper and lower index values for a range
// of indices selected to be used.
type IndexRange struct {
	Lower int
	Upper int
}

// DataPlotting configures the changes to the data which can
// be applied before plotting.
type DataPlotting struct {
	IsTime    bool
	TimeRange *IndexRange
	Transform func(values []float64) []float64
}

// DataRef is a reference to some subset of the stored data.
type DataRef struct {
	PartitionName string
	ValueIndices  []int
	Plotting      *DataPlotting
}

// isTime is a convenience method for finding if the data has
// been configured to the time variable or not.
func (d *DataRef) isTime() bool {
	if d.Plotting != nil {
		return d.Plotting.IsTime
	}
	return false
}

// applyTimeRange applies a time range restriction to the data
// if it has been configured.
func (d *DataRef) applyTimeRange(outValues [][]float64) {
	if d.Plotting != nil {
		if d.Plotting.TimeRange != nil {
			for i, values := range outValues {
				outValues[i] =
					values[d.Plotting.TimeRange.Lower:d.Plotting.TimeRange.Upper]
			}
		}
	}
}

// applyTransform applies a transformation to the data if it
// has been configured.
func (d *DataRef) applyTransform(outValues [][]float64) {
	if d.Plotting != nil {
		if d.Plotting.Transform != nil {
			for i, values := range outValues {
				outValues[i] = d.Plotting.Transform(values)
			}
		}
	}
}

// GetValueIndices populates the value indices slice with all
// of the indices found in the referenced partition if set
// initially to nil.
func (d *DataRef) GetValueIndices(
	storage *simulator.StateTimeStorage,
) []int {
	if d.ValueIndices == nil {
		d.ValueIndices = make([]int, 0)
		for i := range storage.GetValues(d.PartitionName)[0] {
			d.ValueIndices = append(d.ValueIndices, i)
		}
	}
	return d.ValueIndices
}

// GetSeriesName retrieves unique names for each dimension in the
// time series data that is typically used for labelling plots.
func (d *DataRef) GetSeriesNames(
	storage *simulator.StateTimeStorage,
) []string {
	if d.isTime() {
		return []string{"time"}
	}
	names := make([]string, 0)
	for _, index := range d.GetValueIndices(storage) {
		names = append(names, d.PartitionName+" "+strconv.Itoa(index))
	}
	return names
}

// GetFromStorage retrieves the relevant data from storage that
// the reference is pointing to.
func (d *DataRef) GetFromStorage(
	storage *simulator.StateTimeStorage,
) [][]float64 {
	var outValues [][]float64
	if d.isTime() {
		outValues = [][]float64{storage.GetTimes()}
	} else {
		outValues = make([][]float64, 0)
		for _, index := range d.GetValueIndices(storage) {
			values := make([]float64, 0)
			for _, vs := range storage.GetValues(d.PartitionName) {
				values = append(values, vs[index])
			}
			outValues = append(outValues, values)
		}
	}
	d.applyTimeRange(outValues)
	d.applyTransform(outValues)
	return outValues
}
