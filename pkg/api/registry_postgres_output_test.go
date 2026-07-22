package api

import (
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// TestPostgresOutputRegistered proves the Postgres sink is reachable as a data spec.
// Before this registration the sink existed only in Go, so a config could READ from a
// database (data.source.postgres) but never WRITE to one — the asymmetry this closes.
//
// Only the validation paths are exercised: constructing the sink opens a connection (and
// analysis.NewPostgresDbOutputFunction panics if that fails), so the success path needs a
// live database and is covered by the Postgres integration test instead.
func TestPostgresOutputRegistered(t *testing.T) {
	resolve := func(fields map[string]interface{}) error {
		_, err := simulator.ResolveOutputFunction(simulator.ComponentSpec{
			Type:   "postgres",
			Fields: fields,
		})
		return err
	}

	t.Run("the type resolves at all", func(t *testing.T) {
		// An unknown type reports "unknown"; a registered one must not.
		err := resolve(map[string]interface{}{})
		if err == nil {
			t.Fatal("expected a validation error for an empty spec")
		}
		if strings.Contains(strings.ToLower(err.Error()), "unknown") {
			t.Errorf("postgres is not registered as an output_function: %v", err)
		}
	})

	t.Run("a missing required field names it", func(t *testing.T) {
		err := resolve(map[string]interface{}{})
		if err == nil || !strings.Contains(err.Error(), "table") {
			t.Errorf("error should name the missing 'table' field, got: %v", err)
		}
	})

	t.Run("credentials form requires every field", func(t *testing.T) {
		err := resolve(map[string]interface{}{"table": "results", "user": "u"})
		if err == nil {
			t.Fatal("expected an error for incomplete credentials")
		}
		// password or dbname must be named — map iteration order decides which comes first.
		if !strings.Contains(err.Error(), "password") && !strings.Contains(err.Error(), "dbname") {
			t.Errorf("error should name the missing credential field, got: %v", err)
		}
	})

	t.Run("a wrongly-typed field is rejected", func(t *testing.T) {
		err := resolve(map[string]interface{}{"table": 42})
		if err == nil || !strings.Contains(err.Error(), "must be a string") {
			t.Errorf("expected a type error naming the field, got: %v", err)
		}
	})
}
