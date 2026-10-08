package api

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"google.golang.org/protobuf/proto"
)

// twoWayYAML serves a run whose dial echoes its level param, set from the
// served client's messages, and whose walk's variances follow the dial within
// each step. extra is appended to the actions input.
func twoWayYAML(steps int, record, extra string) string {
	recordLine := ""
	if record != "" {
		recordLine = fmt.Sprintf("    record: %q\n", record)
	}
	return fmt.Sprintf(`inputs:
  actions:
    stream: {connection: {}}
%s%smain:
  partitions:
  - {name: dial, iteration: {type: sleepy_param_echo}, params: {level: [0.0]}, params_from_input: {level: {input: actions}}, init_state_values: [0.0], state_history_depth: 1, seed: 1}
  - {name: walk, iteration: {type: wiener_process}, params: {variances: [1.0]}, params_from_upstream: {variances: {upstream: dial}}, init_state_values: [0.0], state_history_depth: 1, seed: 2}
  simulation:
    termination_condition: {type: number_of_steps, max_steps: %d}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
outputs:
- {name: stream, function: {type: connection}}
run: {mode: serve, websocket: {address: "127.0.0.1:0"}}
`, recordLine, extra, steps)
}

// played is what a client saw: the frames streamed to it, by partition in
// order of time.
type played map[string][][]float64

// playClient connects to a served run, sends each message once it has read
// the given number of frames, and reads frames until the run ends.
func playClient(t *testing.T, url string, sends map[int][]byte, messageType int) played {
	t.Helper()
	connection, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer connection.Close()
	type frame struct {
		time  float64
		state []float64
	}
	byPartition := map[string][]frame{}
	for read := 0; ; read++ {
		if message, ok := sends[read]; ok {
			if err := connection.WriteMessage(messageType, message); err != nil {
				t.Fatalf("send: %v", err)
			}
		}
		_, data, err := connection.ReadMessage()
		if err != nil {
			break
		}
		state := &simulator.PartitionState{}
		if err := proto.Unmarshal(data, state); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		byPartition[state.PartitionName] = append(byPartition[state.PartitionName],
			frame{state.CumulativeTimesteps, state.State})
	}
	result := played{}
	for name, frames := range byPartition {
		sort.Slice(frames, func(i, j int) bool { return frames[i].time < frames[j].time })
		for _, f := range frames {
			result[name] = append(result[name], f.state)
		}
	}
	return result
}

