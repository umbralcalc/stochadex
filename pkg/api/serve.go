package api

import (
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// WebsocketServeConfig is where run: {mode: serve} listens: the websocket is
// mounted at Handle ("/" when empty) on Address.
type WebsocketServeConfig struct {
	Address string `yaml:"address"`
	Handle  string `yaml:"handle,omitempty"`
	// AllowedOrigins lists extra browser origins (e.g. "https://dash.example.com")
	// that may connect, beyond the server's own host and loopback hosts; "*"
	// admits every origin.
	AllowedOrigins []string `yaml:"allowed_origins,omitempty"`
}

// handle returns the path the websocket is mounted at.
func (w *WebsocketServeConfig) handle() string {
	if w.Handle == "" {
		return "/"
	}
	return w.Handle
}

// validateRunMode rejects run: fields that the config's mode would ignore, and
// a serve mode with nowhere to listen.
func validateRunMode(config *ApiRunConfig) error {
	run := &config.Run
	if run.Mode != "serve" {
		if run.Websocket != nil {
			return fmt.Errorf("api: run.websocket only applies to run: {mode: serve}")
		}
		if run.PaceMs != 0 && run.Mode == "ensemble" {
			return fmt.Errorf("api: run.pace_ms paces a batch or served run, not " +
				"run: {mode: ensemble}")
		}
		return nil
	}
	if len(run.Seeds) > 0 || run.Concurrency != 0 {
		return fmt.Errorf("api: run.seeds and run.concurrency only apply to " +
			"run: {mode: ensemble}; a served run is one run per connection")
	}
	if run.Websocket == nil || run.Websocket.Address == "" {
		return fmt.Errorf("api: run: {mode: serve} needs websocket: {address: ...}, " +
			"e.g. websocket: {address: \":2112\", handle: /handle}")
	}
	if _, _, err := net.SplitHostPort(run.Websocket.Address); err != nil {
		return fmt.Errorf("api: run.websocket.address %q: %w", run.Websocket.Address, err)
	}
	// What a client receives is an output like any other, so it is declared in
	// outputs:, with its own condition.
	streams := 0
	for _, view := range config.Outputs {
		if view.Function.Type == connectionSink {
			streams++
		}
	}
	if streams != 1 {
		return fmt.Errorf("api: run: {mode: serve} needs exactly one outputs: view "+
			"with function {type: connection} — what each client receives — got %d; "+
			"e.g. outputs: [{name: stream, function: {type: connection}}]", streams)
	}
	return nil
}

// serveHandler returns the websocket handler behind run: {mode: serve}. Each
// connection rebuilds the config, from the document loaded at startup with its
// overrides applied, for a fresh, unshared run of the model. The
// client receives the config's {type: connection} view, gated by that view's
// condition, and the connection writes the other outputs: views for itself with
// {connection} substituted. Each connection reads its inputs afresh, so a connection whose inputs have gone is closed with the
// reason rather than served.
func serveHandler(config *ApiRunConfig) (http.Handler, error) {
	if config.sourcePath == "" {
		return nil, fmt.Errorf("api: serving a websocket requires a config " +
			"loaded from a file (each connection is rebuilt by re-loading it)")
	}
	if len(config.Macros) > 0 {
		// Serve its expansion, which each connection re-loads.
		expanded, err := expandToRun(config)
		if err != nil {
			return nil, err
		}
		config = expanded
	}
	var connections atomic.Int64
	build := func(stream simulator.OutputFunction) (servedRun, error) {
		connection := int(connections.Add(1) - 1)
		connectionConfig, err := config.reload()
		if err != nil {
			return servedRun{}, err
		}
		prepared, err := preparedMainGenerator(connectionConfig)
		if err != nil {
			return servedRun{}, err
		}
		implementations := *prepared.implementations
		views := connectionConfig.connectionOutputViews(connection, stream)
		implementations.OutputFunction = views
		implementations.OutputCondition = &simulator.EveryStepOutputCondition{}
		if prepared.feeds != nil {
			prepared.feeds.forConnection(connection)
		}
		// Staged before the coordinator configures them, so nothing is written
		// at a final path.
		outputs := &stagedOutputs{feeds: prepared.feeds}
		outputs.stageNested(connectionConfig)
		outputs.stage(views)
		return servedRun{
			coordinator: simulator.NewPartitionCoordinator(prepared.settings, &implementations),
			feeds:       prepared.feeds,
			outputs:     outputs,
		}, nil
	}
	return newStreamHandler(build, time.Duration(config.Run.PaceMs)*time.Millisecond,
		config.Run.Websocket.AllowedOrigins), nil
}

// runServe listens on the config's websocket address and serves each
// connection its own run until the server fails. Failing to listen (e.g. the
// address is in use) is unavailable, so an orchestrator may retry.
func runServe(config *ApiRunConfig) error {
	handler, err := serveHandler(config)
	if err != nil {
		return configError(err)
	}
	listener, err := net.Listen("tcp", config.Run.Websocket.Address)
	if err != nil {
		return &Error{Kind: ErrUnavailable, Err: fmt.Errorf(
			"api: serving on %s: %w", config.Run.Websocket.Address, err)}
	}
	mux := http.NewServeMux()
	mux.Handle(config.Run.Websocket.handle(), handler)
	return &Error{Kind: ErrRuntime, Err: http.Serve(listener, mux)}
}
