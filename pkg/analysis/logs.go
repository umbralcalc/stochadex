package analysis

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/go-gota/gota/dataframe"
	dataseries "github.com/go-gota/gota/series"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// QueryLogEntry is the output format from the logs scanner.
type QueryLogEntry struct {
	PartitionIterations int                    `json:"partition_iterations"`
	Entry               simulator.JsonLogEntry `json:"entry"`
}

// QueryLogEntries maps each collection of QueryLogEntry structs to a partition.
type QueryLogEntries struct {
	EntriesByPartition map[string][]QueryLogEntry
}

// DataFrameFromPartition constructs a dataframe from the state values of
// a given partition.
func (q *QueryLogEntries) DataFrameFromPartition(
	partitionName string,
) dataframe.DataFrame {
	series := make([]dataseries.Series, 0)
	entries, ok := q.EntriesByPartition[partitionName]
	if !ok {
		partitions := make([]string, 0)
		for name := range q.EntriesByPartition {
			partitions = append(partitions, name)
		}
		panic("partition name: " + partitionName +
			" not found, choices are: " + strings.Join(partitions, ", "))
	}
	for i := 0; i < len(entries[0].Entry.State); i++ {
		series = append(
			series,
			dataseries.New([]float64{}, dataseries.Float, strconv.Itoa(i)),
		)
	}
	for _, entry := range entries {
		for i, value := range entry.Entry.State {
			series[i].Append(value)
		}
	}
	return dataframe.New(series...)
}

// ReadLogEntries reads a file up to a given number of iterations into
// a collection which maps each entry to a partition.
func ReadLogEntries(
	filename string,
	numIterations int,
) (*QueryLogEntries, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Loop through for the specified number of iterations
	partitionIterations := make(map[string]int)
	entriesByPartition := make(map[string][]QueryLogEntry)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var logEntry simulator.JsonLogEntry
		line := scanner.Bytes()

		err := json.Unmarshal(line, &logEntry)
		if err != nil {
			fmt.Println("Error decoding JSON:", err)
			continue
		}

		// keep a track of how many iterations each partition has been through
		if iters, ok := partitionIterations[logEntry.PartitionName]; ok {
			partitionIterations[logEntry.PartitionName] = iters + 1
		} else {
			partitionIterations[logEntry.PartitionName] = 0
		}

		// Append the log entries
		if entries, ok := entriesByPartition[logEntry.PartitionName]; ok {
			entriesByPartition[logEntry.PartitionName] = append(
				entries,
				QueryLogEntry{
					PartitionIterations: partitionIterations[logEntry.PartitionName],
					Entry:               logEntry,
				},
			)
		} else {
			entriesByPartition[logEntry.PartitionName] = []QueryLogEntry{{
				PartitionIterations: partitionIterations[logEntry.PartitionName],
				Entry:               logEntry,
			}}
		}
		if partitionIterations[logEntry.PartitionName] > numIterations {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading file:", err)
	}

	return &QueryLogEntries{EntriesByPartition: entriesByPartition}, nil
}
