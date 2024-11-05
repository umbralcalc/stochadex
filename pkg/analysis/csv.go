package analysis

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// NewStateTimeStorageFromCsv creates a new StateTimeStorage based on
// data that is read in from the provided csv file and some specified
// columns for time and state.
func NewStateTimeStorageFromCsv(
	filePath string,
	timeColumn int,
	stateColumnsByPartition map[string][]int,
	skipHeaderRow bool,
) (*simulator.StateTimeStorage, error) {
	f, err := os.Open(filePath)
	if err != nil {
		log.Fatal("Unable to read input file " + filePath)
		return nil, err
	}
	defer f.Close()

	storage := simulator.NewStateTimeStorage()
	csvReader := csv.NewReader(f)
	records, err := csvReader.ReadAll()
	if err != nil {
		log.Fatal("Unable to parse file as CSV for " + filePath)
		return nil, err
	}
	for _, row := range records {
		if skipHeaderRow {
			skipHeaderRow = false
			continue
		}
		time, err := strconv.ParseFloat(row[timeColumn], 64)
		if err != nil {
			fmt.Printf("Error converting string: %v", err)
		}
		for partition, columns := range stateColumnsByPartition {
			data := make([]float64, 0)
			for _, column := range columns {
				dataPoint, err := strconv.ParseFloat(row[column], 64)
				if err != nil {
					fmt.Printf("Error converting string")
					return nil, err
				}
				data = append(data, dataPoint)
			}
			storage.ConcurrentAppend(partition, time, data)
		}
	}
	return storage, nil
}
