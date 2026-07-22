# s3store — opt-in object-storage ingress/egress

Move simulation runs between the engine and **Amazon S3**, or any S3-compatible store
(MinIO, Cloudflare R2, Ceph). It is a **separate Go module** so the AWS SDK and its
dependency tree — around fifteen modules — stay entirely out of the core engine's
`go.mod`; the engine stays lean and you opt in only by importing this module.

```bash
go get github.com/umbralcalc/stochadex/pkg/s3store
```

## A transport, not a format

This package does not know how to parse anything. Object storage is orthogonal to what the
bytes contain, so it moves **whole objects** and you pair it with whichever reader or sink
already handles that format.

The alternative — an "S3 source" reimplementing CSV, JSON-log and Arrow handling against S3
— would need extending for every future format and would duplicate field handling that
already exists. Moving bytes instead means **every present and future format works over
object storage for free**.

## Credentials

Never taken from configuration. They come from the standard AWS chain: environment
variables, the shared config/credentials files, or an instance/pod IAM role. A stochadex
config therefore names only a bucket and key, and can be committed and shared safely.

`Config` carries the two optional overrides:

| Field | Purpose |
|---|---|
| `Region` | Overrides the region the SDK would otherwise resolve. |
| `Endpoint` | Points at an S3-compatible store. Also switches on path-style addressing, which those stores expect — virtual-host style assumes the bucket is a subdomain of a real AWS endpoint. |

## Reading

`Fetch` downloads an object to a temporary file and returns its path plus a cleanup
function. Hand the path to the reader for that format:

```go
client, err := s3store.NewClient(ctx, s3store.Config{Region: "eu-west-2"})
path, cleanup, err := s3store.Fetch(ctx, client, "my-bucket", "runs/in.csv")
defer cleanup()
storage, err := analysis.NewStateTimeStorageFromCsv(path, 0, columns, true)
```

## Writing

`OutputFunction` wraps any other sink: rows go to the inner sink, which writes to a local
staging path, and the finished file is uploaded once the run ends.

```go
inner := simulator.NewJsonLogOutputFunction(staged)
sink := s3store.NewOutputFunction(inner, staged, "my-bucket", "runs/out.log", cfg)
```

It implements `simulator.FinalizingOutputFunction`, and the transfer belongs at `Finalize`
for two reasons: uploading per step would be one request per row, and a columnar file is not
even readable until its buffers are sealed. Transfer errors are reported on stderr rather
than panicking — the simulation has already completed by then, and losing a finished run to
a transfer failure would be worse than reporting it.

## From config (no Go)

The distributed CLI registers this package against the config surface:

```yaml
data:
  source:
    s3: {bucket: my-bucket, key: runs/in.arrow, format: arrow}

simulation:
  output_function: {type: s3, bucket: my-bucket, key: runs/out.arrow, format: arrow}
```

`format:` names what the object contains (`arrow`, `csv` or `json_log` for sources; `arrow`
or `json_log` for sinks), and the transport pairs with the existing reader/sink for it.
Optional `region:` and `endpoint:` map to the `Config` fields above.
