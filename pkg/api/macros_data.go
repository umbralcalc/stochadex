package api

import (
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// DataSource is the data: tier's optional pre-recorded source: instead of running
// a sub-simulation, storage is loaded from a file or a database. Exactly one field
// is set.
type DataSource struct {
	Csv      *csvSource      `yaml:"csv,omitempty"`
	JsonLog  *jsonLogSource  `yaml:"json_log,omitempty"`
	Postgres *postgresSource `yaml:"postgres,omitempty"`
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

// postgresSource reads storage from a Postgres table over the range [start_time,
// end_time] for the named partitions. The connection is opened from the
// user/password/dbname fields (analysis.PostgresDb.OpenTableConnection).
type postgresSource struct {
	User           string   `yaml:"user"`
	Password       string   `yaml:"password"`
	Dbname         string   `yaml:"dbname"`
	Table          string   `yaml:"table"`
	PartitionNames []string `yaml:"partition_names"`
	StartTime      float64  `yaml:"start_time"`
	EndTime        float64  `yaml:"end_time"`
}

// load reads storage from whichever single source is configured.
func (s *DataSource) load() (*simulator.StateTimeStorage, error) {
	set := 0
	for _, present := range []bool{s.Csv != nil, s.JsonLog != nil, s.Postgres != nil} {
		if present {
			set++
		}
	}
	if set > 1 {
		return nil, fmt.Errorf("api: data.source sets more than one source; pick one")
	}
	switch {
	case s.Csv != nil:
		return analysis.NewStateTimeStorageFromCsv(
			s.Csv.Path, s.Csv.TimeColumn, s.Csv.StateColumns, s.Csv.SkipHeader,
		)
	case s.JsonLog != nil:
		return analysis.NewStateTimeStorageFromJsonLogEntries(s.JsonLog.Path)
	case s.Postgres != nil:
		return analysis.NewStateTimeStorageFromPostgresDb(
			&analysis.PostgresDb{
				User:      s.Postgres.User,
				Password:  s.Postgres.Password,
				Dbname:    s.Postgres.Dbname,
				TableName: s.Postgres.Table,
			},
			s.Postgres.PartitionNames,
			s.Postgres.StartTime,
			s.Postgres.EndTime,
		)
	default:
		return nil, fmt.Errorf("api: data.source is empty; set csv:, json_log: or postgres:")
	}
}
