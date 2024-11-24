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
	XRef DataRef,
	YRefs []DataRef,
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
	scatter.SetGlobalOptions(
		charts.WithXAxisOpts(opts.XAxis{
			Name: XRef.GetSeriesNames(storage)[0],
		}),
		charts.WithTooltipOpts(opts.Tooltip{
			Trigger:   "item",
			Formatter: "({c})",
		}),
	)
	xValues := XRef.GetFromStorage(storage)[0]
	for _, yData := range YRefs {
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
