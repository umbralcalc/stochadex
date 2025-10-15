package analysis

import (
	"slices"
	"strconv"

	"github.com/go-gota/gota/dataframe"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// GetDataFrameFromPartition materializes a partition's values and times
// into a Gota dataframe. The first column is named "time" followed by one
// column per value index labeled by its integer index.
func GetDataFrameFromPartition(
	storage *simulator.StateTimeStorage,
	partitionName string,
) dataframe.DataFrame {
	storedTimes := storage.GetTimes()
	storedValues := storage.GetValues(partitionName)
	df := dataframe.LoadMatrix(
		mat.NewDense(
			len(storedValues),
			len(storedValues[0]),
			slices.Concat(storedValues...),
		),
	)
	df = dataframe.LoadMatrix(
		mat.NewVecDense(len(storedTimes), storedTimes)).CBind(df)
	cols := []string{"time"}
	for i := range storedValues[0] {
		cols = append(cols, strconv.Itoa(i))
	}
	df.SetNames(cols...)
	return df
}

// SetPartitionFromDataFrame updates a partition's values from a Gota
// dataframe with schema [time, 0, 1, ...]. If overwriteTime is true, the
// storage's time vector is replaced with the "time" column.
func SetPartitionFromDataFrame(
	storage *simulator.StateTimeStorage,
	partitionName string,
	df dataframe.DataFrame,
	overwriteTime bool,
) {
	data := make([][]float64, 0)
	for i := range df.Nrow() {
		row := make([]float64, 0)
		for j := range df.Ncol() - 1 {
			row = append(
				row,
				df.Col(strconv.Itoa(j)).Elem(i).Float(),
			)
		}
		data = append(data, row)
	}
	storage.SetValues(partitionName, data)
	if overwriteTime {
		timeData := make([]float64, 0)
		for i := range df.Nrow() {
			timeData = append(timeData, df.Col("time").Elem(i).Float())
		}
		storage.SetTimes(timeData)
	}
}
