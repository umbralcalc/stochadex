// Package s3store is the opt-in object-storage integration for the simulation's data
// boundary: it moves runs between the engine and Amazon S3, or any S3-compatible store
// (MinIO, Cloudflare R2, Ceph). It is a SEPARATE Go module
// (github.com/umbralcalc/stochadex/pkg/s3store) so that the AWS SDK and its dependency
// tree — around fifteen modules — stay entirely out of the core engine's go.mod. The
// engine module remains lean; you opt in only by importing this module.
//
// # A transport, not a format
//
// This is the design decision that shapes the whole package. Object storage is orthogonal
// to what the bytes contain, so s3store does not know how to parse anything. It moves whole
// objects, and the caller pairs it with whichever reader or sink already handles that
// format — analysis.NewStateTimeStorageFromCsv, an Arrow reader, a JSON-log sink.
//
// The alternative — an "S3 source" that reimplements CSV, JSON-log and Arrow handling
// against S3 — would need extending again for every future format, and would duplicate
// field handling (a CSV's time column and state columns) that already exists. Moving bytes
// instead means every present and future format works over object storage for free.
//
// # Credentials
//
// Credentials are never taken from configuration. They come from the standard AWS chain:
// environment variables, the shared config and credentials files, or an instance or pod IAM
// role. A stochadex config therefore names only a bucket and key, and can be committed and
// shared without leaking anything.
//
// Config carries the two optional overrides. Region overrides what the SDK would resolve.
// Endpoint points the client at an S3-compatible store, and also switches on path-style
// addressing, which those stores expect — virtual-host style assumes the bucket is a
// subdomain of a real AWS endpoint.
//
// # Reading
//
// Fetch downloads an object to a temporary file and returns its path with a cleanup
// function. Hand the path to the reader for the object's format:
//
//	client, err := s3store.NewClient(ctx, s3store.Config{Region: "eu-west-2"})
//	path, cleanup, err := s3store.Fetch(ctx, client, "my-bucket", "runs/in.csv")
//	defer cleanup()
//	storage, err := analysis.NewStateTimeStorageFromCsv(path, 0, columns, true)
//
// # Writing
//
// OutputFunction wraps any other sink: rows go to the inner sink, which writes to a local
// staging path, and the finished file is uploaded once the run ends. It implements
// simulator.FinalizingOutputFunction, and the transfer belongs at Finalize for two reasons —
// uploading per step would be one request per row, and a columnar file is not even readable
// until its buffers are sealed.
//
//	inner := simulator.NewJsonLogOutputFunction(staged)
//	sink := s3store.NewOutputFunction(inner, staged, "my-bucket", "runs/out.log", cfg)
//
// Finalize reports transfer errors on stderr rather than panicking: the simulation has
// already completed successfully by that point, and losing a finished run to a transfer
// failure would be worse than reporting it.
//
// # From config
//
// The distributed CLI (cmd/stochadex) registers this package against the config
// surface, so a run needs no Go at all:
//
//	data:
//	  source:
//	    s3: {bucket: my-bucket, key: runs/in.arrow, format: arrow}
//
//	simulation:
//	  output_function: {type: s3, bucket: my-bucket, key: runs/out.arrow, format: arrow}
//
// The format key names what the object contains, and the transport pairs with the existing
// reader or sink for it.
package s3store
