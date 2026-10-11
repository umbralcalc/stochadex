package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// feedSource is a registered stream transport for these tests: it sends each
// of its levels as a message for the stream partition "level", then waits to
// be closed.
type feedSource struct {
	levels []float64
	stop   chan struct{}
}

// closedFeeds counts feedSources closed, so a test can tell a run closed its
// stream.
var closedFeeds atomic.Int64

func (f *feedSource) Read(deliver func(message []byte) error) {
	for _, level := range f.levels {
		message, _ := json.Marshal(simulator.JsonLogEntry{PartitionName: "level", State: []float64{level}})
		if deliver(message) != nil {
			return
		}
	}
	<-f.stop
}

func (f *feedSource) Close() error {
	close(f.stop)
	closedFeeds.Add(1)
	return nil
}

func feedLevels(fields map[string]interface{}) ([]float64, error) {
	raw, ok := fields["levels"].([]interface{})
	if !ok || len(raw) == 0 {
		return nil, errors.New("needs levels: [...]")
	}
	levels := make([]float64, len(raw))
	for i, value := range raw {
		number, ok := value.(float64)
		if !ok {
			return nil, fmt.Errorf("level %v is not a number", value)
		}
		levels[i] = number
	}
	return levels, nil
}

func init() {
	RegisterStreamTransport("test_feed", StreamTransport{
		Check: func(fields map[string]interface{}) error {
			_, err := feedLevels(fields)
			return err
		},
		Open: func(fields map[string]interface{}) (StreamSource, error) {
			if fields["refuse"] == true {
				return nil, errors.New("the feed is down")
			}
			levels, err := feedLevels(fields)
			return &feedSource{levels: levels, stop: make(chan struct{})}, err
		},
	})
}

func TestRegisteredStreamTransports(t *testing.T) {
	t.Run("a registered transport steers a run, and replays exactly from its recording", func(t *testing.T) {
		record := filepath.Join(t.TempDir(), "feed.log")
		before := closedFeeds.Load()
		live, err := RunToStorage(writeConfig(t, streamConfigYAML(fmt.Sprintf(
			"    stream: {test_feed: {levels: [2.0, 5.0]}}\n    record: %q", record), 300)))
		if err != nil {
			t.Fatal(err)
		}
		if closedFeeds.Load() != before+1 {
			t.Error("the run did not close its stream")
		}
		dial := live.Storage.GetValues("dial")
		if last := dial[len(dial)-1][0]; last != 5 {
			t.Fatalf("the run ended on level %v, want the stream's last, 5", last)
		}
		for k := 2; k < len(dial); k++ {
			if dial[k][0] < dial[k-1][0] {
				t.Fatalf("the level fell from %v to %v at step %d; it should hold the newest", dial[k-1][0], dial[k][0], k)
			}
		}
		replay, err := RunToStorage(writeConfig(t, streamConfigYAML(fmt.Sprintf(
			"    source: {json_log: {path: %q}}", record), 300)))
		if err != nil {
			t.Fatal(err)
		}
		assertStoragesEqual(t, "replay vs live", replay.Storage, live.Storage)
	})

	cases := []struct{ name, stream, want string }{
		{"fields its transport rejects", "{test_feed: {levels: []}}",
			`input "feed": stream test_feed: needs levels: [...]`},
		{"an unknown transport", "{carrier_pigeon: {}}",
			`input "feed": unknown stream transport "carrier_pigeon" (this binary has: websocket, connection, `},
		{"two transports", `{test_feed: {levels: [1.0]}, websocket: {url: "ws://localhost:1"}}`,
			"stream: needs one transport"},
	}
	for _, c := range cases {
		t.Run(c.name+" is a config error at load", func(t *testing.T) {
			_, err := LoadConfig(writeConfigPath(t, streamConfigYAML("    stream: "+c.stream, 5)))
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}

	t.Run("a transport that will not open is unavailable", func(t *testing.T) {
		_, err := RunToStorage(writeConfig(t, streamConfigYAML(
			"    stream: {test_feed: {levels: [1.0], refuse: true}}", 5)))
		if KindOf(err) != ErrUnavailable || !strings.Contains(err.Error(), "opening test_feed: the feed is down") {
			t.Errorf("expected ErrUnavailable naming the transport, got %v", err)
		}
	})

	t.Run("a served run cannot share a transport the process opens once", func(t *testing.T) {
		yaml := servedYAML(streamConfigYAML("    stream: {test_feed: {levels: [1.0]}}", 5), "127.0.0.1:0")
		_, err := LoadConfig(writeConfigPath(t, yaml))
		if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), "a served run, one per connection, cannot share") {
			t.Errorf("expected a config error, got %v", err)
		}
	})

	t.Run("the manifest names the transport, and the run is not cacheable", func(t *testing.T) {
		pinnedBuild(t, "pinned")
		config, err := LoadConfig(writeConfigPath(t, streamConfigYAML("    stream: {test_feed: {levels: [1.0]}}", 5)))
		if err != nil {
			t.Fatal(err)
		}
		if input := Manifest(config).Inputs[0]; input.Kind != "stream" || input.Location != "test_feed" {
			t.Errorf("manifest input %+v", input)
		}
		provenance, err := ComputeProvenance(config)
		if err != nil || provenance.Key != "" {
			t.Errorf("a live stream should leave the run without a key, got %q (%v)", provenance.Key, err)
		}
	})

	t.Run("names the engine owns, or already registered, cannot be registered again", func(t *testing.T) {
		for _, name := range []string{"websocket", "connection", "test_feed"} {
			func() {
				defer func() {
					if recover() == nil {
						t.Errorf("registering %q should panic", name)
					}
				}()
				RegisterStreamTransport(name, StreamTransport{})
			}()
		}
	})
}

func TestBatchPacing(t *testing.T) {
	yaml := checkYAML(walkPartition("w", ""), "run: {pace_ms: 30}\n")
	path := writeConfigPath(t, yaml)
	start := time.Now()
	captureStdout(t, func() {
		if err := Execute([]string{"stochadex", "--config", path}); err != nil {
			t.Fatal(err)
		}
	})
	if elapsed := time.Since(start); elapsed < 5*30*time.Millisecond {
		t.Errorf("5 steps paced at 30ms took %v from the CLI", elapsed)
	}
	start = time.Now()
	if _, err := RunToStorage(writeConfig(t, yaml)); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed >= 5*30*time.Millisecond {
		t.Errorf("RunToStorage returns results and should run unpaced, took %v", elapsed)
	}
}
