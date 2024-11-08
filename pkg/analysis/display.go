package analysis

import (
	"strconv"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// PlotDataRef
type PlotDataRef struct {
	PartitionName string
	ValueIndex    int
	IsTime        bool
}

// GetSeriesName
func (p *PlotDataRef) GetSeriesName() string {
	return p.PartitionName + " " + strconv.Itoa(p.ValueIndex)
}

// GetFromStorage
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

// ScatterPlotConfig
type ScatterPlotConfig struct {
	XRefs []PlotDataRef
	YRefs []PlotDataRef
}

// NewScatterPlotFromPartition
func NewScatterPlotFromPartition(
	storage *simulator.StateTimeStorage,
	config *ScatterPlotConfig,
) *charts.Scatter {
	if len(config.XRefs) != 1 {
		panic(strconv.Itoa(len(config.XRefs)) +
			" X-axes have been been provided")
	}
	if len(config.YRefs) == 0 {
		panic("0 Y-axes have been been provided")
	}
	yPartitions := make(map[string][][]float64, 0)
	for _, yData := range config.YRefs {
		_, ok := yPartitions[yData.PartitionName]
		if !ok {
			yPartitions[yData.PartitionName] =
				storage.GetValues(yData.PartitionName)
		}
	}
	scatter := charts.NewScatter()
	xValues := config.XRefs[0].GetFromStorage(storage)
	for _, yData := range config.YRefs {
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
