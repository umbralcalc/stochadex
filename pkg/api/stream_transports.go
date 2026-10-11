package api

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Stream transports registered from outside this package (PLAN.md 1.7). A
// stream input names a transport, e.g. stream: {keyboard: {...}}, and the run
// reads that transport's messages exactly as it reads a websocket's: decoded
// as json_log entries or ActionState protobufs (decode:), held between
// messages (hold_last), and recorded with record: for replay.
//
// A transport lives where its dependencies do, and registers itself, as data
// sources do with RegisterDataSource: pkg/keyboard registers keyboard, which
// the CLI imports, so this package (which dashboards compile to WebAssembly)
// does not depend on a terminal library.

// StreamSource is one open stream from a registered transport.
type StreamSource interface {
	// Read delivers each message the stream produces until the stream ends,
	// Close is called, or deliver returns an error.
	Read(deliver func(message []byte) error)
	// Close stops the stream and releases what it holds.
	Close() error
}

// StreamTransport is a registered stream transport: Check validates a
// config's fields for it when the config loads, and Open opens it when a run
// starts.
type StreamTransport struct {
	Check func(fields map[string]interface{}) error
	Open  func(fields map[string]interface{}) (StreamSource, error)
}

var (
	streamTransportsMu sync.RWMutex
	streamTransports   = map[string]StreamTransport{}
)

// RegisterStreamTransport makes a stream transport available to configs under
// name, as stream: {name: {...fields}}.
func RegisterStreamTransport(name string, transport StreamTransport) {
	streamTransportsMu.Lock()
	defer streamTransportsMu.Unlock()
	if _, exists := streamTransports[name]; exists || name == "websocket" || name == "connection" {
		panic("api: stream transport " + name + " is already registered")
	}
	streamTransports[name] = transport
}

func streamTransport(name string) (StreamTransport, bool) {
	streamTransportsMu.RLock()
	defer streamTransportsMu.RUnlock()
	transport, ok := streamTransports[name]
	return transport, ok
}

func registeredStreamTransports() string {
	streamTransportsMu.RLock()
	defer streamTransportsMu.RUnlock()
	names := []string{"websocket", "connection"}
	for name := range streamTransports {
		names = append(names, name)
	}
	sort.Strings(names[2:])
	return strings.Join(names, ", ")
}

// registeredTransport is the one registered transport a stream input names,
// if it names one.
func (s *StreamConfig) registeredTransport() (string, map[string]interface{}, bool) {
	for name, fields := range s.Extra {
		return name, fields, true
	}
	return "", nil, false
}

// checkRegisteredTransport validates a stream input's registered transport.
func checkRegisteredTransport(input string, name string, fields map[string]interface{}) error {
	transport, ok := streamTransport(name)
	if !ok {
		return fmt.Errorf("api: input %q: unknown stream transport %q (this binary has: %s)",
			input, name, registeredStreamTransports())
	}
	if transport.Check != nil {
		if err := transport.Check(fields); err != nil {
			return fmt.Errorf("api: input %q: stream %s: %w", input, name, err)
		}
	}
	return nil
}
