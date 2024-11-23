package analysis

import (
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// DataRef is a reference to some subset of the stored data.
type DataRef struct {
	PartitionName string
	ValueIndex    int
	IsTime        bool
}

// GetSeriesName retrieves a unique name for labelling plots.
func (d *DataRef) GetSeriesName() string {
	if d.IsTime {
		return "time"
	}
	return d.PartitionName + " " + strconv.Itoa(d.ValueIndex)
}

// GetFromStorage retrieves the relevant data from storage that
// the reference is pointing to.
func (d *DataRef) GetFromStorage(
	storage *simulator.StateTimeStorage,
) []float64 {
	var plotValues []float64
	if d.IsTime {
		plotValues = storage.GetTimes()
	} else {
		values := make([]float64, 0)
		for _, vs := range storage.GetValues(d.PartitionName) {
			values = append(values, vs[d.ValueIndex])
		}
		plotValues = values
	}
	return plotValues
}
