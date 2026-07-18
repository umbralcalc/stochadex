package api

import (
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// DataSource is the data: tier's optional file source: instead of running a
// sub-simulation, storage is loaded from a file. Exactly one field is set. (A
// postgres source is deferred — it needs a live *sql.DB connection, a resource
// rather than pure data.)
type DataSource struct {
	Csv     *csvSource     `yaml:"csv,omitempty"`
	JsonLog *jsonLogSource `yaml:"json_log,omitempty"`
}

type csvSource struct {
	Path         string           `yaml:"path"`
	TimeColumn   int              `yaml:"time_column"`
	StateColumns map[string][]int `yaml:"state_columns"`
	SkipHeader   bool             `yaml:"skip_header,omitempty"`
}

type jsonLogSource struct {
	Path string `yaml:"path"`
}

// load reads storage from whichever single source is configured.
func (s *DataSource) load() (*simulator.StateTimeStorage, error) {
	switch {
	case s.Csv != nil && s.JsonLog != nil:
		return nil, fmt.Errorf("api: data.source sets more than one source; pick one")
	case s.Csv != nil:
		return analysis.NewStateTimeStorageFromCsv(
			s.Csv.Path, s.Csv.TimeColumn, s.Csv.StateColumns, s.Csv.SkipHeader,
		)
	case s.JsonLog != nil:
		return analysis.NewStateTimeStorageFromJsonLogEntries(s.JsonLog.Path)
	default:
		return nil, fmt.Errorf("api: data.source is empty; set csv: or json_log:")
	}
}
