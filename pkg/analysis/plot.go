package analysis

import (
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-gota/gota/dataframe"
	"github.com/go-gota/gota/series"
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

// NewScatterPlotFromDataFrame creates a new scatter plot from
// the dataframe given the axes references to subsets of it.
func NewScatterPlotFromDataFrame(
	df *dataframe.DataFrame,
	xAxis string,
	yAxis string,
	groupBy ...string,
) *charts.Scatter {
	scatter := charts.NewScatter()
	scatter.SetGlobalOptions(
		charts.WithXAxisOpts(opts.XAxis{Name: xAxis}),
		charts.WithTooltipOpts(opts.Tooltip{
			Trigger:   "item",
			Formatter: "({c})",
		}),
	)
	groupCol := ""
	if len(groupBy) > 0 {
		groupCol = groupBy[0]
	}
	if groupCol != "" {
		// Automatically get unique group values
		groups := df.Col(groupCol).Records()
		uniqueGroups := make(map[string]bool)
		for _, g := range groups {
			uniqueGroups[g] = true
		}
		for group := range uniqueGroups {
			points := make([]opts.ScatterData, 0)
			fdf := df.Filter(dataframe.F{
				Colname:    groupCol,
				Comparator: series.Eq,
				Comparando: group,
			})
			xSeries := fdf.Col(xAxis)
			ySeries := fdf.Col(yAxis)
			for i := 0; i < xSeries.Len(); i++ {
				points = append(points, opts.ScatterData{
					Value: []interface{}{
						xSeries.Elem(i).Val(),
						ySeries.Elem(i).Val(),
					},
				})
			}
			scatter.AddSeries(group, points)
		}
	} else {
		// No grouping, single series
		points := make([]opts.ScatterData, 0)
		xSeries := df.Col(xAxis)
		ySeries := df.Col(yAxis)
		for i := 0; i < xSeries.Len(); i++ {
			points = append(points, opts.ScatterData{
				Value: []interface{}{
					xSeries.Elem(i).Val(),
					ySeries.Elem(i).Val(),
				},
			})
		}
		scatter.AddSeries(yAxis, points)
	}
	return scatter
}

// FillLineRef holds the data required to construct a filled
// region in the line plot.
type FillLineRef struct {
	Upper DataRef
	Lower DataRef
}

// echartsColours matches the ECharts default colour palette.
var echartsColours = []string{
	"#5470C6", // Blue
	"#91CC75", // Green
	"#FAC858", // Yellow
	"#EE6666", // Red
	"#73C0DE", // Light Blue
	"#3BA272", // Dark Green
	"#FC8452", // Orange
	"#9A60B4", // Purple
	"#EA7CCC", // Pink
}

// ColourGenerator keeps track of the current colour index.
type ColourGenerator struct {
	index int
}

// Next returns the next colour in the ECharts palette,
// cycling back if needed.
func (cg *ColourGenerator) Next() string {
	colour := echartsColours[cg.index]
	cg.index = (cg.index + 1) % len(echartsColours)
	return colour
}

// appendFilledRegionToLinePlot uses the fillYRef data references
// to create filled a region in the referenced line plot.
func appendFilledRegionToLinePlot(
	storage *simulator.StateTimeStorage,
	fillYRef FillLineRef,
	xValues []float64,
	line *charts.Line,
	names []string,
	colours []string,
) {
	lowerValuesArr := fillYRef.Lower.GetFromStorage(storage)
	for i, upperValues := range fillYRef.Upper.GetFromStorage(storage) {
		confLowerData := make([]opts.LineData, 0)
		confUpperData := make([]opts.LineData, 0)
		lowerValues := lowerValuesArr[i]
		for j, upperValue := range upperValues {
			confLowerData = append(confLowerData, opts.LineData{
				Value: []interface{}{xValues[j], upperValue},
			})
			confUpperData = append(confUpperData, opts.LineData{
				Value: []interface{}{xValues[j], lowerValues[j]},
			})
		}
		line.AddSeries(names[i], confUpperData,
			charts.WithItemStyleOpts(opts.ItemStyle{Color: colours[i]}),
			charts.WithSeriesOpts(func(s *charts.SingleSeries) {
				s.ShowSymbol = opts.Bool(false)
			}),
		)
		line.AddSeries(names[i], confLowerData,
			charts.WithItemStyleOpts(opts.ItemStyle{Color: colours[i]}),
			charts.WithSeriesOpts(func(s *charts.SingleSeries) {
				s.ShowSymbol = opts.Bool(false)
			}),
		)
	}
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
	colourGen := &ColourGenerator{}
	for refIndex, yData := range yRefs {
		yNames := yData.GetSeriesNames(storage)
		colours := make([]string, 0)
		for i, yValues := range yData.GetFromStorage(storage) {
			colour := colourGen.Next()
			colours = append(colours, colour)
			plotData := make([]opts.LineData, 0)
			for j, yYalue := range yValues {
				plotData = append(plotData, opts.LineData{
					Value: []interface{}{xValues[j], yYalue},
				})
			}
			line.AddSeries(yNames[i], plotData,
				charts.WithItemStyleOpts(opts.ItemStyle{Color: colour}),
			)
		}
		if fillYRefs != nil {
			appendFilledRegionToLinePlot(
				storage,
				fillYRefs[refIndex],
				xValues,
				line,
				yNames,
				colours,
			)
		}
	}
	return line
}

// NewLinePlotFromDataFrame creates a new line plot from
// the dataframe given the axes references to subsets of it.
func NewLinePlotFromDataFrame(
	df *dataframe.DataFrame,
	xAxis string,
	yAxis string,
	groupBy ...string,
) *charts.Line {
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithXAxisOpts(opts.XAxis{Name: xAxis}),
		charts.WithTooltipOpts(opts.Tooltip{
			Trigger:   "item",
			Formatter: "({c})",
		}),
	)
	groupCol := ""
	if len(groupBy) > 0 {
		groupCol = groupBy[0]
	}
	if groupCol != "" {
		// Automatically get unique group values
		groups := df.Col(groupCol).Records()
		uniqueGroups := make(map[string]bool)
		for _, g := range groups {
			uniqueGroups[g] = true
		}
		for group := range uniqueGroups {
			points := make([]opts.LineData, 0)
			fdf := df.Filter(dataframe.F{
				Colname:    groupCol,
				Comparator: series.Eq,
				Comparando: group,
			})
			xSeries := fdf.Col(xAxis)
			ySeries := fdf.Col(yAxis)
			for i := 0; i < xSeries.Len(); i++ {
				points = append(points, opts.LineData{
					Value: []interface{}{
						xSeries.Elem(i).Val(),
						ySeries.Elem(i).Val(),
					},
				})
			}
			line.AddSeries(group, points)
		}
	} else {
		// No grouping, single series
		points := make([]opts.LineData, 0)
		xSeries := df.Col(xAxis)
		ySeries := df.Col(yAxis)
		for i := 0; i < xSeries.Len(); i++ {
			points = append(points, opts.LineData{
				Value: []interface{}{
					xSeries.Elem(i).Val(),
					ySeries.Elem(i).Val(),
				},
			})
		}
		line.AddSeries(yAxis, points)
	}
	return line
}