func TestTwoWayServe(t *testing.T) {
	t.Run("a client's messages drive its run, and the run replays from its record", func(t *testing.T) {
		dir := t.TempDir()
		record := filepath.Join(dir, "actions-{connection}.log")
		server := servedHandler(t, twoWayYAML(300, record, ""))
		// Send level 5 after 40 frames (20 steps of 2 partitions), then 9 after 300.
		seen := playClient(t, wsURLOf(server), map[int][]byte{
			40:  []byte(`{"partition_name": "level", "state": [5]}`),
			300: []byte(`[{"partition_name": "level", "state": [9]}, {"partition_name": "other", "state": [1]}]`),
		}, websocket.TextMessage)
		dial := seen["dial"]
		if len(dial) != 301 {
			t.Fatalf("streamed %d dial rows, want 301", len(dial))
		}
		// The level is 0 until the first message, then 5, then 9: each value
		// arrives between steps and holds until the next.
		changes := []float64{}
		for k := 1; k < len(dial); k++ {
			if dial[k][0] != dial[k-1][0] {
				changes = append(changes, dial[k][0])
			}
		}
		if dial[1][0] != 0 || fmt.Sprint(changes) != "[5 9]" {
			t.Fatalf("the level started at %v and changed to %v; want 0, then [5 9]", dial[1], changes)
		}
		// Replay: the same model run in batch, its actions from connection 0's record.
		replayed := strings.NewReplacer(
			"    stream: {connection: {}}\n", fmt.Sprintf("    source: {json_log: {path: %q}}\n",
				filepath.Join(dir, "actions-0.log")),
			fmt.Sprintf("    record: %q\n", record), "",
			"run: {mode: serve, websocket: {address: \"127.0.0.1:0\"}}\n", "",
			"outputs:\n- {name: stream, function: {type: connection}}\n", "",
		).Replace(twoWayYAML(300, record, ""))
		result, err := RunToStorage(writeConfig(t, replayed))
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"dial", "walk"} {
			if fmt.Sprint(result.Storage.GetValues(name)) != fmt.Sprint(seen[name]) {
				t.Errorf("%s: the replay differs from what the client was streamed", name)
			}
		}
	})

	t.Run("dexetera's ActionState messages, named and broadcast", func(t *testing.T) {
		yaml := strings.Replace(twoWayYAML(300, "", "    decode: protobuf_action_state\n"),
			"  simulation:", "  - {name: dial2, iteration: {type: sleepy_param_echo}, params: {level: [0.0]}, "+
				"params_from_input: {level: {input: actions, partition: values}}, init_state_values: [0.0], "+
				"state_history_depth: 1, seed: 3}\n  simulation:", 1)
		named, _ := proto.Marshal(&simulator.ActionState{
			Partitions: map[string]*simulator.ActionValues{"level": {Values: []float64{7}}},
			Values:     []float64{100}, // ignored: named entries take precedence
		})
		broadcast, _ := proto.Marshal(&simulator.ActionState{Values: []float64{3}})
		server := servedHandler(t, yaml)
		seen := playClient(t, wsURLOf(server), map[int][]byte{30: named, 300: broadcast},
			websocket.BinaryMessage)
		last := len(seen["dial"]) - 1
		if seen["dial"][last][0] != 7 || seen["dial2"][last][0] != 3 {
			t.Errorf("finished with dial %v and dial2 %v, want [7] (named) and [3] (broadcast)",
				seen["dial"][last], seen["dial2"][last])
		}
		for k, row := range seen["dial2"] {
			if row[0] == 100 {
				t.Fatalf("dial2 took the named message's values at step %d", k)
			}
		}
	})

	t.Run("concurrent clients each drive and record their own run", func(t *testing.T) {
		dir := t.TempDir()
		server := servedHandler(t, twoWayYAML(200, filepath.Join(dir, "actions-{connection}.log"), ""))
		var wg sync.WaitGroup
		finals := make([]float64, 2)
		for client := range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				level := fmt.Sprintf(`{"partition_name": "level", "state": [%d]}`, 10+client)
				seen := playClient(t, wsURLOf(server), map[int][]byte{20: []byte(level)}, websocket.TextMessage)
				finals[client] = seen["dial"][len(seen["dial"])-1][0]
			}()
		}
		wg.Wait()
		if !(finals[0] == 10 && finals[1] == 11) {
			t.Errorf("clients finished at levels %v, want each its own (10, 11)", finals)
		}
		// Each connection wrote its own record, which ends at its client's level.
		levels := map[float64]bool{}
		for connection := range 2 {
			entries := readLogEntries(t, filepath.Join(dir, fmt.Sprintf("actions-%d.log", connection)))
			if len(entries) != 201 {
				t.Fatalf("connection %d recorded %d rows, want 201", connection, len(entries))
			}
			levels[entries[len(entries)-1].State[0]] = true
		}
		if !levels[10] || !levels[11] {
			t.Errorf("the records end at %v, want one at 10 and one at 11", levels)
		}
	})

	t.Run("a message that cannot be decoded closes the connection with the reason", func(t *testing.T) {
		server := servedHandler(t, twoWayYAML(2000, "", ""))
		connection, _, err := websocket.DefaultDialer.Dial(wsURLOf(server), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer connection.Close()
		if err := connection.WriteMessage(websocket.TextMessage, []byte("level=3")); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if _, _, err = connection.ReadMessage(); err != nil {
				break
			}
		}
		closeErr, ok := err.(*websocket.CloseError)
		if !ok || closeErr.Code != websocket.CloseInternalServerErr ||
			!strings.Contains(closeErr.Text, `stream input "actions"`) {
			t.Errorf("expected a 1011 close naming the input, got %v", err)
		}
	})

	t.Run("a served run can read a websocket server too", func(t *testing.T) {
		url := feedServer(t, func(c *websocket.Conn) {
			c.WriteMessage(websocket.TextMessage, []byte(`{"partition_name": "level", "state": [4]}`))
		})
		yaml := strings.Replace(twoWayYAML(200, "", ""), "    stream: {connection: {}}\n",
			fmt.Sprintf("    stream: {websocket: {url: %q}}\n", url), 1)
		server := servedHandler(t, yaml)
		seen := playClient(t, wsURLOf(server), nil, websocket.TextMessage)
		if last := seen["dial"][len(seen["dial"])-1]; last[0] != 4 {
			t.Errorf("finished at level %v, want the feed's [4]", last)
		}
	})
}

