package simulator

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strconv"

	"golang.org/x/exp/slices"
	"gonum.org/v1/gonum/mat"
)

// MemoryIteration provides a stream of data which is already know from a
// separate data source and is held in memory.
type MemoryIteration struct {
	Data *StateHistory
}

func (m *MemoryIteration) Configure(
	partitionIndex int,
	settings *Settings,
) {
}

func (m *MemoryIteration) Iterate(
	params *OtherParams,
	partitionIndex int,
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) []float64 {
	data := m.Data.Values.RawRowView(m.Data.StateHistoryDepth -
		timestepsHistory.CurrentStepNumber)
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
		Data: &StateHistory{
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
	Data *CumulativeTimestepsHistory
}

func (m *MemoryTimestepFunction) SetNextIncrement(
	timestepsHistory *CumulativeTimestepsHistory,
) *CumulativeTimestepsHistory {
	i := m.Data.StateHistoryDepth - timestepsHistory.CurrentStepNumber
	timestepsHistory.NextIncrement =
		m.Data.Values.AtVec(i-1) - m.Data.Values.AtVec(i)
	return timestepsHistory
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
		Data: &CumulativeTimestepsHistory{
			Values:            mat.NewVecDense(len(times), times),
			StateHistoryDepth: len(times),
		},
	}
}
