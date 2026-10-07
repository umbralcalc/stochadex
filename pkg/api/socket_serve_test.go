package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

// keyByTimeAndPartition indexes streamed states by (time, partition). Partitions
// within a step are output concurrently, so position in the stream carries no
// meaning; the (time, partition) pair is the row's identity.
func keyByTimeAndPartition(stream []*simulator.PartitionState) map[string][]float64 {
	byKey := make(map[string][]float64, len(stream))
	for _, state := range stream {
		byKey[fmt.Sprintf("%v/%s", state.CumulativeTimesteps, state.PartitionName)] =
			state.State
	}
	return byKey
}

// referenceRun runs the config once offline into storage and keys every
// recorded row the same way, as the ground truth a served stream must match.
func referenceRun(t *testing.T, path string) map[string][]float64 {
	t.Helper()
	generator := LoadApiRunConfigFromYaml(path).GetConfigGenerator()
	storage := simulator.NewStateTimeStorage()
	simulation := generator.GetSimulation()
	simulation.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: storage}
	generator.SetSimulation(simulation)
	simulator.NewPartitionCoordinator(generator.GenerateConfigs()).Run()
	byKey := make(map[string][]float64)
	times := storage.GetTimes()
	for _, name := range storage.GetNames() {
		for step, row := range storage.GetValues(name) {
			byKey[fmt.Sprintf("%v/%s", times[step], name)] = row
		}
	}
	return byKey
}

// assertMatchesReference fails unless the stream holds exactly the reference
// rows, value for value.
func assertMatchesReference(
	t *testing.T,
	label string,
	stream []*simulator.PartitionState,
	reference map[string][]float64,
) {
	t.Helper()
	got := keyByTimeAndPartition(stream)
	if len(stream) != len(reference) || len(got) != len(reference) {
		t.Fatalf("%s: got %d messages (%d distinct rows), want %d",
			label, len(stream), len(got), len(reference))
	}
	for key, want := range reference {
		if values, ok := got[key]; !ok || !floats.Equal(values, want) {
			t.Fatalf("%s: row %s = %v, want %v", label, key, values, want)
		}
	}
}

// reloadBuild is a NewWebsocketHandler build that re-loads the config file for
// every connection, so concurrent clients never share iteration instances.
func reloadBuild(path string) func() *simulator.ConfigGenerator {
	return func() *simulator.ConfigGenerator {
		return LoadApiRunConfigFromYaml(path).GetConfigGenerator()
	}
}

func writeServeConfig(t *testing.T, yaml string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "serve.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func wsURLOf(server *httptest.Server) string {
	return "ws" + strings.TrimPrefix(server.URL, "http")
}

