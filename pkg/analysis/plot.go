package analysis

import (
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// NewScatterPlotFromPartition creates a new scatter plot from
// the storage data given the axes references to subsets of it.
func NewScatterPlotFromPartition(
	storage *simulator.StateTimeStorage,
	xRef DataRef,
	yRefs []DataRef,
) *charts.Scatter {
	if len(yRefs) == 0 {
		panic("0 Y-axes have been been provided")
	}
	yPartitions := make(map[string][][]float64, 0)
	for _, yData := range yRefs {
		_, ok := yPartitions[yData.PartitionName]
		if !ok {
			yPartitions[yData.PartitionName] =
				storage.GetValues(yData.PartitionName)
		}
	}
	scatter := charts.NewScatter()
	scatter.SetGlobalOptions(
		charts.WithXAxisOpts(opts.XAxis{
			Name: xRef.GetSeriesNames(storage)[0],
		}),
		charts.WithTooltipOpts(opts.Tooltip{
			Trigger:   "item",
			Formatter: "({c})",
		}),
	)
	xValues := xRef.GetFromStorage(storage)[0]
	for _, yData := range yRefs {
		yNames := yData.GetSeriesNames(storage)
		for i, yValues := range yData.GetFromStorage(storage) {
			plotData := make([]opts.ScatterData, 0)
			for j, yYalue := range yValues {
				plotData = append(plotData, opts.ScatterData{
					Value: []interface{}{xValues[j], yYalue},
				})
			}
			scatter.AddSeries(yNames[i], plotData)
		}
	}
	return scatter
}

// SymFillLineRef
type SymFillLineRef struct {
	Variance DataRef
}

// AsymFillLineRef
type AsymFillLineRef struct {
	Upper DataRef
	Lower DataRef
}

// FillLineRef
type FillLineRef struct {
	Sym  SymFillLineRef
	Asym AsymFillLineRef
}

// NewLinePlotFromPartition creates a new line plot from
// the storage data given the axes references to subsets of it.
func NewLinePlotFromPartition(
	storage *simulator.StateTimeStorage,
	xRef DataRef,
	yRefs []DataRef,
	fillYRefs []FillLineRef,
) *charts.Line {
	if len(yRefs) == 0 {
		panic("0 Y-axes have been been provided")
	}
	yPartitions := make(map[string][][]float64, 0)
	for _, yData := range yRefs {
		_, ok := yPartitions[yData.PartitionName]
		if !ok {
			yPartitions[yData.PartitionName] =
				storage.GetValues(yData.PartitionName)
		}
	}
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithXAxisOpts(opts.XAxis{
			Name: xRef.GetSeriesNames(storage)[0],
		}),
		charts.WithTooltipOpts(opts.Tooltip{
			Trigger:   "item",
			Formatter: "({c})",
		}),
	)
	xValues := xRef.GetFromStorage(storage)[0]
	for _, yData := range yRefs {
		yNames := yData.GetSeriesNames(storage)
		for i, yValues := range yData.GetFromStorage(storage) {
			plotData := make([]opts.LineData, 0)
			for j, yYalue := range yValues {
				plotData = append(plotData, opts.LineData{
					Value: []interface{}{xValues[j], yYalue},
				})
			}
			line.AddSeries(yNames[i], plotData)
		}
	}
	return line
}
