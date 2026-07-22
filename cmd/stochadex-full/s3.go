package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/umbralcalc/stochadex/pkg/api"
	"github.com/umbralcalc/stochadex/pkg/s3store"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Config wiring for the object-storage integration. The transport itself lives in
// pkg/s3store — an ordinary Go package, so a downstream repo can move runs to and from S3
// without this binary. What belongs here, and only here, is the mapping from YAML to that
// package, because it needs the format loaders and sinks that live in api and this module.
//
//	data:
//	  source:
//	    s3: {bucket: my-bucket, key: runs/in.arrow, format: arrow}
//
//	simulation:
//	  output_function: {type: s3, bucket: my-bucket, key: runs/out.arrow, format: arrow}
//
// `format:` names what the object contains, and the transport pairs with the existing
// reader/sink for it — so S3 supports whatever local files support. Credentials come from
// the standard AWS chain, never the config. Optional `region:` and `endpoint:` are passed
// through; set `endpoint:` for any S3-compatible store.
func init() {
	api.RegisterDataSource("s3", func(
		fields map[string]interface{},
	) (*simulator.StateTimeStorage, error) {
		bucket, err := sourceStringNamed(fields, "s3 source", "bucket")
		if err != nil {
			return nil, err
		}
		key, err := sourceStringNamed(fields, "s3 source", "key")
		if err != nil {
			return nil, err
		}
		format, err := sourceStringNamed(fields, "s3 source", "format")
		if err != nil {
			return nil, err
		}

		ctx := context.Background()
		client, err := s3store.NewClient(ctx, s3ConfigFrom(fields))
		if err != nil {
			return nil, err
		}
		local, cleanup, err := s3store.Fetch(ctx, client, bucket, key)
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Hand the local copy to the loader for the declared format, passing any
		// format-specific options (a CSV's time_column, state_columns, …) straight through.
		inner := map[string]interface{}{}
		for name, value := range fields {
			if !isTransportField(name) {
				inner[name] = value
			}
		}
		inner["path"] = local
		return loadByFormat(format, inner)
	})

	simulator.RegisterComponent(
		"output_function",
		"s3",
		func(spec simulator.ComponentSpec) (interface{}, error) {
			bucket, err := stringField(spec, "bucket")
			if err != nil {
				return nil, err
			}
			key, err := stringField(spec, "key")
			if err != nil {
				return nil, err
			}
			format, err := stringField(spec, "format")
			if err != nil {
				return nil, err
			}

			// Stage locally, upload once at Finalize. The staged name keeps the key's
			// extension so format sniffing on the far side still works.
			staged := filepath.Join(os.TempDir(),
				fmt.Sprintf("stochadex-s3-%s", filepath.Base(key)))
			inner, err := localSinkForFormat(format, staged, spec)
			if err != nil {
				return nil, err
			}
			return s3store.NewOutputFunction(
				inner, staged, bucket, key, s3ConfigFrom(spec.Fields)), nil
		},
	)
}

// isTransportField reports whether a key configures the transport rather than the payload
// format, so it is not forwarded to the format's own loader/sink.
func isTransportField(name string) bool {
	switch name {
	case "bucket", "key", "format", "region", "endpoint":
		return true
	}
	return false
}

// s3ConfigFrom reads the optional client overrides out of a spec's fields.
func s3ConfigFrom(fields map[string]interface{}) s3store.Config {
	config := s3store.Config{}
	if region, ok := fields["region"].(string); ok {
		config.Region = region
	}
	if endpoint, ok := fields["endpoint"].(string); ok {
		config.Endpoint = endpoint
	}
	return config
}

// loadByFormat dispatches a downloaded object to the loader for its declared format.
func loadByFormat(
	format string,
	fields map[string]interface{},
) (*simulator.StateTimeStorage, error) {
	switch format {
	case "arrow":
		path, ok := fields["path"].(string)
		if !ok {
			return nil, fmt.Errorf("s3 source: arrow format has no usable path")
		}
		return loadArrowStorage(path)
	case "csv", "json_log":
		// Reuse the local loaders' own field handling verbatim rather than duplicating it.
		return api.LoadFormat(format, fields)
	default:
		return nil, fmt.Errorf(
			"s3 source: unsupported format %q; expected arrow, csv or json_log", format)
	}
}

// localSinkForFormat builds the ordinary local sink that the S3 sink writes through.
func localSinkForFormat(
	format, path string,
	spec simulator.ComponentSpec,
) (simulator.OutputFunction, error) {
	fields := map[string]interface{}{"path": path}
	for name, value := range spec.Fields {
		if !isTransportField(name) {
			fields[name] = value
		}
	}
	switch format {
	case "arrow", "json_log":
		return simulator.ResolveOutputFunction(
			simulator.ComponentSpec{Type: format, Fields: fields})
	default:
		return nil, fmt.Errorf(
			"s3 output: unsupported format %q; expected arrow or json_log", format)
	}
}

// sourceStringNamed reads a required string key, naming the source in the error.
func sourceStringNamed(fields map[string]interface{}, who, key string) (string, error) {
	raw, ok := fields[key]
	if !ok {
		return "", fmt.Errorf("%s: missing required field %q", who, key)
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s: field %q must be a string, got %T", who, key, raw)
	}
	return value, nil
}
