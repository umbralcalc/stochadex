package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/umbralcalc/stochadex/pkg/api"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// S3 is wired as a TRANSPORT rather than a format. Object storage is orthogonal to what
// the bytes contain, so instead of an "S3 source" that must re-implement CSV, JSON-log
// and Arrow parsing (and be extended again for the next format), the S3 source downloads
// the object to a temporary file and hands it to the existing loader for `format:`, and
// the S3 sink writes through the existing local sink and uploads the result once the run
// finishes. Every present and future format is reachable over S3 for free.
//
//	data:
//	  source:
//	    s3: {bucket: my-bucket, key: runs/in.arrow, format: arrow}
//
//	simulation:
//	  output_function: {type: s3, bucket: my-bucket, key: runs/out.arrow, format: arrow}
//
// Credentials come from the standard AWS chain (environment, shared config, IAM role) —
// nothing is ever put in the config file. `region` and `endpoint` are optional; setting
// `endpoint` points the client at any S3-compatible store (MinIO, Cloudflare R2, Ceph).
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

		local, cleanup, err := downloadS3Object(fields, bucket, key)
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Delegate to the loader for the declared format, with the local copy's path
		// substituted in — so the S3 path supports exactly what local files support.
		inner := map[string]interface{}{}
		for name, value := range fields {
			switch name {
			case "bucket", "key", "format", "region", "endpoint":
			default:
				inner[name] = value // format-specific options pass straight through
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

			// Stage locally, upload at Finalize. The staged name keeps the key's
			// extension so any format-sniffing on the far side still works.
			staged := filepath.Join(os.TempDir(),
				fmt.Sprintf("stochadex-s3-%s", filepath.Base(key)))
			inner, err := localSinkForFormat(format, staged, spec)
			if err != nil {
				return nil, err
			}
			return &s3Output{
				inner: inner, staged: staged, bucket: bucket, key: key, spec: spec,
			}, nil
		},
	)
}

// loadByFormat dispatches a downloaded object to the loader for its declared format.
func loadByFormat(
	format string,
	fields map[string]interface{},
) (*simulator.StateTimeStorage, error) {
	switch format {
	case "arrow":
		return loadArrowStorage(fields["path"].(string))
	case "csv", "json_log":
		// Reuse the local loaders' own field handling (time_column, state_columns,
		// skip_header) verbatim rather than duplicating it here.
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
		switch name {
		case "bucket", "key", "format", "region", "endpoint":
		default:
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

// s3Output writes the run through a local sink and uploads the finished object once,
// at Finalize. It implements simulator.FinalizingOutputFunction: uploading per step
// would be one PUT per row.
type s3Output struct {
	inner  simulator.OutputFunction
	staged string
	bucket string
	key    string
	spec   simulator.ComponentSpec
}

func (o *s3Output) Configure(settings *simulator.Settings) { o.inner.Configure(settings) }

func (o *s3Output) Output(name string, state []float64, t float64) {
	o.inner.Output(name, state, t)
}

func (o *s3Output) Finalize() {
	// Let the inner sink seal its file first — an Arrow buffer is not a readable file
	// until it has been finalized, so uploading before this would ship a truncated object.
	if f, ok := o.inner.(simulator.FinalizingOutputFunction); ok {
		f.Finalize()
	}
	defer os.Remove(o.staged)

	client, err := newS3Client(fieldsOf(o.spec))
	if err != nil {
		fmt.Fprintf(os.Stderr, "stochadex: s3 output: %v\n", err)
		return
	}
	file, err := os.Open(o.staged)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stochadex: s3 output: opening staged file: %v\n", err)
		return
	}
	defer file.Close()

	uploader := manager.NewUploader(client)
	if _, err := uploader.Upload(context.Background(), &s3.PutObjectInput{
		Bucket: &o.bucket, Key: &o.key, Body: file,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "stochadex: s3 output: uploading s3://%s/%s: %v\n",
			o.bucket, o.key, err)
		return
	}
	fmt.Fprintf(os.Stderr, "stochadex: wrote s3://%s/%s\n", o.bucket, o.key)
}

// downloadS3Object fetches an object to a temporary file, returning its path and a
// cleanup func.
func downloadS3Object(
	fields map[string]interface{},
	bucket, key string,
) (string, func(), error) {
	client, err := newS3Client(fields)
	if err != nil {
		return "", func() {}, err
	}
	file, err := os.CreateTemp("", "stochadex-s3-*"+filepath.Ext(key))
	if err != nil {
		return "", func() {}, fmt.Errorf("s3 source: creating temp file: %w", err)
	}
	cleanup := func() { file.Close(); os.Remove(file.Name()) }

	downloader := manager.NewDownloader(client)
	if _, err := downloader.Download(context.Background(), file, &s3.GetObjectInput{
		Bucket: &bucket, Key: &key,
	}); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf(
			"s3 source: downloading s3://%s/%s: %w", bucket, key, err)
	}
	return file.Name(), cleanup, nil
}

// newS3Client builds a client from the standard AWS credential chain, honouring optional
// region and endpoint overrides. Nothing secret is read from the stochadex config.
func newS3Client(fields map[string]interface{}) (*s3.Client, error) {
	var options []func(*awsconfig.LoadOptions) error
	if region, ok := fields["region"].(string); ok && region != "" {
		options = append(options, awsconfig.WithRegion(region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), options...)
	if err != nil {
		return nil, fmt.Errorf(
			"loading AWS config (credentials come from the environment, shared config "+
				"or an IAM role): %w", err)
	}
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint, ok := fields["endpoint"].(string); ok && endpoint != "" {
			o.BaseEndpoint = &endpoint
			// Path-style addressing is what S3-compatible stores (MinIO, Ceph) expect;
			// virtual-host style assumes bucket-as-subdomain of the real AWS endpoint.
			o.UsePathStyle = true
		}
	}), nil
}

func fieldsOf(spec simulator.ComponentSpec) map[string]interface{} {
	if spec.Fields == nil {
		return map[string]interface{}{}
	}
	return spec.Fields
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
