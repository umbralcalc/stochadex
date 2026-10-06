package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"google.golang.org/protobuf/proto"
)

// pushReceiver is a websocket server recording, per connection, every
// PartitionState it receives and how the connection ended.
type pushReceiver struct {
	mutex       sync.Mutex
	connections [][]*simulator.PartitionState
	closeCodes  []int
	finished    sync.WaitGroup
}

func newPushReceiver(t *testing.T) (*pushReceiver, *httptest.Server) {
	receiver := &pushReceiver{}
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer connection.Close()
		receiver.finished.Add(1)
		defer receiver.finished.Done()
		receiver.mutex.Lock()
		index := len(receiver.connections)
		receiver.connections = append(receiver.connections, nil)
		receiver.closeCodes = append(receiver.closeCodes, 0)
		receiver.mutex.Unlock()
		for {
			_, data, err := connection.ReadMessage()
			if err != nil {
				code := -1
				if closeErr, ok := err.(*websocket.CloseError); ok {
					code = closeErr.Code
				}
				receiver.mutex.Lock()
				receiver.closeCodes[index] = code
				receiver.mutex.Unlock()
				return
			}
			state := &simulator.PartitionState{}
			if err := proto.Unmarshal(data, state); err != nil {
				t.Errorf("unmarshal: %v", err)
				return
			}
			receiver.mutex.Lock()
			receiver.connections[index] = append(receiver.connections[index], state)
			receiver.mutex.Unlock()
		}
	}))
	return receiver, server
}

// waitForConnections waits until n connections have been opened and every one
// has ended, failing (rather than hanging) if that takes longer than a few
// seconds — e.g. because a sink was never finalized, leaving its connection open.
func (p *pushReceiver) waitForConnections(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		p.mutex.Lock()
		count := len(p.connections)
		p.mutex.Unlock()
		if count >= n {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected %d connections, got %d", n, count)
		}
		time.Sleep(10 * time.Millisecond)
	}
	ended := make(chan struct{})
	go func() {
		p.finished.Wait()
		close(ended)
	}()
	select {
	case <-ended:
	case <-time.After(time.Until(deadline) + time.Second):
		t.Fatal("connections did not end: was the sink finalized and its connection closed?")
	}
}

func pushConfigYAML(url string) string {
	return strings.Replace(fmt.Sprintf(batchConfigYAML, "{type: every_step}", "unused"),
		`output_function: {type: json_log, path: "unused"}`,
		fmt.Sprintf(`output_function: {type: websocket, url: "%s"}`, url), 1)
}

func TestWebsocketPushOutput(t *testing.T) {
	t.Run("the run's outputs reach the server exactly, one connection per run", func(t *testing.T) {
		receiver, server := newPushReceiver(t)
		defer server.Close()
		url := "ws" + strings.TrimPrefix(server.URL, "http")
		config := writeConfig(t, pushConfigYAML(url))

		// Loading and running into storage must not connect.
		reference, err := RunToStorage(config)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond)
		receiver.mutex.Lock()
		if n := len(receiver.connections); n != 0 {
			t.Fatalf("loading or RunToStorage connected %d times", n)
		}
		receiver.mutex.Unlock()

		// Two runs of the same config object: the sink reconnects for the second.
		Run(config, &SocketConfig{})
		Run(config, &SocketConfig{})
		receiver.waitForConnections(t, 2)

		times := reference.Storage.GetTimes()
		indexOf := map[float64]int{}
		for i, time := range times {
			indexOf[time] = i
		}
		for run, stream := range receiver.connections {
			if len(stream) != 2*31 {
				t.Fatalf("run %d: got %d messages, want 62 (2 partitions x 31 rows)",
					run, len(stream))
			}
			seen := map[string]bool{}
			for _, state := range stream {
				key := fmt.Sprintf("%v/%s", state.CumulativeTimesteps, state.PartitionName)
				if seen[key] {
					t.Fatalf("run %d: duplicate message %s", run, key)
				}
				seen[key] = true
				want := reference.Storage.GetValues(state.PartitionName)[indexOf[state.CumulativeTimesteps]]
				if !floats.Equal(state.State, want) {
					t.Fatalf("run %d: %s = %v, want %v", run, key, state.State, want)
				}
			}
			if receiver.closeCodes[run] != websocket.CloseNormalClosure {
				t.Errorf("run %d: connection ended with code %d, want a normal close (%d)",
					run, receiver.closeCodes[run], websocket.CloseNormalClosure)
			}
		}
	})

	t.Run("an unreachable server fails the run, not the load", func(t *testing.T) {
		config := writeConfig(t, pushConfigYAML("ws://127.0.0.1:1/never"))
		if _, err := RunToStorage(config); err != nil {
			t.Fatalf("RunToStorage needs no server, got %v", err)
		}
		message := func() (message string) {
			defer func() {
				if r := recover(); r != nil {
					message = fmt.Sprint(r)
				}
			}()
			Run(config, &SocketConfig{})
			return ""
		}()
		if !strings.Contains(message, "ws://127.0.0.1:1/never") {
			t.Errorf("running should fail naming the URL, got %q", message)
		}
	})

	t.Run("an unknown field is rejected at load", func(t *testing.T) {
		_, err := simulator.ResolveOutputFunction(simulator.ComponentSpec{
			Type:   "websocket",
			Fields: map[string]interface{}{"url": "ws://x", "adress": "typo"},
		})
		if err == nil {
			t.Error("expected an error for the unknown field")
		}
	})
}
