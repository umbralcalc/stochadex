package analysis

import (
	"strconv"
	"strings"

	"github.com/go-gota/gota/dataframe"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// StateTimeHistories is a collection of simulator state histories for
// named partitions that have a cumulative timestep value associated to
// each row in the history.
type StateTimeHistories struct {
	StateHistories   map[string]*simulator.StateHistory
	TimestepsHistory *simulator.CumulativeTimestepsHistory
}

// GetDataFrameFromPartition constructs a dataframe from the state history
// of a given partition. A "time" column is also provided.
func (s *StateTimeHistories) GetDataFrameFromPartition(
	partitionName string,
) dataframe.DataFrame {
	stateHistory, ok := s.StateHistories[partitionName]
	if !ok {
		partitions := make([]string, 0)
		for name := range s.StateHistories {
			partitions = append(partitions, name)
		}
		panic("partition name: " + partitionName +
			" not found, choices are: " + strings.Join(partitions, ", "))
	}
	df := dataframe.LoadMatrix(stateHistory.Values)
	df = dataframe.LoadMatrix(s.TimestepsHistory.Values).CBind(df)
	cols := []string{"time"}
	for i := 0; i < stateHistory.StateWidth; i++ {
		cols = append(cols, strconv.Itoa(i))
	}
	df.SetNames(cols...)
	return df
}

// SetPartitionFromDataFrame sets the values in the state history of a
// given partition using a dataframe as input. This dataframe can optionally
// also be used to overwrite the timesteps history by setting the overwriteTime
// boolean flag to true.
func (s *StateTimeHistories) SetPartitionFromDataFrame(
	partitionName string,
	df dataframe.DataFrame,
	overwriteTime bool,
) {
	nCols := df.Ncol() - 1
	nRows := df.Nrow()
	data := make([]float64, 0)
	for i := 0; i < nRows; i++ {
		for j := 0; j < nCols; j++ {
			data = append(data, df.Col(strconv.Itoa(j)).Elem(i).Float())
		}
	}
	s.StateHistories[partitionName] = &simulator.StateHistory{
		Values:            mat.NewDense(nRows, nCols, data),
		StateWidth:        nCols,
		StateHistoryDepth: nRows,
	}
	if overwriteTime {
		timeData := make([]float64, 0)
		for i := 0; i < nRows; i++ {
			timeData = append(timeData, df.Col("time").Elem(i).Float())
		}
		s.TimestepsHistory = &simulator.CumulativeTimestepsHistory{
			Values:            mat.NewVecDense(nRows, timeData),
			StateHistoryDepth: nRows,
		}
	}
}
