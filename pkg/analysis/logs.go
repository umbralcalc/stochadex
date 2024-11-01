package analysis

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// NewStateTimeHistoriesFromJsonLogEntries reads a file up to a given number
// of iterations into a StateTimeHistories struct.
func NewStateTimeHistoriesFromJsonLogEntries(
	filename string,
	numIterations int,
) (*StateTimeHistories, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	timeIteration := -1
	partitionIterations := make(map[string]int)
	stateTimeHistories := &StateTimeHistories{
		StateHistories: make(map[string]*simulator.StateHistory),
		TimestepsHistory: &simulator.CumulativeTimestepsHistory{
			Values:            mat.NewVecDense(numIterations, nil),
			StateHistoryDepth: numIterations,
		},
	}
	// Loop through for the specified number of iterations
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var logEntry simulator.JsonLogEntry
		line := scanner.Bytes()

		err := json.Unmarshal(line, &logEntry)
		if err != nil {
			fmt.Println("Error decoding JSON")
			return nil, err
		}

		// Keep a track of how many iterations each partition has been through
		// and also do the same for the cumulative timesteps
		if iters, ok := partitionIterations[logEntry.PartitionName]; ok {
			iters += 1
			if iters == numIterations {
				break
			}
			partitionIterations[logEntry.PartitionName] = iters
			if iters > timeIteration {
				timeIteration = iters
				stateTimeHistories.TimestepsHistory.Values.SetVec(
					numIterations-iters-1, logEntry.CumulativeTimesteps)
			}
		} else {
			partitionIterations[logEntry.PartitionName] = 0
			if 0 > timeIteration {
				timeIteration = 0
				stateTimeHistories.TimestepsHistory.Values.SetVec(
					numIterations-1, logEntry.CumulativeTimesteps)
			}
		}

		// Append the state time histories
		iters := partitionIterations[logEntry.PartitionName]
		if stateHistory, ok :=
			stateTimeHistories.StateHistories[logEntry.PartitionName]; ok {
			stateHistory.Values.SetRow(numIterations-iters-1, logEntry.State)
		} else {
			stateWidth := len(logEntry.State)
			matValues := mat.NewDense(numIterations, stateWidth, nil)
			matValues.SetRow(numIterations-1, logEntry.State)
			stateTimeHistories.StateHistories[logEntry.PartitionName] =
				&simulator.StateHistory{
					Values:            matValues,
					StateWidth:        stateWidth,
					StateHistoryDepth: numIterations,
				}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading file")
		return nil, err
	}

	return stateTimeHistories, nil
}
