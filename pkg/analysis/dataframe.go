package analysis

import (
	"slices"
	"strconv"

	"github.com/go-gota/gota/dataframe"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// GetDataFrameFromPartition constructs a dataframe from the state time
// storage of a given partition. A "time" column is also provided.
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
	for i := 0; i < len(storedValues[0]); i++ {
		cols = append(cols, partitionName+strconv.Itoa(i))
	}
	df.SetNames(cols...)
	return df
}

// SetPartitionFromDataFrame sets the values in the state time storage of a
// given partition using a dataframe as input. This dataframe can optionally
// also be used to overwrite the stored times by setting the overwriteTime
// boolean flag to true.
func SetPartitionFromDataFrame(
	storage *simulator.StateTimeStorage,
	partitionName string,
	df dataframe.DataFrame,
	overwriteTime bool,
) {
	data := make([][]float64, 0)
	for i := 0; i < df.Nrow(); i++ {
		row := make([]float64, 0)
		for j := 0; j < df.Ncol()-1; j++ {
			row = append(
				row,
				df.Col(partitionName+strconv.Itoa(j)).Elem(i).Float(),
			)
		}
		data = append(data, row)
	}
	storage.SetValues(partitionName, data)
	if overwriteTime {
		timeData := make([]float64, 0)
		for i := 0; i < df.Nrow(); i++ {
			timeData = append(timeData, df.Col("time").Elem(i).Float())
		}
		storage.SetTimes(timeData)
	}
}
