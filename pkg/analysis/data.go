package analysis

import (
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// IndexRange represents an inclusive-exclusive [Lower, Upper) span of
// indices. It is commonly used to clip time-series windows for plotting.
type IndexRange struct {
	Lower int
	Upper int
}

// DataPlotting declares optional transformations for plotting, such as
// treating a reference as time and restricting to a time index range.
type DataPlotting struct {
	IsTime    bool
	TimeRange *IndexRange
}

// DataRef identifies a subset of data stored in StateTimeStorage. It can
// reference the special time axis or one or more value indices of a
// partition. Optional plotting hints may be supplied via Plotting.
type DataRef struct {
	PartitionName string
	ValueIndices  []int
	Plotting      *DataPlotting
}

// isTime reports whether the reference targets the simulation time axis.
func (d *DataRef) isTime() bool {
	if d.Plotting != nil {
		return d.Plotting.IsTime
	}
	return false
}

// isOutsideTimeRange reports whether a time index falls outside the
// configured plotting range, if any.
func (d *DataRef) isOutsideTimeRange(index int) bool {
	if d.Plotting != nil {
		if d.Plotting.TimeRange != nil {
			if index < d.Plotting.TimeRange.Lower ||
				index >= d.Plotting.TimeRange.Upper {
				return true
			}
		}
	}
	return false
}

// applyTimeRange slices all provided series to the configured plotting
// time window, if set.
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

// GetValueIndices returns the referenced value indices, defaulting to
// all indices within the partition when ValueIndices is nil.
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

// GetSeriesNames returns human-readable series labels for plotting.
// Time references are labeled "time"; value references are labeled as
// "<partition> <index>".
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

// GetTimeIndexFromStorage returns the data at a specific time index. For a
// time reference, this is a single-element slice containing the time value;
// for a value reference, this is the row slice for that time index.
func (d *DataRef) GetTimeIndexFromStorage(
	storage *simulator.StateTimeStorage,
	timeIndex int,
) []float64 {
	var outValues []float64
	if d.isTime() {
		outValues = []float64{storage.GetTimes()[timeIndex]}
	} else {
		outValues = storage.GetValues(d.PartitionName)[timeIndex]
	}
	if d.isOutsideTimeRange(timeIndex) {
		panic("requested index " + strconv.Itoa(timeIndex) +
			" is outside of configured time range")
	}
	return outValues
}

// GetFromStorage returns the entire referenced series. For a time
// reference, this is a single series containing all times; for a value
// reference, this is one series per value index.
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
	return outValues
}
