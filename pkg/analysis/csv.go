package analysis

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// NewStateTimeHistoriesFromCsv creates a new StateTimeHistories based on
// data that is read in from the provided csv file and some specified
// columns for time and state.
func NewStateTimeHistoriesFromCsv(
	filePath string,
	timeColumn int,
	stateColumnsByPartition map[string][]int,
	skipHeaderRow bool,
) *StateTimeHistories {
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
	data := make(map[string][]float64, 0)
	for partition := range stateColumnsByPartition {
		data[partition] = make([]float64, 0)
	}
	times := make([]float64, 0)
	timeSeriesLength := 0
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
		for partition, columns := range stateColumnsByPartition {
			numberOfColumns := len(columns)
			for i := 0; i < numberOfColumns; i++ {
				// work backwards along the column indices so that
				// the slices.Reverse operation is consistent later
				dataPoint, err := strconv.ParseFloat(
					row[columns[numberOfColumns-i-1]], 64)
				if err != nil {
					fmt.Printf("Error converting string: %v", err)
				}
				data[partition] = append(data[partition], dataPoint)
			}
		}
		timeSeriesLength += 1
	}
	stateHistories := make(map[string]*simulator.StateHistory)
	for partition, partitionData := range data {
		// default is to have the same index ordering as for a windowed
		// history so that row 0 is the last point
		slices.Reverse(partitionData)
		stateWidth := len(stateColumnsByPartition[partition])
		stateHistories[partition] = &simulator.StateHistory{
			Values: mat.NewDense(
				timeSeriesLength,
				stateWidth,
				partitionData,
			),
			StateWidth:        stateWidth,
			StateHistoryDepth: timeSeriesLength,
		}

	}
	// default also applies here
	slices.Reverse(times)
	return &StateTimeHistories{
		StateHistories: stateHistories,
		TimestepsHistory: &simulator.CumulativeTimestepsHistory{
			Values:            mat.NewVecDense(len(times), times),
			StateHistoryDepth: len(times),
		},
	}
}