func TestWebsocketServing(t *testing.T) {
	path := writeServeConfig(t, serveConfigYAML)
	reference := referenceRun(t, path)

	t.Run("a single client receives exactly the offline run", func(t *testing.T) {
		server := httptest.NewServer(NewWebsocketHandler(reloadBuild(path), 0, nil))
		defer server.Close()
		// the initial state plus 40 steps, for 2 partitions
		if len(reference) != 82 {
			t.Fatalf("reference run has %d rows, want 82", len(reference))
		}
		assertMatchesReference(t, "client", readStream(t, wsURLOf(server)), reference)
	})

	t.Run("concurrent clients each receive exactly the offline run", func(t *testing.T) {
		// Each connection must be built afresh: count the builds, and check every
		// client's stream against the offline run rather than only against each
		// other, so identical-but-wrong streams cannot pass.
		inner := reloadBuild(path)
		var builds atomic.Int32
		build := func() *simulator.ConfigGenerator {
			builds.Add(1)
			return inner()
		}
		// a 1ms step delay keeps the runs in flight at once, so any iteration
		// state shared between connections would interleave their draws
		server := httptest.NewServer(NewWebsocketHandler(build, 1, nil))
		defer server.Close()

		const clients = 3
		streams := make([][]*simulator.PartitionState, clients)
		var wg sync.WaitGroup
		for i := range clients {
			wg.Add(1)
			go func() {
				defer wg.Done()
				streams[i] = readStream(t, wsURLOf(server))
			}()
		}
		wg.Wait()

		if got := builds.Load(); got != clients {
			t.Errorf("expected one build per connection (%d), got %d", clients, got)
		}
		for i, stream := range streams {
			assertMatchesReference(t, fmt.Sprintf("client %d", i), stream, reference)
		}
	})

	t.Run("the run stops when the client disconnects", func(t *testing.T) {
		// A run that would take ~1000s to finish: the handler must return soon
		// after the client goes away rather than stepping to termination.
		long := strings.Replace(serveConfigYAML, "max_steps: 40", "max_steps: 1000000", 1)
		build := reloadBuild(writeServeConfig(t, long))
		handlerDone := make(chan struct{})
		handler := NewWebsocketHandler(build, 1, nil)
		server := httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				defer close(handlerDone)
				handler.ServeHTTP(w, r)
			},
		))
		defer server.Close()

		connection, _, err := websocket.DefaultDialer.Dial(wsURLOf(server), nil)
		if err != nil {
			t.Fatal(err)
		}
		for range 5 {
			if _, _, err := connection.ReadMessage(); err != nil {
				t.Fatalf("reading before disconnect: %v", err)
			}
		}
		connection.Close()

		select {
		case <-handlerDone:
		case <-time.After(5 * time.Second):
			t.Fatal("handler kept stepping after the client disconnected")
		}
	})

	t.Run("an in-memory config cannot be served", func(t *testing.T) {
		if _, err := serveHandler(&ApiRunConfig{}); err == nil {
			t.Error("expected an error serving a config with no source file")
		}
	})
}

func TestWebsocketOriginEnforcement(t *testing.T) {
	path := writeServeConfig(t, serveConfigYAML)
	build := reloadBuild(path)
	dialWithOrigin := func(server *httptest.Server, origin string) (*http.Response, error) {
		header := http.Header{}
		header.Set("Origin", origin)
		connection, response, err := websocket.DefaultDialer.Dial(wsURLOf(server), header)
		if err == nil {
			connection.Close()
		}
		return response, err
	}

	t.Run("a foreign browser origin is refused at upgrade", func(t *testing.T) {
		server := httptest.NewServer(NewWebsocketHandler(build, 0, nil))
		defer server.Close()
		response, err := dialWithOrigin(server, "https://evil.example.org")
		if err == nil {
			t.Fatal("expected the upgrade to be refused")
		}
		if response == nil || response.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden, got %v", response)
		}
	})

	t.Run("a listed origin is admitted at upgrade", func(t *testing.T) {
		server := httptest.NewServer(NewWebsocketHandler(
			build, 0, []string{"https://dash.example.org"}))
		defer server.Close()
		if _, err := dialWithOrigin(server, "https://dash.example.org"); err != nil {
			t.Errorf("expected the listed origin to connect: %v", err)
		}
	})

	t.Run("allowed_origins loads from the socket config", func(t *testing.T) {
		socketPath := filepath.Join(t.TempDir(), "socket.yaml")
		if err := os.WriteFile(socketPath, []byte(
			"address: \":2112\"\nhandle: \"/handle\"\nmillisecond_delay: 0\n"+
				"allowed_origins: [\"https://dash.example.org\", \"*\"]\n",
		), 0o644); err != nil {
			t.Fatal(err)
		}
		socket := LoadSocketConfigFromYaml(socketPath)
		want := []string{"https://dash.example.org", "*"}
		if len(socket.AllowedOrigins) != len(want) ||
			socket.AllowedOrigins[0] != want[0] || socket.AllowedOrigins[1] != want[1] {
			t.Errorf("allowed_origins = %v, want %v", socket.AllowedOrigins, want)
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
