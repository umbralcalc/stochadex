package api

import (
	"database/sql"
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Registers `output_function: {type: postgres, ...}`, making database writes drivable
// from config rather than Go.
//
// This lives in pkg/api, not in the simulator component registry, for a layering reason:
// the registry is in pkg/simulator, and the Postgres sink is in pkg/analysis, which
// imports simulator — registering it there would be an import cycle. simulator's
// RegisterComponent hook exists precisely so a package above it can contribute a
// component, which is the same route the embedded-window from_history timestep takes.
//
// Two spellings, because analysis.PostgresDb takes either a DSN-opened handle or local
// credentials:
//
//	output_function: {type: postgres, user: u, password: p, dbname: d, table: results}
//	output_function: {type: postgres, driver: pgx, dsn: "postgres://…", table: results}
//
// The `driver`/`dsn` form goes through database/sql, so it reaches any Postgres-wire
// database (TimescaleDB, CockroachDB, a managed instance with sslmode) — and any other
// database whose driver is compiled in — rather than only a local Postgres.
func init() {
	simulator.RegisterComponent(
		"output_function",
		"postgres",
		func(spec simulator.ComponentSpec) (interface{}, error) {
			table, err := specString(spec, "table", true)
			if err != nil {
				return nil, err
			}
			dsn, err := specString(spec, "dsn", false)
			if err != nil {
				return nil, err
			}

			db := &analysis.PostgresDb{TableName: table}
			if dsn != "" {
				driver, err := specString(spec, "driver", false)
				if err != nil {
					return nil, err
				}
				if driver == "" {
					driver = "postgres"
				}
				handle, err := sql.Open(driver, dsn)
				if err != nil {
					return nil, fmt.Errorf(
						"output_function postgres: opening %s connection: %w", driver, err)
				}
				db.DB = handle
			} else {
				// Local-credentials form: every field is required, since a missing one
				// would otherwise surface as an opaque connection failure at run time.
				for field, target := range map[string]*string{
					"user": &db.User, "password": &db.Password, "dbname": &db.Dbname,
				} {
					value, err := specString(spec, field, true)
					if err != nil {
						return nil, err
					}
					*target = value
				}
			}
			return analysis.NewPostgresDbOutputFunction(db), nil
		},
	)
}

// specString reads a string key from a data spec. A required key that is absent, or any
// key of the wrong type, is an error naming the field — so a typo fails at load with a
// located message instead of silently writing nowhere.
func specString(spec simulator.ComponentSpec, key string, required bool) (string, error) {
	raw, ok := spec.Fields[key]
	if !ok {
		if required {
			return "", fmt.Errorf(
				"output_function %q: missing required field %q", spec.Type, key)
		}
		return "", nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf(
			"output_function %q: field %q must be a string, got %T", spec.Type, key, raw)
	}
	return value, nil
}
