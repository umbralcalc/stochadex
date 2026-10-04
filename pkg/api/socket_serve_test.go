package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"google.golang.org/protobuf/proto"
)

const serveConfigYAML = `main:
  partitions:
  - name: first_wiener_process
    iteration: {type: wiener_process}
    params:
      variances: [1.0, 1.0]
    init_state_values: [0.0, 0.0]
    state_history_depth: 1
    seed: 7167
  - name: second_wiener_process
    iteration: {type: wiener_process}
    params:
      variances: [1.0]
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 2939
  simulation:
    output_condition: {type: every_step}
    output_function: {type: nil}
    termination_condition: {type: number_of_steps, max_steps: 40}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`

// readStream connects to the server and collects every PartitionState it sends
// until the server closes the connection at the end of the run.
func readStream(t *testing.T, wsURL string) []*simulator.PartitionState {
	t.Helper()
	connection, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Errorf("dial: %v", err)
		return nil
	}
	defer connection.Close()
	states := make([]*simulator.PartitionState, 0)
	for {
		_, data, err := connection.ReadMessage()
		if err != nil {
			return states
		}
		state := &simulator.PartitionState{}
		if err := proto.Unmarshal(data, state); err != nil {
			t.Errorf("unmarshal: %v", err)
			return states
		}
		states = append(states, state)
	}
}

func TestWebsocketServing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serve.yaml")
	if err := os.WriteFile(path, []byte(serveConfigYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	config := LoadApiRunConfigFromYaml(path)

	t.Run("concurrent clients each get an isolated, identical run", func(t *testing.T) {
		build, err := perConnectionBuild(config)
		if err != nil {
			t.Fatal(err)
		}
		// a 1ms step delay keeps both runs in flight at once, so shared
		// iteration state between connections would interleave their draws
		server := httptest.NewServer(NewWebsocketHandler(build, 1, nil))
		defer server.Close()
		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

		const clients = 3
		streams := make([][]*simulator.PartitionState, clients)
		var wg sync.WaitGroup
		for i := range clients {
			wg.Add(1)
			go func() {
				defer wg.Done()
				streams[i] = readStream(t, wsURL)
			}()
		}
		wg.Wait()

		// the initial state plus 40 steps, for 2 partitions
		if len(streams[0]) != 82 {
			t.Fatalf("expected 82 messages, got %d", len(streams[0]))
		}
		// partitions within a step are output concurrently, so compare each
		// client's stream keyed by (time, partition) rather than by position
		keyed := func(stream []*simulator.PartitionState) map[string][]float64 {
			byKey := make(map[string][]float64, len(stream))
			for _, state := range stream {
				byKey[fmt.Sprintf("%v/%s", state.CumulativeTimesteps,
					state.PartitionName)] = state.State
			}
			return byKey
		}
		reference := keyed(streams[0])
		for i := 1; i < clients; i++ {
			other := keyed(streams[i])
			if len(other) != len(reference) {
				t.Fatalf("client %d got %d distinct rows, client 0 got %d",
					i, len(other), len(reference))
			}
			for key, want := range reference {
				if got, ok := other[key]; !ok || !floats.Equal(got, want) {
					t.Fatalf("client %d diverged from client 0 at %s: %v vs %v",
						i, key, got, want)
				}
			}
		}
	})

	t.Run("an in-memory config cannot be served", func(t *testing.T) {
		if _, err := perConnectionBuild(&ApiRunConfig{}); err == nil {
			t.Error("expected an error serving a config with no source file")
		}
	})
}

func TestWebsocketOriginCheck(t *testing.T) {
	cases := []struct {
		name    string
		origin  string
		allowed []string
		want    bool
	}{
		{"no origin header (non-browser client)", "", nil, true},
		{"same origin", "http://sim.example.com:2112", nil, true},
		{"localhost on another port", "http://localhost:5173", nil, true},
		{"loopback ip", "http://127.0.0.1:8080", nil, true},
		{"foreign origin", "https://evil.example.org", nil, false},
		{"foreign origin listed", "https://dash.example.org", []string{"https://dash.example.org"}, true},
		{"wildcard", "https://anything.example.org", []string{"*"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			request := &http.Request{Host: "sim.example.com:2112", Header: http.Header{}}
			if c.origin != "" {
				request.Header.Set("Origin", c.origin)
			}
			if got := websocketOriginCheck(c.allowed)(request); got != c.want {
				t.Errorf("origin %q: got %v, want %v", c.origin, got, c.want)
			}
		})
	}
}
