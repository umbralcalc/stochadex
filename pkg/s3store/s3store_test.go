package s3store

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// stagingSink is a minimal inner sink for the OutputFunction test: it collects rows and
// writes them to its staging path at Finalize, which is exactly the shape (write-once,
// on finalize) that the real columnar sinks have.
type stagingSink struct {
	path string
	rows bytes.Buffer
}

func (s *stagingSink) Configure(*simulator.Settings) {}

func (s *stagingSink) Output(name string, state []float64, t float64) {
	s.rows.WriteString(name)
}

func (s *stagingSink) Finalize() { os.WriteFile(s.path, s.rows.Bytes(), 0o644) }

// testConfig points at the S3-compatible server CI provides (MinIO). Credentials come from
// the standard AWS chain, so CI sets AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY as usual —
// this package never reads them itself.
func testConfig(t *testing.T) (Config, string) {
	t.Helper()
	endpoint := os.Getenv("S3STORE_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("set S3STORE_TEST_ENDPOINT to an S3-compatible server (CI runs MinIO) " +
			"to exercise real transfers")
	}
	bucket := os.Getenv("S3STORE_TEST_BUCKET")
	if bucket == "" {
		bucket = "stochadex-test"
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}
	return Config{Region: region, Endpoint: endpoint}, bucket
}

// TestS3StoreRoundTrip is the live-endpoint check: it proves bytes actually move, which
// compilation and config-validation tests cannot. Everything else about this package is
// plumbing around these two transfers.
func TestS3StoreRoundTrip(t *testing.T) {
	config, bucket := testConfig(t)
	ctx := context.Background()

	client, err := NewClient(ctx, config)
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	t.Run("Upload then Fetch returns the same bytes", func(t *testing.T) {
		want := []byte("time,walk\n0,0.0\n1,1.5\n")
		local := filepath.Join(t.TempDir(), "in.csv")
		if err := os.WriteFile(local, want, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := Upload(ctx, client, bucket, "roundtrip/in.csv", local); err != nil {
			t.Fatalf("Upload: %v", err)
		}

		fetched, cleanup, err := Fetch(ctx, client, bucket, "roundtrip/in.csv")
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		defer cleanup()

		got, err := os.ReadFile(fetched)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("round-tripped bytes differ:\n got %q\nwant %q", got, want)
		}
		// The temp file must keep the key's extension, since downstream readers may
		// sniff on it.
		if ext := filepath.Ext(fetched); ext != ".csv" {
			t.Errorf("fetched temp file has extension %q, want .csv", ext)
		}
	})

	t.Run("Fetch of a missing key errors and names the object", func(t *testing.T) {
		_, cleanup, err := Fetch(ctx, client, bucket, "definitely/absent.csv")
		defer cleanup()
		if err == nil {
			t.Fatal("expected an error fetching a missing key")
		}
		if !bytes.Contains([]byte(err.Error()), []byte("absent.csv")) {
			t.Errorf("error should name the object, got: %v", err)
		}
	})

	t.Run("OutputFunction uploads the run at Finalize", func(t *testing.T) {
		staged := filepath.Join(t.TempDir(), "out.log")
		inner := &stagingSink{path: staged}
		sink := NewOutputFunction(inner, staged, bucket, "roundtrip/out.log", config)

		sink.Configure(nil)
		sink.Output("walk", []float64{1.0}, 0.0)
		sink.Output("walk", []float64{2.0}, 1.0)

		// Nothing should exist remotely until Finalize — the whole point of deferring
		// the transfer.
		if _, _, err := Fetch(ctx, client, bucket, "roundtrip/out.log"); err == nil {
			t.Error("object existed before Finalize; the upload is not deferred")
		}

		sink.Finalize()

		fetched, cleanup, err := Fetch(ctx, client, bucket, "roundtrip/out.log")
		if err != nil {
			t.Fatalf("Fetch after Finalize: %v", err)
		}
		defer cleanup()
		got, err := os.ReadFile(fetched)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "walkwalk" {
			t.Errorf("uploaded content = %q, want the inner sink's output", got)
		}
		// Finalize must clean up after itself rather than leaving staging files behind.
		if _, err := os.Stat(staged); !os.IsNotExist(err) {
			t.Errorf("staged file %s still exists after Finalize", staged)
		}
	})
}
