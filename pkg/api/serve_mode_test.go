package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// serveRun is a run: block serving on address.
func serveRun(address string) string {
	return fmt.Sprintf("run: {mode: serve, websocket: {address: %q, handle: /handle}}\n", address)
}

// servedHandler loads a serve config and returns an httptest server for its
// handler, the same handler runServe mounts.
func servedHandler(t *testing.T, yaml string) *httptest.Server {
	t.Helper()
	handler, err := serveHandler(writeConfig(t, yaml))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

// capturedRun runs the batch twin of a served config (the same model with no
// run: block) into an in-memory view selected by condition (every step when
// nil) and keys its rows by (time, partition).
func capturedRun(t *testing.T, yaml string, condition simulator.OutputCondition) map[string][]float64 {
	t.Helper()
	result, err := RunWith(writeConfig(t, yaml), CaptureView("view", condition))
	if err != nil {
		t.Fatal(err)
	}
	return storageEntries(result.Views["view"])
}

// freeAddress returns a loopback address with a port nothing is listening on.
func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().String()
}

// executeServing starts Execute in the background (it serves until the process
// ends) and waits, boundedly, until address accepts connections. A serve that
// fails to start is reported rather than waited on.
func executeServing(t *testing.T, address string, args ...string) {
	t.Helper()
	failed := make(chan error, 1)
	go func() { failed <- Execute(append([]string{"stochadex"}, args...)) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		select {
		case err := <-failed:
			t.Fatalf("Execute returned instead of serving: %v", err)
		default:
		}
		if connection, err := net.Dial("tcp", address); err == nil {
			connection.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("nothing listening on %s", address)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestServeMode(t *testing.T) {
	served := serveConfigYAML + serveRun("127.0.0.1:0")

	t.Run("a connection streams exactly an in-memory view of the same run", func(t *testing.T) {
		want := capturedRun(t, serveConfigYAML, nil)
		if len(want) != 82 {
			t.Fatalf("reference has %d rows, want 82 (41 times x 2 partitions)", len(want))
		}
		server := servedHandler(t, served)
		assertMatchesReference(t, "stream", readStream(t, wsURLOf(server)), want)
	})

	t.Run("the stream is gated by the config's output_condition", func(t *testing.T) {
		yaml := replaceOnce(t, served, "output_condition: {type: every_step}",
			"output_condition: {type: only_given_partitions, partitions: [second_wiener_process]}")
		want := capturedRun(t, serveConfigYAML, &simulator.OnlyGivenPartitionsOutputCondition{
			Partitions: map[string]bool{"second_wiener_process": true}})
		if len(want) != 41 {
			t.Fatalf("reference has %d rows, want 41", len(want))
		}
		server := servedHandler(t, yaml)
		assertMatchesReference(t, "gated stream", readStream(t, wsURLOf(server)), want)
	})

	t.Run("concurrent connections each write their own outputs: views", func(t *testing.T) {
		dir := t.TempDir()
		views := fmt.Sprintf(`outputs:
- {name: all, function: {type: json_log, path: %q}}
- {name: second, condition: {type: only_given_partitions, partitions: [second_wiener_process]}, function: {type: json_log, path: %q}}
`, filepath.Join(dir, "all-{connection}.log"), filepath.Join(dir, "second-{connection}.log"))
		shorthand := "    output_condition: {type: every_step}\n    output_function: {type: nil}\n"
		yaml := replaceOnce(t, served, shorthand, "") + views

		// Reference: the batch twin writing the same views to plain paths.
		referenceDir := t.TempDir()
		reference := replaceOnce(t, serveConfigYAML, shorthand, "") + strings.NewReplacer(
			filepath.Join(dir, "all-{connection}.log"), filepath.Join(referenceDir, "all.log"),
			filepath.Join(dir, "second-{connection}.log"), filepath.Join(referenceDir, "second.log"),
		).Replace(views)
		if _, err := RunWith(writeConfig(t, reference), WithConfigOutputs()); err != nil {
			t.Fatal(err)
		}

		server := servedHandler(t, yaml)
		const clients = 2
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
		full := keyedEntries(t, filepath.Join(referenceDir, "all.log"))
		for i, stream := range streams {
			// With outputs: views, the stream itself is every step.
			assertMatchesReference(t, fmt.Sprintf("stream %d", i), stream, full)
		}
		for connection := range clients {
			for _, view := range []string{"all", "second"} {
				assertSameEntries(t, fmt.Sprintf("connection %d's %s view", connection, view),
					keyedEntries(t, filepath.Join(dir, fmt.Sprintf("%s-%d.log", view, connection))),
					keyedEntries(t, filepath.Join(referenceDir, view+".log")))
			}
		}
	})

	t.Run("a connection's views are finalized, including when the client leaves early", func(t *testing.T) {
		// A push sink holds a connection open until it is finalized, so the
		// receiver seeing every connection end proves finalization.
		receiver, pushServer := newPushReceiver(t)
		defer pushServer.Close()
		url := "ws" + strings.TrimPrefix(pushServer.URL, "http") + "/{connection}"
		yaml := replaceOnce(t, served, "    output_condition: {type: every_step}\n    output_function: {type: nil}\n", "") +
			fmt.Sprintf("outputs:\n- {name: push, function: {type: websocket, url: %q}}\n", url)

		server := servedHandler(t, yaml)
		readStream(t, wsURLOf(server)) // runs to completion
		receiver.waitForConnections(t, 1)

		long := servedHandler(t, strings.Replace(yaml, "max_steps: 40", "max_steps: 1000000", 1))
		connection, _, err := websocket.DefaultDialer.Dial(wsURLOf(long), nil)
		if err != nil {
			t.Fatal(err)
		}
		for range 5 {
			if _, _, err := connection.ReadMessage(); err != nil {
				t.Fatal(err)
			}
		}
		connection.Close()
		receiver.waitForConnections(t, 2)
	})

	t.Run("pace_ms paces the stream", func(t *testing.T) {
		yaml := strings.Replace(replaceOnce(t, served, "handle: /handle}}",
			"handle: /handle}, pace_ms: 25}"), "max_steps: 40", "max_steps: 8", 1)
		server := servedHandler(t, yaml)
		start := time.Now()
		if n := len(readStream(t, wsURLOf(server))); n != 18 {
			t.Fatalf("got %d messages, want 18", n)
		}
		if elapsed := time.Since(start); elapsed < 8*25*time.Millisecond {
			t.Errorf("8 steps paced at 25ms took %v", elapsed)
		}
	})

	t.Run("allowed_origins comes from the config", func(t *testing.T) {
		yaml := replaceOnce(t, served, "handle: /handle}",
			`handle: /handle, allowed_origins: ["https://dash.example.org"]}`)
		server := servedHandler(t, yaml)
		dial := func(origin string) (*http.Response, error) {
			header := http.Header{}
			header.Set("Origin", origin)
			connection, response, err := websocket.DefaultDialer.Dial(wsURLOf(server), header)
			if err == nil {
				connection.Close()
			}
			return response, err
		}
		if _, err := dial("https://dash.example.org"); err != nil {
			t.Errorf("the listed origin should connect: %v", err)
		}
		if response, err := dial("https://evil.example.org"); err == nil ||
			response == nil || response.StatusCode != http.StatusForbidden {
			t.Errorf("a foreign origin should get 403, got %v, %v", response, err)
		}
	})

	t.Run("a connection whose input has gone is closed with the reason", func(t *testing.T) {
		csv := filepath.Join(t.TempDir(), "prices.csv")
		writeFile(t, csv, "0,1\n1,3\n2,2\n3,5\n")
		server := servedHandler(t, fmt.Sprintf(`inputs:
  prices:
    source: {csv: {path: %q, time_column: 0, state_columns: {price: [1]}}}
main:
  partitions:
  - {name: price, iteration: {type: from_input, input: prices}, state_history_depth: 1, seed: 0}
  simulation:
    output_condition: {type: every_step}
    output_function: {type: nil}
    termination_condition: {type: input_exhausted, input: prices}
    timestep_function: {type: from_input, input: prices}
    init_time_value: 0.0
`, csv)+serveRun("127.0.0.1:0"))
		// Positive control: while the input exists, a connection replays it.
		var got []float64
		for _, state := range readStream(t, wsURLOf(server)) {
			got = append(got, state.State...)
		}
		if !floats.Equal(got, []float64{1, 3, 2, 5}) {
			t.Fatalf("streamed %v, want the input's rows [1 3 2 5]", got)
		}

		if err := os.Remove(csv); err != nil {
			t.Fatal(err)
		}
		connection, _, err := websocket.DefaultDialer.Dial(wsURLOf(server), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer connection.Close()
		_, _, err = connection.ReadMessage()
		closeErr, ok := err.(*websocket.CloseError)
		if !ok || closeErr.Code != websocket.CloseInternalServerErr ||
			!strings.Contains(closeErr.Text, `"prices"`) {
			t.Errorf("expected a 1011 close naming the input, got %v", err)
		}
	})

	t.Run("a connection after the config file broke is closed with the reason", func(t *testing.T) {
		path := writeConfigPath(t, served)
		handler, err := serveHandler(LoadApiRunConfigFromYaml(path))
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(handler)
		defer server.Close()
		writeFile(t, path, replaceOnce(t, served, "wiener_process}", "wiener_proces}"))
		connection, _, err := websocket.DefaultDialer.Dial(wsURLOf(server), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer connection.Close()
		_, _, err = connection.ReadMessage()
		closeErr, ok := err.(*websocket.CloseError)
		if !ok || closeErr.Code != websocket.CloseInternalServerErr ||
			!strings.Contains(closeErr.Text, "wiener_proces") {
			t.Errorf("expected a 1011 close naming the bad type, got %v", err)
		}
	})

	t.Run("a long reason is cut to fit a close frame, on a character boundary", func(t *testing.T) {
		yaml := servedConfigWithLongPath(t)
		server := servedHandler(t, yaml.config)
		os.Remove(yaml.csv)
		connection, _, err := websocket.DefaultDialer.Dial(wsURLOf(server), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer connection.Close()
		_, _, err = connection.ReadMessage()
		closeErr, ok := err.(*websocket.CloseError)
		if !ok || closeErr.Code != websocket.CloseInternalServerErr ||
			len(closeErr.Text) > 120 || !utf8.ValidString(closeErr.Text) {
			t.Errorf("expected a 1011 close with a valid reason of at most 120 bytes, got %v", err)
		}
	})

	t.Run("RunWith on a serve config is a usage error", func(t *testing.T) {
		if _, err := RunWith(writeConfig(t, served)); KindOf(err) != ErrUsage {
			t.Errorf("expected ErrUsage, got %v", err)
		}
	})
}

// servedConfigWithLongPath is a served config replaying a CSV input whose path
// is long and contains multi-byte characters, so a missing-input error is
// longer than a close frame allows.
func servedConfigWithLongPath(t *testing.T) struct{ config, csv string } {
	t.Helper()
	dir := filepath.Join(t.TempDir(), strings.Repeat("é", 70))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	csv := filepath.Join(dir, "prices.csv")
	writeFile(t, csv, "0,1\n1,3\n")
	config := fmt.Sprintf(`inputs:
  prices:
    source: {csv: {path: %q, time_column: 0, state_columns: {price: [1]}}}
main:
  partitions:
  - {name: price, iteration: {type: from_input, input: prices}, state_history_depth: 1, seed: 0}
  simulation:
    output_condition: {type: every_step}
    output_function: {type: nil}
    termination_condition: {type: input_exhausted, input: prices}
    timestep_function: {type: from_input, input: prices}
    init_time_value: 0.0
`, csv) + serveRun("127.0.0.1:0")
	return struct{ config, csv string }{config, csv}
}

func TestCloseReason(t *testing.T) {
	// Byte 120 falls inside the "é" at bytes 119-120, so cutting must drop it.
	long := strings.Repeat("a", 119) + "é" + strings.Repeat("b", 10)
	if got := closeReason(errors.New(long)); got != strings.Repeat("a", 119) {
		t.Errorf("closeReason = %q (%d bytes), want the 119 a's", got, len(got))
	}
	if got := closeReason(errors.New("short — fine")); got != "short — fine" {
		t.Errorf("a short reason should be kept whole, got %q", got)
	}
}

func TestServeExampleConfig(t *testing.T) {
	// The shipped example, with no pacing, streams exactly its batch twin's run.
	yaml := readFile(t, "../../cfg/example_serve_config.yaml")
	batch := yaml[:strings.Index(yaml, "run:\n")]
	want := capturedRun(t, batch, nil)
	if len(want) != 101 {
		t.Fatalf("reference has %d rows, want 101", len(want))
	}
	server := servedHandler(t, replaceOnce(t, yaml, "pace_ms: 200", "pace_ms: 0"))
	assertMatchesReference(t, "example", readStream(t, wsURLOf(server)), want)
}

func TestServeModeEndToEnd(t *testing.T) {
	want := capturedRun(t, serveConfigYAML, nil)

	t.Run("the CLI serves a run: {mode: serve} config", func(t *testing.T) {
		address := freeAddress(t)
		path := writeConfigPath(t, serveConfigYAML+serveRun(address))
		executeServing(t, address, "--config", path)
		assertMatchesReference(t, "served", readStream(t, "ws://"+address+"/handle"), want)
	})

	t.Run("a --socket file still serves, as an alias for serve mode", func(t *testing.T) {
		address := freeAddress(t)
		socket := filepath.Join(t.TempDir(), "socket.yaml")
		writeFile(t, socket, fmt.Sprintf("address: %q\nhandle: \"/handle\"\nmillisecond_delay: 0\n", address))
		executeServing(t, address, "--config", writeConfigPath(t, serveConfigYAML), "--socket", socket)
		assertMatchesReference(t, "served", readStream(t, "ws://"+address+"/handle"), want)
	})

	t.Run("--socket alongside a serve config is a usage error", func(t *testing.T) {
		socket := filepath.Join(t.TempDir(), "socket.yaml")
		writeFile(t, socket, "address: \":2112\"\nhandle: /handle\n")
		err := Execute([]string{"stochadex",
			"--config", writeConfigPath(t, serveConfigYAML+serveRun(":2112")), "--socket", socket})
		if KindOf(err) != ErrUsage || ExitCode(err) != 64 {
			t.Errorf("expected ErrUsage (64), got %v", err)
		}
	})

	t.Run("a serve config with no file to re-load is a config error", func(t *testing.T) {
		config := writeConfig(t, serveConfigYAML+serveRun("127.0.0.1:0"))
		config.sourcePath = ""
		if err := runE(config, &SocketConfig{}); KindOf(err) != ErrConfig {
			t.Errorf("expected ErrConfig, got %v", err)
		}
	})

	t.Run("an address already in use is unavailable", func(t *testing.T) {
		occupied, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer occupied.Close()
		path := writeConfigPath(t, serveConfigYAML+serveRun(occupied.Addr().String()))
		err = Execute([]string{"stochadex", "--config", path})
		if KindOf(err) != ErrUnavailable || ExitCode(err) != 75 {
			t.Errorf("expected ErrUnavailable (75), got %v", err)
		}
	})
}

func TestServeModeValidation(t *testing.T) {
	dir := t.TempDir()
	shorthand := "    output_condition: {type: every_step}\n    output_function: {type: nil}\n"
	noShorthand := strings.Replace(serveConfigYAML, shorthand, "", 1)
	view := func(path string) string {
		return fmt.Sprintf("outputs:\n- {name: log, function: {type: json_log, path: %q}}\n",
			filepath.Join(dir, path))
	}
	cases := []struct{ name, yaml, want string }{
		{"serve with no websocket", serveConfigYAML + "run: {mode: serve}\n",
			"needs websocket: {address"},
		{"an address with no port", serveConfigYAML + serveRun("localhost"),
			`run.websocket.address "localhost"`},
		{"websocket on a batch run", serveConfigYAML + "run: {websocket: {address: \":2112\"}}\n",
			"only apply to run: {mode: serve}"},
		{"pace_ms on an ensemble", serveConfigYAML + "run: {mode: ensemble, seeds: [1], pace_ms: 5}\n",
			"only apply to run: {mode: serve}"},
		{"seeds on a serve run", serveConfigYAML +
			"run: {mode: serve, seeds: [1], websocket: {address: \":2112\"}}\n",
			"only apply to run: {mode: ensemble}"},
		{"a view every connection would share", noShorthand + serveRun(":2112") + view("run.log"),
			"put {connection} in it"},
		{"{connection} outside serve mode", noShorthand + view("run-{connection}.log"),
			"uses {connection}"},
		{"{member} in serve mode", noShorthand + serveRun(":2112") + view("run-{member}-{connection}.log"),
			"uses {member}"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := LoadConfig(writeConfigPath(t, c.yaml))
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}
	// A macros config's run: block is checked with its macro context, when it runs.
	for name, run := range map[string]string{
		"serve mode":             serveRun(":2112"),
		"pace_ms with no mode":   "run: {pace_ms: 5}\n",
		"websocket with no mode": "run: {websocket: {address: \":2112\"}}\n",
	} {
		t.Run(name+" on a macros config fails the run", func(t *testing.T) {
			_, err := RunToStorage(writeConfig(t, macroConfigYAML+run))
			if KindOf(err) != ErrConfig ||
				!strings.Contains(err.Error(), "do not yet support run modes (ensemble, serve") {
				t.Errorf("expected ErrConfig rejecting the run: block, got %v", err)
			}
		})
	}
	t.Run("a nil view needs no placeholder, and handle defaults to /", func(t *testing.T) {
		config, err := LoadConfig(writeConfigPath(t, noShorthand+
			"run: {mode: serve, websocket: {address: \":2112\"}}\n"+
			"outputs:\n- {name: quiet, function: {type: nil}}\n"))
		if err != nil {
			t.Fatal(err)
		}
		if got := config.Run.Websocket.handle(); got != "/" {
			t.Errorf("default handle = %q, want /", got)
		}
	})
}

func TestSocketAlias(t *testing.T) {
	socket := &SocketConfig{Address: "127.0.0.1:2112", Handle: "/h", MillisecondDelay: 7,
		AllowedOrigins: []string{"https://dash.example.org"}}

	t.Run("it switches a batch config to serving, leaving the caller's config alone", func(t *testing.T) {
		config := writeConfig(t, serveConfigYAML)
		served, err := withSocketAlias(config, socket, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		w := served.Run.Websocket
		if served.Run.Mode != "serve" || served.Run.PaceMs != 7 || w == nil ||
			w.Address != socket.Address || w.Handle != "/h" ||
			len(w.AllowedOrigins) != 1 || w.AllowedOrigins[0] != "https://dash.example.org" {
			t.Errorf("aliased run: block = %+v (websocket %+v)", served.Run, w)
		}
		if config.Run.Mode != "" || config.Run.Websocket != nil {
			t.Errorf("the caller's config was modified: %+v", config.Run)
		}
	})

	t.Run("it prints a deprecation notice", func(t *testing.T) {
		config := writeConfig(t, serveConfigYAML)
		var notices bytes.Buffer
		if _, err := withSocketAlias(config, socket, &notices); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(notices.String(), "--socket is deprecated") ||
			!strings.Contains(notices.String(), `address: "127.0.0.1:2112"`) {
			t.Errorf("notice = %q", notices.String())
		}
	})

	t.Run("an inactive socket leaves the config as it is", func(t *testing.T) {
		config := writeConfig(t, serveConfigYAML)
		got, err := withSocketAlias(config, &SocketConfig{Handle: "/h"}, io.Discard)
		if err != nil || got != config {
			t.Errorf("expected the same config back, got %v, %v", got, err)
		}
	})

	cases := []struct {
		name, yaml string
		kind       ErrorKind
		want       string
	}{
		{"a config that already serves", serveConfigYAML + serveRun(":2112"), ErrUsage,
			"both configure serving"},
		{"an ensemble", readFile(t, "../../cfg/example_ensemble_config.yaml"), ErrUsage,
			"only applies to a batch run, not run: {mode: ensemble}"},
		{"a macros config", macroConfigYAML, ErrUsage, "does not apply to a macros: config"},
		{"outputs: views with no {connection}", strings.Replace(serveConfigYAML,
			"    output_condition: {type: every_step}\n    output_function: {type: nil}\n", "", 1) +
			fmt.Sprintf("outputs:\n- {name: log, function: {type: json_log, path: %q}}\n",
				filepath.Join(t.TempDir(), "run.log")), ErrConfig, "put {connection} in it"},
	}
	for _, c := range cases {
		t.Run("it is rejected alongside "+c.name, func(t *testing.T) {
			_, err := withSocketAlias(writeConfig(t, c.yaml), socket, io.Discard)
			if KindOf(err) != c.kind || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected kind %v containing %q, got %v", c.kind, c.want, err)
			}
		})
	}
}
