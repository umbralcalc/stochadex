package api

import (
	"fmt"
	"io"
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
// a serve mode with nowhere to listen. A macros: config's run: block is
// validated with the rest of its macro context instead.
func validateRunMode(config *ApiRunConfig) error {
	if len(config.Macros) > 0 {
		return nil
	}
	run := &config.Run
	if run.Mode != "serve" {
		if run.Websocket != nil || run.PaceMs != 0 {
			return fmt.Errorf("api: run.websocket and run.pace_ms only apply to " +
				"run: {mode: serve}")
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
	return nil
}

// withSocketAlias returns the config a deprecated socket file describes: a
// batch config switched to serve mode with the socket's address, handle,
// origins and delay. An inactive socket (no address) leaves the config as it
// is, and the caller's config is never modified. A socket file is rejected
// alongside a config that already serves, runs an ensemble, or has macros —
// before serve mode existed in the config it was silently ignored there. The
// deprecation notice goes to notices.
func withSocketAlias(
	config *ApiRunConfig,
	socket *SocketConfig,
	notices io.Writer,
) (*ApiRunConfig, error) {
	if socket == nil || !socket.Active() {
		return config, nil
	}
	usage := func(format string, args ...interface{}) error {
		return &Error{Kind: ErrUsage, Err: fmt.Errorf(format, args...)}
	}
	switch {
	case config.Run.Mode == "serve":
		return nil, usage("api: --socket and run: {mode: serve} both configure " +
			"serving; drop --socket")
	case len(config.Macros) > 0:
		return nil, usage("api: --socket does not apply to a macros: config")
	case config.Run.Mode != "" && config.Run.Mode != "batch":
		return nil, usage("api: --socket only applies to a batch run, not "+
			"run: {mode: %s}", config.Run.Mode)
	}
	for _, view := range config.Outputs {
		if err := checkInstancePlaceholders(view, "serve"); err != nil {
			return nil, configError(err)
		}
	}
	fmt.Fprintf(notices, "stochadex: --socket is deprecated; put run: {mode: serve, "+
		"websocket: {address: %q, handle: %q}, pace_ms: %d} in the config instead\n",
		socket.Address, socket.Handle, socket.MillisecondDelay)
	served := *config
	served.Run = RunModeConfig{
		Mode: "serve",
		Websocket: &WebsocketServeConfig{
			Address:        socket.Address,
			Handle:         socket.Handle,
			AllowedOrigins: socket.AllowedOrigins,
		},
		PaceMs: socket.MillisecondDelay,
	}
	return &served, nil
}

// serveHandler returns the websocket handler behind run: {mode: serve}. Each
// connection re-loads the config for a fresh, unshared run of the model and
// streams it to the client. The stream is the config's output with the
// websocket in place of its sink: gated by output_condition when the config
// uses the shorthand pair (whose output_function is not written), or every
// step alongside the config's outputs: views, which each connection writes for
// itself with {connection} substituted. Each connection reads the config file
// and its inputs afresh, so a connection whose inputs have gone is closed with
// the reason rather than served.
func serveHandler(config *ApiRunConfig) (http.Handler, error) {
	if config.sourcePath == "" {
		return nil, fmt.Errorf("api: serving a websocket requires a config " +
			"loaded from a file (each connection is rebuilt by re-loading it)")
	}
	path := config.sourcePath
	var connections atomic.Int64
	build := func(stream simulator.OutputFunction) (*simulator.ConfigGenerator, error) {
		connection := int(connections.Add(1) - 1)
		connectionConfig, err := LoadConfig(path)
		if err != nil {
			return nil, err
		}
		if err := bindInputs(connectionConfig); err != nil {
			return nil, err
		}
		generator := connectionConfig.GetConfigGenerator()
		simulation := generator.GetSimulation()
		if len(connectionConfig.Outputs) > 0 {
			views := connectionConfig.connectionOutputViews(connection)
			views.Views = append([]simulator.OutputView{{
				Name:      "websocket",
				Condition: &simulator.EveryStepOutputCondition{},
				Function:  stream,
			}}, views.Views...)
			simulation.OutputFunction = views
			simulation.OutputCondition = &simulator.EveryStepOutputCondition{}
		} else {
			simulation.OutputFunction = stream
		}
		generator.SetSimulation(simulation)
		return generator, nil
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
