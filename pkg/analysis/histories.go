package analysis

import (
	"strconv"

	"github.com/go-gota/gota/dataframe"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// StateTimeHistories is a collection of simulator state histories for
// named partitions that have a cumulative timestep value associated to
// each row in the history.
type StateTimeHistories struct {
	StateHistories   map[string]*simulator.StateHistory
	TimestepsHistory *simulator.CumulativeTimestepsHistory
}

// GetDataFrame constructs a dataframe from the state history of a given
// partition. A "time" column is also provided.
func (s *StateTimeHistories) GetDataFrame(
	partitionName string,
) dataframe.DataFrame {
	stateHistory := s.StateHistories[partitionName]
	df := dataframe.LoadMatrix(stateHistory.Values)
	df = dataframe.LoadMatrix(s.TimestepsHistory.Values).CBind(df)
	cols := []string{"time"}
	for i := 0; i < stateHistory.StateWidth; i++ {
		cols = append(cols, strconv.Itoa(i))
	}
	df.SetNames(cols...)
	return df
}
