package general

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/slices"
	"gonum.org/v1/gonum/mat"
)

// MemoryIteration provides a stream of data which is already know from a
// separate data source and is held in memory.
type MemoryIteration struct {
	Data *simulator.StateHistory
}

func (m *MemoryIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (m *MemoryIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	var data []float64
	// starts from one step into the window because it makes it possible to
	// use the i := m.Data.StateHistoryDepth - timestepsHistory.CurrentStepNumber value
	// for the initial conditions
	if i := m.Data.StateHistoryDepth - timestepsHistory.CurrentStepNumber - 1; i >= 0 {
		data = m.Data.Values.RawRowView(i)
	} else if i == -1 {
		data = params.Get("latest_data_values")
	} else {
		panic("timesteps have gone beyond the available data")
	}
	return data
}

// NewMemoryIterationFromCsv creates a new MemoryIteration based on data
// that is read in from the provided csv file and some specified columns
// for time and state.
func NewMemoryIterationFromCsv(
	filePath string,
	stateColumns []int,
	skipHeaderRow bool,
) *MemoryIteration {
	f, err := os.Open(filePath)
	if err != nil {
		log.Fatal("Unable to read input file "+filePath, err)
	}
	defer f.Close()

	// create this as a faster lookup
	stateColumnsMap := make(map[int]bool)
	for _, column := range stateColumns {
		stateColumnsMap[column] = true
	}

	csvReader := csv.NewReader(f)
	records, err := csvReader.ReadAll()
	if err != nil {
		log.Fatal("Unable to parse file as CSV for "+filePath, err)
	}
	data := make([]float64, 0)
	timeSeriesLength := 0
	for _, row := range records {
		if skipHeaderRow {
			skipHeaderRow = false
			continue
		}
		floatRow := make([]float64, 0)
		for i, r := range row {
			_, ok := stateColumnsMap[i]
			if !ok {
				continue
			}
			dataPoint, err := strconv.ParseFloat(r, 64)
			if err != nil {
				fmt.Printf("Error converting string: %v", err)
			}
			floatRow = append(floatRow, dataPoint)
		}
		data = append(data, floatRow...)
		timeSeriesLength += 1
	}
	// default is to have the same index ordering as for a windowed
	// history so that row 0 is the last point
	slices.Reverse(data)
	return &MemoryIteration{
		Data: &simulator.StateHistory{
			Values: mat.NewDense(
				timeSeriesLength,
				len(stateColumns),
				data,
			),
			StateWidth:        len(stateColumns),
			StateHistoryDepth: timeSeriesLength,
		},
	}
}

// MemoryTimestepFunction provides a stream of timesteps which already known from
// a separate data source and is held in memory.
type MemoryTimestepFunction struct {
	Data *simulator.CumulativeTimestepsHistory
}

func (m *MemoryTimestepFunction) NextIncrement(
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) float64 {
	// starts from one step into the window because it makes it possible to
	// use the i := m.Data.StateHistoryDepth - timestepsHistory.CurrentStepNumber value
	// for the initial conditions
	if i := m.Data.StateHistoryDepth - timestepsHistory.CurrentStepNumber - 1; i >= 0 {
		return m.Data.Values.AtVec(i) - timestepsHistory.Values.AtVec(0)
	} else if i == -1 {
		return m.Data.NextIncrement
	} else {
		panic("timesteps have gone beyond the available data")
	}
}

// NewMemoryTimestepFunctionFromCsv creates a new MemoryTimestepFunction
// based on data that is read in from the provided csv file and some specified
// columns for time and state.
func NewMemoryTimestepFunctionFromCsv(
	filePath string,
	timeColumn int,
	skipHeaderRow bool,
) *MemoryTimestepFunction {
	f, err := os.Open(filePath)
	if err != nil {
		log.Fatal("Unable to read input file "+filePath, err)
	}
	defer f.Close()

	csvReader := csv.NewReader(f)
	records, err := csvReader.ReadAll()
	if err != nil {
		log.Fatal("Unable to parse file as CSV for "+filePath, err)
	}
	times := make([]float64, 0)
	for _, row := range records {
		if skipHeaderRow {
			skipHeaderRow = false
			continue
		}
		time, err := strconv.ParseFloat(row[timeColumn], 64)
		if err != nil {
			fmt.Printf("Error converting string: %v", err)
		}
		times = append(times, time)
	}
	// default is to have the same index ordering as for a windowed
	// history so that row 0 is the last point
	slices.Reverse(times)
	return &MemoryTimestepFunction{
		Data: &simulator.CumulativeTimestepsHistory{
			Values:            mat.NewVecDense(len(times), times),
			StateHistoryDepth: len(times),
		},
	}
}
