package main

import (
	"fmt"
	"os"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/umbralcalc/stochadex/pkg/api"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Registers `data: {source: {arrow: {path: run.arrow}}}`, closing the round trip: a run
// written with `output_function: {type: arrow}` can be read straight back in as the
// dataset for an analysis or inference macro, with no CSV detour and no Go.
//
// It goes through api.RegisterDataSource because arrow-go lives in a separate opt-in
// module — pkg/api cannot import it without putting Arrow in the engine's go.mod.
func init() {
	api.RegisterDataSource("arrow", func(
		fields map[string]interface{},
	) (*simulator.StateTimeStorage, error) {
		path, err := sourceString(fields, "path")
		if err != nil {
			return nil, err
		}
		return loadArrowStorage(path)
	})
}

// loadArrowStorage reads an Arrow IPC file written by the arrow output function — a
// float64 "time" column plus one fixed-size-list column per partition — back into the
// engine's StateTimeStorage.
func loadArrowStorage(path string) (*simulator.StateTimeStorage, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("arrow source: opening %s: %w", path, err)
	}
	defer file.Close()

	reader, err := ipc.NewFileReader(file)
	if err != nil {
		return nil, fmt.Errorf("arrow source: reading %s as Arrow IPC: %w", path, err)
	}
	defer reader.Close()

	storage := simulator.NewStateTimeStorage()
	for i := 0; i < reader.NumRecords(); i++ {
		record, err := reader.Record(i)
		if err != nil {
			return nil, fmt.Errorf("arrow source: record %d of %s: %w", i, path, err)
		}
		if err := appendRecord(storage, record); err != nil {
			return nil, fmt.Errorf("arrow source: %s: %w", path, err)
		}
	}
	if len(storage.GetNames()) == 0 {
		return nil, fmt.Errorf(
			"arrow source: %s contains no partition columns (expected a 'time' column "+
				"plus one fixed-size-list column per partition)", path)
	}
	return storage, nil
}

// appendRecord copies one Arrow record into storage, matching the schema the arrow
// output function writes.
func appendRecord(storage *simulator.StateTimeStorage, record arrow.Record) error {
	schema := record.Schema()
	timeIndices := schema.FieldIndices("time")
	if len(timeIndices) == 0 {
		return fmt.Errorf("no 'time' column in the record schema")
	}
	times, ok := record.Column(timeIndices[0]).(*array.Float64)
	if !ok {
		return fmt.Errorf("'time' column is %s, expected float64",
			record.Column(timeIndices[0]).DataType())
	}

	for column := range int(record.NumCols()) {
		name := schema.Field(column).Name
		if name == "time" {
			continue
		}
		values, ok := record.Column(column).(*array.FixedSizeList)
		if !ok {
			// Skip columns the writer would not have produced rather than failing the
			// whole load — a file may legitimately carry extra metadata columns.
			continue
		}
		width := int(values.DataType().(*arrow.FixedSizeListType).Len())
		items, ok := values.ListValues().(*array.Float64)
		if !ok {
			return fmt.Errorf("partition %q holds %s values, expected float64",
				name, values.ListValues().DataType())
		}
		for row := range values.Len() {
			state := make([]float64, width)
			for j := range width {
				state[j] = items.Value(row*width + j)
			}
			storage.Append(name, times.Value(row), state)
		}
	}
	return nil
}

// sourceString reads a required string key from a data-source spec, naming the source and
// key so a mistyped config fails at load with an actionable message.
func sourceString(fields map[string]interface{}, key string) (string, error) {
	raw, ok := fields[key]
	if !ok {
		return "", fmt.Errorf("arrow source: missing required field %q", key)
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("arrow source: field %q must be a string, got %T", key, raw)
	}
	return value, nil
}
