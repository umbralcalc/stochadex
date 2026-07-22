// Package s3store is the opt-in object-storage integration: it moves simulation input and
// output between the engine and Amazon S3 (or any S3-compatible store).
//
// It is deliberately a TRANSPORT, not a format. Object storage is orthogonal to what the
// bytes contain, so rather than reimplementing CSV, JSON-log and Arrow parsing against S3 —
// and needing extension again for the next format — this package moves whole objects, and
// the caller pairs it with whichever reader or sink already handles that format. Every
// present and future format is reachable over S3 for free.
//
// Credentials are never taken from configuration. They come from the standard AWS chain:
// environment variables, shared config/credentials files, or an instance/pod IAM role.
package s3store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Config carries the optional client overrides. Both fields may be empty, in which case the
// SDK's own resolution applies.
type Config struct {
	// Region overrides the region the SDK would otherwise resolve.
	Region string
	// Endpoint points the client at an S3-compatible store — MinIO, Cloudflare R2, Ceph.
	// Setting it also switches on path-style addressing, which those stores expect;
	// virtual-host style assumes the bucket is a subdomain of a real AWS endpoint.
	Endpoint string
}

// NewClient builds an S3 client from the standard AWS credential chain, applying any
// overrides in cfg.
func NewClient(ctx context.Context, cfg Config) (*s3.Client, error) {
	var options []func(*awsconfig.LoadOptions) error
	if cfg.Region != "" {
		options = append(options, awsconfig.WithRegion(cfg.Region))
	}
	loaded, err := awsconfig.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf(
			"s3store: loading AWS config (credentials come from the environment, shared "+
				"config, or an IAM role): %w", err)
	}
	return s3.NewFromConfig(loaded, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			endpoint := cfg.Endpoint
			o.BaseEndpoint = &endpoint
			o.UsePathStyle = true
		}
	}), nil
}

// Fetch downloads an object to a temporary file and returns its path together with a
// cleanup function the caller must invoke when done. The temporary file keeps the key's
// extension, so any format sniffing downstream still works.
//
// Pair it with the reader for the object's format, e.g. analysis.NewStateTimeStorageFromCsv.
func Fetch(
	ctx context.Context,
	client *s3.Client,
	bucket, key string,
) (string, func(), error) {
	file, err := os.CreateTemp("", "stochadex-s3-*"+filepath.Ext(key))
	if err != nil {
		return "", func() {}, fmt.Errorf("s3store: creating temp file: %w", err)
	}
	cleanup := func() { file.Close(); os.Remove(file.Name()) }

	if _, err := manager.NewDownloader(client).Download(ctx, file, &s3.GetObjectInput{
		Bucket: &bucket, Key: &key,
	}); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf(
			"s3store: downloading s3://%s/%s: %w", bucket, key, err)
	}
	return file.Name(), cleanup, nil
}

// Upload copies a local file to an object.
func Upload(
	ctx context.Context,
	client *s3.Client,
	bucket, key, path string,
) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("s3store: opening %s: %w", path, err)
	}
	defer file.Close()

	if _, err := manager.NewUploader(client).Upload(ctx, &s3.PutObjectInput{
		Bucket: &bucket, Key: &key, Body: file,
	}); err != nil {
		return fmt.Errorf("s3store: uploading s3://%s/%s: %w", bucket, key, err)
	}
	return nil
}

// OutputFunction wraps any other sink so a run lands in object storage: rows go to the
// inner sink, which writes to a local staging path, and the finished file is uploaded once
// the run ends.
//
// It implements simulator.FinalizingOutputFunction. Uploading per step would be one request
// per row, and a columnar file is not even readable until its buffers are sealed — so the
// transfer belongs at Finalize, after the inner sink has finalized itself.
type OutputFunction struct {
	inner  simulator.OutputFunction
	staged string
	bucket string
	key    string
	config Config
}

// NewOutputFunction wraps inner, which must write to the file at staged.
func NewOutputFunction(
	inner simulator.OutputFunction,
	staged, bucket, key string,
	config Config,
) *OutputFunction {
	return &OutputFunction{
		inner: inner, staged: staged, bucket: bucket, key: key, config: config,
	}
}

func (o *OutputFunction) Configure(settings *simulator.Settings) {
	o.inner.Configure(settings)
}

func (o *OutputFunction) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
	o.inner.Output(partitionName, state, cumulativeTimesteps)
}

// Finalize seals the inner sink, uploads the staged file, and removes it. Errors are
// reported on stderr rather than panicking: the simulation itself has already completed
// successfully by this point, and losing the run to a transfer failure would be worse than
// reporting it.
func (o *OutputFunction) Finalize() {
	if f, ok := o.inner.(simulator.FinalizingOutputFunction); ok {
		f.Finalize()
	}
	defer os.Remove(o.staged)

	ctx := context.Background()
	client, err := NewClient(ctx, o.config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stochadex: %v\n", err)
		return
	}
	if err := Upload(ctx, client, o.bucket, o.key, o.staged); err != nil {
		fmt.Fprintf(os.Stderr, "stochadex: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "stochadex: wrote s3://%s/%s\n", o.bucket, o.key)
}
