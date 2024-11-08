package analysis

import (
	"strconv"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// PlotDataRef is a reference to some subset of the stored data
// for plotting purposes.
type PlotDataRef struct {
	PartitionName string
	ValueIndex    int
	IsTime        bool
}

// GetSeriesName retrieves a unique name for labelling plots.
func (p *PlotDataRef) GetSeriesName() string {
	if p.IsTime {
		return "time"
	}
	return p.PartitionName + " " + strconv.Itoa(p.ValueIndex)
}

// GetFromStorage retrieves the relevant data from storage that
// the reference is pointing to.
func (p *PlotDataRef) GetFromStorage(
	storage *simulator.StateTimeStorage,
) []float64 {
	var plotValues []float64
	if p.IsTime {
		plotValues = storage.GetTimes()
	} else {
		values := make([]float64, 0)
		for _, vs := range storage.GetValues(p.PartitionName) {
			values = append(values, vs[p.ValueIndex])
		}
		plotValues = values
	}
	return plotValues
}

// NewScatterPlotFromPartition creates a new scatter plot from
// the storage data given the axes references to subsets of it.
func NewScatterPlotFromPartition(
	storage *simulator.StateTimeStorage,
	XRef PlotDataRef,
	YRefs []PlotDataRef,
) *charts.Scatter {
	if len(YRefs) == 0 {
		panic("0 Y-axes have been been provided")
	}
	yPartitions := make(map[string][][]float64, 0)
	for _, yData := range YRefs {
		_, ok := yPartitions[yData.PartitionName]
		if !ok {
			yPartitions[yData.PartitionName] =
				storage.GetValues(yData.PartitionName)
		}
	}
	scatter := charts.NewScatter()
	scatter.SetGlobalOptions(charts.WithXAxisOpts(opts.XAxis{
		Name: XRef.GetSeriesName(),
	}))
	xValues := XRef.GetFromStorage(storage)
	for _, yData := range YRefs {
		plotData := make([]opts.ScatterData, 0)
		for i, yYalue := range yData.GetFromStorage(storage) {
			plotData = append(plotData, opts.ScatterData{
				Value: []interface{}{xValues[i], yYalue},
			})
		}
		scatter.AddSeries(yData.GetSeriesName(), plotData)
	}
	return scatter
}
