package api

import (
	"fmt"
	"sort"
	"strings"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gopkg.in/yaml.v2"
)

// DataSource is the data: tier's optional pre-recorded source: instead of running
// a sub-simulation, storage is loaded from a file or a database. Exactly one field
// is set.
type DataSource struct {
	Csv      *csvSource      `yaml:"csv,omitempty"`
	JsonLog  *jsonLogSource  `yaml:"json_log,omitempty"`
	Postgres *postgresSource `yaml:"postgres,omitempty"`
	// Inline carries the data in the config itself.
	Inline *InlineSource `yaml:"inline,omitempty"`
	// Extra captures any source key not named above, so a package layered on top of
	// api can contribute one through RegisterDataSource without this struct (and
	// therefore the engine's go.mod) having to know about its dependencies. The Arrow
	// source is registered this way by the distributed CLI, because arrow-go lives in
	// a separate opt-in module.
	Extra map[string]map[string]interface{} `yaml:",inline"`
}

// extraDataSources holds sources registered from above this package, keyed by the YAML
// key that selects them (e.g. "arrow" for `source: {arrow: {path: run.arrow}}`).
var extraDataSources = map[string]func(
	fields map[string]interface{},
) (*simulator.StateTimeStorage, error){}

// RegisterDataSource adds a data: source spelling that this package cannot depend on
// directly. It mirrors simulator.RegisterComponent: the engine stays lean, and the
// distributed CLI contributes the sources whose dependencies it alone carries.
func RegisterDataSource(
	name string,
	build func(fields map[string]interface{}) (*simulator.StateTimeStorage, error),
) {
	if _, exists := extraDataSources[name]; exists {
		panic("api: duplicate data source registration " + name)
	}
	extraDataSources[name] = build
}

// InlineSource is data carried in the config itself (source: {inline: ...}): a
// time axis, and each partition's rows, one row per time. A source with only
// times can drive a clock (timestep_function: {type: from_input}).
type InlineSource struct {
	Times      []float64              `yaml:"times"`
	Partitions map[string][][]float64 `yaml:"partitions,omitempty"`
}

// storage builds the data as a StateTimeStorage, checking its shape: a row per
// time, each partition's rows as wide as each other.
func (s *InlineSource) storage() (*simulator.StateTimeStorage, error) {
	if len(s.Times) == 0 {
		return nil, fmt.Errorf("inline: needs times:, the time of each row")
	}
	storage := simulator.NewStateTimeStorage()
	for _, name := range sortedKeys(s.Partitions) {
		rows := s.Partitions[name]
		if len(rows) != len(s.Times) {
			return nil, fmt.Errorf("inline: partition %q has %d rows for %d times",
				name, len(rows), len(s.Times))
		}
		copied := make([][]float64, len(rows))
		for i, row := range rows {
			if len(row) == 0 || len(row) != len(rows[0]) {
				return nil, fmt.Errorf("inline: partition %q's row %d has %d values, "+
					"want %d like its first", name, i, len(row), len(rows[0]))
			}
			copied[i] = append([]float64(nil), row...)
		}
		storage.SetValues(name, copied)
	}
	storage.SetTimes(append([]float64(nil), s.Times...))
	return storage, nil
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
	set := len(s.Extra)
	for _, present := range []bool{s.Csv != nil, s.JsonLog != nil, s.Postgres != nil, s.Inline != nil} {
		if present {
			set++
		}
	}
	if set > 1 {
		return nil, fmt.Errorf("api: data.source sets more than one source; pick one")
	}
	switch {
	case s.Inline != nil:
		return s.Inline.storage()
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
		// A registered source (see RegisterDataSource) — or an unknown key, which must
		// name what is actually available rather than fail vaguely.
		for name, fields := range s.Extra {
			build, ok := extraDataSources[name]
			if !ok {
				available := []string{"csv", "json_log", "postgres"}
				for registered := range extraDataSources {
					available = append(available, registered)
				}
				sort.Strings(available)
				return nil, fmt.Errorf(
					"api: unknown data.source %q; this binary supports: %s",
					name, strings.Join(available, ", "))
			}
			return build(fields)
		}
		return nil, fmt.Errorf("api: data.source is empty; set csv:, json_log: or postgres:")
	}
}

// LoadFormat loads storage for a single named source format from raw fields. It exists so
// a transport registered above this package — the S3 source, which fetches an object and
// then needs it parsed — can reuse the local loaders verbatim instead of re-implementing
// their field handling (a CSV's time_column/state_columns/skip_header, and so on).
//
// The fields are round-tripped through YAML into the same typed structs the config path
// uses, so a transported source validates exactly like a local one.
func LoadFormat(
	format string,
	fields map[string]interface{},
) (*simulator.StateTimeStorage, error) {
	encoded, err := yaml.Marshal(map[string]interface{}{format: fields})
	if err != nil {
		return nil, fmt.Errorf("api: encoding %s source fields: %w", format, err)
	}
	var source DataSource
	if err := yaml.UnmarshalStrict(encoded, &source); err != nil {
		return nil, fmt.Errorf("api: %s source: %w", format, err)
	}
	return source.load()
}