func TestTwoWayServeValidation(t *testing.T) {
	cases := []struct{ name, yaml, want string }{
		{"a served client outside serve mode", strings.NewReplacer(
			"run: {mode: serve, websocket: {address: \"127.0.0.1:0\"}}\n", "",
			"outputs:\n- {name: stream, function: {type: connection}}\n", "",
		).Replace(twoWayYAML(5, "", "")),
			`input "actions" reads a served client (stream: {connection: {}}), which only applies to run: {mode: serve}`},
		{"two inputs reading the served client", strings.Replace(twoWayYAML(5, "", ""),
			"main:", "  more:\n    stream: {connection: {}}\nmain:", 1) + "",
			"2 inputs read the served client"},
		{"a record every connection would share", twoWayYAML(5, "actions.log", ""),
			"put {connection} in it"},
		{"{connection} in a record outside serve mode", streamConfigYAML(
			"    stream: {websocket: {url: \"ws://x\"}}\n    record: \"feed-{connection}.log\"", 5),
			"record: uses {connection}"},
		{"both transports", strings.Replace(twoWayYAML(5, "", ""), "stream: {connection: {}}",
			"stream: {connection: {}, websocket: {url: \"ws://x\"}}", 1), "stream: needs one transport"},
		{"an unknown decode", twoWayYAML(5, "", "    decode: msgpack\n"),
			`unknown decode "msgpack" (expected json or protobuf_action_state)`},
	}
	for _, c := range cases {
		t.Run(c.name+" is a config error", func(t *testing.T) {
			_, err := LoadConfig(writeConfigPath(t, c.yaml))
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}
	t.Run("--socket alongside stream inputs is a usage error", func(t *testing.T) {
		_, err := withSocketAlias(writeConfig(t, streamConfigYAML(
			"    stream: {websocket: {url: \"ws://x\"}}", 5)),
			&SocketConfig{Address: ":2112"}, io.Discard)
		if KindOf(err) != ErrUsage || !strings.Contains(err.Error(), "does not support stream inputs") {
			t.Errorf("expected ErrUsage, got %v", err)
		}
	})
}

func TestDecodeActionState(t *testing.T) {
	named, _ := proto.Marshal(&simulator.ActionState{Values: []float64{9},
		Partitions: map[string]*simulator.ActionValues{"a": {Values: []float64{1, 2}}, "b": {}}})
	entries, err := decodeActionState(named)
	if err != nil || len(entries) != 2 {
		t.Fatalf("named: got %+v, %v", entries, err)
	}
	got := map[string]string{}
	for _, entry := range entries {
		got[entry.PartitionName] = fmt.Sprint(entry.State)
	}
	if got["a"] != "[1 2]" || got["b"] != "[]" {
		t.Errorf("named entries = %v", got)
	}
	broadcast, _ := proto.Marshal(&simulator.ActionState{Values: []float64{3, 4}})
	if entries, err := decodeActionState(broadcast); err != nil || len(entries) != 1 ||
		entries[0].PartitionName != "values" || fmt.Sprint(entries[0].State) != "[3 4]" {
		t.Errorf("broadcast: got %+v, %v", entries, err)
	}
	if _, err := decodeActionState([]byte{0xff, 0xff, 0xff}); err == nil {
		t.Error("bytes that are not an ActionState should be an error")
	}
}

func TestAStreamStopsAtItsFirstBadMessage(t *testing.T) {
	stream := &streamFeed{name: "actions", served: true, current: map[string][]float64{"level": {0}}}
	if err := stream.open(); err != nil {
		t.Fatal(err)
	}
	first := stream.deliver([]byte("level=3"))
	if KindOf(first) != ErrData {
		t.Fatalf("a bad message should be a data error, got %v", first)
	}
	// Later messages are refused with the first error, and not kept.
	if err := stream.deliver([]byte(`{"partition_name": "level", "state": [5]}`)); err != first {
		t.Errorf("a message after the error returned %v, want the first error", err)
	}
	if stream.fresh["level"] {
		t.Error("a message after the error was kept")
	}
	if err := stream.inject(); err != first {
		t.Errorf("the next step should fail with the first error, got %v", err)
	}
	stream.close()
}

func TestInteractiveExampleConfig(t *testing.T) {
	// The shipped example, unpaced and recording into a temporary directory: a
	// client that sets a large drift partway through moves the walk far from
	// where diffusion alone would, and the run replays from its record.
	dir := t.TempDir()
	yaml := strings.NewReplacer(
		`record: "controls-{connection}.log"`, fmt.Sprintf("record: %q", filepath.Join(dir, "controls-{connection}.log")),
		"pace_ms: 100", "pace_ms: 0",
		`address: ":2112"`, `address: "127.0.0.1:0"`,
	).Replace(readFile(t, "../../cfg/example_interactive_config.yaml"))
	server := servedHandler(t, yaml)
	seen := playClient(t, wsURLOf(server)+"/handle", map[int][]byte{
		20: []byte(`{"partition_name": "drift", "state": [50]}`)}, websocket.TextMessage)
	walk := seen["walk"]
	if len(walk) != 1001 || walk[len(walk)-1][0] < 1000 {
		t.Fatalf("streamed %d rows ending at %v; want 1001 ending far up the drift", len(walk), walk[len(walk)-1])
	}
	replayed := strings.NewReplacer(
		"    stream: {connection: {}}\n", fmt.Sprintf("    source: {json_log: {path: %q}}\n",
			filepath.Join(dir, "controls-0.log")),
		fmt.Sprintf("    record: %q\n", filepath.Join(dir, "controls-{connection}.log")), "",
	).Replace(yaml)
	replayed = replayed[:strings.Index(replayed, "outputs:")]
	result, err := RunToStorage(writeConfig(t, replayed))
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(result.Storage.GetValues("walk")) != fmt.Sprint(walk) {
		t.Error("the replay from the record differs from the served run")
	}
}
