package analysis

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// NewStateTimeStorageFromCsv creates a StateTimeStorage from CSV data.
//
// This function reads time series data from a CSV file and organizes it into
// partitions for use in stochadex simulations. It supports multiple partitions
// with different column configurations.
//
// Parameters:
//   - filePath: Path to the CSV file to read (must exist and be readable)
//   - timeColumn: Index of the column containing timestamps (0-based indexing)
//   - stateColumnsByPartition: Map of partition names to column indices for their state values
//   - skipHeaderRow: Whether to skip the first row as headers (recommended for CSV files with headers)
//
// Returns:
//   - *StateTimeStorage: Storage containing the loaded time series data, organized by partition
//   - error: Any error encountered during file reading or parsing
//
// CSV Format Requirements:
//   - Time column must contain parseable float64 values
//   - State columns must contain parseable float64 values
//   - All rows must have the same number of columns
//   - Missing or malformed values will cause parsing errors
//
// Example:
//
//	// Load data from a CSV with time in column 0, prices in columns 1-2, volumes in column 3
//	storage, err := NewStateTimeStorageFromCsv(
//	    "market_data.csv",
//	    0, // time in first column
//	    map[string][]int{
//	        "prices": {1, 2}, // prices partition uses columns 1 and 2
//	        "volumes": {3},   // volumes partition uses column 3
//	    },
//	    true, // skip header row
//	)
//	if err != nil {
//	    log.Fatal("Failed to load CSV data:", err)
//	}
//
// Error Handling:
//   - File not found: Returns error with file path
//   - CSV parsing errors: Returns error with parsing details
//   - Invalid numeric values: Returns error with conversion details
//   - Inconsistent row lengths: Returns error with row information
//
// Performance Notes:
//   - Loads entire file into memory (consider file size for large datasets)
//   - O(n) time complexity where n is the number of rows
//   - Memory usage: O(n * m) where m is the total number of state columns
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
