package analysis

import (
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// DataRef is a reference to some subset of the stored data.
type DataRef struct {
	PartitionName string
	ValueIndices  []int
	IsTime        bool
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
	if d.IsTime {
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
	if d.IsTime {
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
	return outValues
}
