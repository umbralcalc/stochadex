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

// GetSeriesName retrieves a unique names for labelling plots.
func (d *DataRef) GetSeriesNames() []string {
	if d.IsTime {
		return []string{"time"}
	}
	names := make([]string, 0)
	for _, index := range d.ValueIndices {
		names = append(names, d.PartitionName+" "+strconv.Itoa(index))
	}
	return names
}

// GetFromStorage retrieves the relevant data from storage that
// the reference is pointing to.
func (d *DataRef) GetFromStorage(
	storage *simulator.StateTimeStorage,
) [][]float64 {
	var plotValues [][]float64
	if d.IsTime {
		plotValues = [][]float64{storage.GetTimes()}
	} else {
		plotValues = make([][]float64, 0)
		for _, index := range d.ValueIndices {
			values := make([]float64, 0)
			for _, vs := range storage.GetValues(d.PartitionName) {
				values = append(values, vs[index])
			}
			plotValues = append(plotValues, values)
		}
	}
	return plotValues
}
