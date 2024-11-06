package analysis

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// NewStateTimeStorageFromJsonLogEntries reads a file up to a given number
// of iterations into a simulator.StateTimeStorage struct.
func NewStateTimeStorageFromJsonLogEntries(
	filename string,
) (*simulator.StateTimeStorage, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Loop through for the specified number of iterations
	storage := simulator.NewStateTimeStorage()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var logEntry simulator.JsonLogEntry
		line := scanner.Bytes()

		err := json.Unmarshal(line, &logEntry)
		if err != nil {
			fmt.Println("Error decoding JSON")
			return nil, err
		}
		storage.ConcurrentAppend(
			logEntry.PartitionName,
			logEntry.CumulativeTimesteps,
			logEntry.State,
		)
	}
	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading file")
		return nil, err
	}
	return storage, nil
}
