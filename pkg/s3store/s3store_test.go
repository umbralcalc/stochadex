package s3store

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
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

// testConfig starts an in-process S3 server and points a Config at it.
//
// Deliberately NOT a container: a service image is an external dependency that can be
// re-licensed, renamed or abandoned out from under the test, and it forces CI-only
// execution plus health-check waiting. An in-process server has none of that, runs on a
// developer machine with a plain `go test`, and still exercises the real SDK path — actual
// HTTP requests, real request signing and response parsing — which is what this package's
// correctness depends on.
//
// S3STORE_TEST_ENDPOINT overrides it, so the same test can be aimed at a real S3 bucket or
// any S3-compatible server when you want to confirm against the genuine article.
func testConfig(t *testing.T) (Config, string) {
	t.Helper()
	const bucket = "stochadex-test"

	if endpoint := os.Getenv("S3STORE_TEST_ENDPOINT"); endpoint != "" {
		name := os.Getenv("S3STORE_TEST_BUCKET")
		if name == "" {
			name = bucket
		}
		region := os.Getenv("AWS_REGION")
		if region == "" {
			region = "us-east-1"
		}
		return Config{Region: region, Endpoint: endpoint}, name
	}

	backend := s3mem.New()
	if err := backend.CreateBucket(bucket); err != nil {
		t.Fatalf("creating test bucket: %v", err)
	}
	server := httptest.NewServer(gofakes3.New(backend).Server())
	t.Cleanup(server.Close)

	// The SDK still signs requests, so it needs credentials present; they are never read
	// by this package itself.
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	return Config{Region: "us-east-1", Endpoint: server.URL}, bucket
}

// TestS3StoreRoundTrip proves bytes actually move over the S3 API — which compilation and
// config-validation tests cannot show. Everything else in this package is plumbing around
// these two transfers.
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

	t.Run("Version changes when the object is rewritten, and not otherwise", func(t *testing.T) {
		local := filepath.Join(t.TempDir(), "in.csv")
		version := func(contents string) string {
			if contents != "" {
				os.WriteFile(local, []byte(contents), 0o644)
				if err := Upload(ctx, client, bucket, "versioned/in.csv", local); err != nil {
					t.Fatal(err)
				}
			}
			v, err := Version(ctx, client, bucket, "versioned/in.csv")
			if err != nil {
				t.Fatal(err)
			}
			return v
		}
		first := version("0,1.0\n")
		if again := version(""); again != first {
			t.Errorf("an unchanged object's version moved: %s then %s", first, again)
		}
		if changed := version("0,2.0\n"); changed == first {
			t.Errorf("a rewritten object kept its version %s", first)
		}
		if _, err := Version(ctx, client, bucket, "versioned/absent.csv"); err == nil {
			t.Error("a missing object should have no version")
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

	t.Run("a staged OutputFunction uploads only at Commit, and never after Abort", func(t *testing.T) {
		var _ simulator.StagedOutputFunction = (*OutputFunction)(nil)
		run := func(key string) (*OutputFunction, string) {
			staged := filepath.Join(t.TempDir(), "out.log")
			sink := NewOutputFunction(&stagingSink{path: staged}, staged, bucket, key, config)
			sink.Stage()
			sink.Configure(nil)
			sink.Output("walk", []float64{1.0}, 0.0)
			sink.Finalize()
			return sink, staged
		}
		exists := func(key string) bool {
			_, cleanup, err := Fetch(ctx, client, bucket, key)
			cleanup()
			return err == nil
		}

		committed, _ := run("staged/committed.log")
		if exists("staged/committed.log") {
			t.Fatal("a staged run was uploaded at Finalize")
		}
		if err := committed.Commit(); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		if !exists("staged/committed.log") {
			t.Error("Commit did not upload the run")
		}

		aborted, staged := run("staged/aborted.log")
		aborted.Abort()
		if exists("staged/aborted.log") {
			t.Error("an aborted run was uploaded")
		}
		if _, err := os.Stat(staged); !os.IsNotExist(err) {
			t.Error("Abort left the staged file behind")
		}
	})

	t.Run("a staged upload that fails is returned from Commit", func(t *testing.T) {
		staged := filepath.Join(t.TempDir(), "out.log")
		sink := NewOutputFunction(&stagingSink{path: staged}, staged, "no-such-bucket", "x.log", config)
		sink.Stage()
		sink.Configure(nil)
		sink.Finalize()
		if err := sink.Commit(); err == nil {
			t.Error("an upload to a missing bucket should fail the commit")
		}
	})
}
