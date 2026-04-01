package simulator

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// OutputFunction writes state/time to an output sink when the OutputCondition
// is met.
//
// Configure is called once before parallel output begins (from
// NewPartitionCoordinator). Use it to pre-register partition names, cache
// indices, or open resources. Implementations that need no setup can leave
// it empty.
type OutputFunction interface {
	Configure(settings *Settings)
	Output(partitionName string, state []float64, cumulativeTimesteps float64)
}

// NilOutputFunction outputs nothing from the simulation.
type NilOutputFunction struct{}

func (f *NilOutputFunction) Configure(*Settings) {}

func (f *NilOutputFunction) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
}

// StdoutOutputFunction outputs the state to the terminal.
type StdoutOutputFunction struct{}

func (s *StdoutOutputFunction) Configure(*Settings) {}

func (s *StdoutOutputFunction) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
	fmt.Println(cumulativeTimesteps, partitionName, state)
}

// StateTimeStorageOutputFunction stores output into StateTimeStorage when the
// condition is met.
type StateTimeStorageOutputFunction struct {
	Store       *StateTimeStorage
	nameToIndex map[string]int // populated by Configure; read-only during Output
}

// Configure pre-registers all partition names on Store and caches their
// indices for lock-free lookup in Output. Safe to call multiple times.
func (f *StateTimeStorageOutputFunction) Configure(settings *Settings) {
	if f == nil || f.Store == nil || settings == nil {
		return
	}
	names := make([]string, 0, len(settings.Iterations))
	for _, it := range settings.Iterations {
		names = append(names, it.Name)
	}
	f.Store.PreRegisterPartitions(names)
	nameToIndex := make(map[string]int, len(names))
	for _, name := range names {
		if index, ok := f.Store.IndexOf(name); ok {
			nameToIndex[name] = index
		}
	}
	f.nameToIndex = nameToIndex
}

func (f *StateTimeStorageOutputFunction) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
	f.Store.AppendByIndex(f.nameToIndex[partitionName], cumulativeTimesteps, state)
}

// JsonLogEntry is the serialised record format used by JSON log outputs.
type JsonLogEntry struct {
	PartitionName       string    `json:"partition_name"`
	State               []float64 `json:"state"`
	CumulativeTimesteps float64   `json:"time"`
}

// JsonLogOutputFunction writes newline-delimited JSON log entries.
type JsonLogOutputFunction struct {
	file  *os.File
	mutex *sync.Mutex
}

func (j *JsonLogOutputFunction) Configure(*Settings) {}

func (j *JsonLogOutputFunction) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
	logEntry := JsonLogEntry{
		PartitionName:       partitionName,
		State:               state,
		CumulativeTimesteps: cumulativeTimesteps,
	}
	jsonData, err := json.Marshal(logEntry)
	if err != nil {
		log.Printf("Error encoding JSON: %s\n", err)
		panic(err)
	}
	jsonData = append(jsonData, '\n')

	j.mutex.Lock()
	defer j.mutex.Unlock()
	_, err = j.file.Write(jsonData)
	if err != nil {
		panic(err)
	}
}

// NewJsonLogOutputFunction creates a new JsonLogOutputFunction.
func NewJsonLogOutputFunction(
	filePath string,
) *JsonLogOutputFunction {
	var mutex sync.Mutex
	file, err := os.Create(filePath)
	if err != nil {
		log.Fatal("Error creating log file:", err)
		panic(err)
	}
	return &JsonLogOutputFunction{file: file, mutex: &mutex}
}

// JsonLogChannelOutputFunction writes JSON log entries via a background
// goroutine using a channel for improved throughput.
type JsonLogChannelOutputFunction struct {
	logChannel chan JsonLogEntry
}

func (j *JsonLogChannelOutputFunction) Configure(*Settings) {}

func (j *JsonLogChannelOutputFunction) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
	j.logChannel <- JsonLogEntry{
		PartitionName:       partitionName,
		State:               state,
		CumulativeTimesteps: cumulativeTimesteps,
	}
}

// Close flushes and stops the background writer. Defer it after construction.
func (j *JsonLogChannelOutputFunction) Close() {
	close(j.logChannel)
}

// NewJsonLogChannelOutputFunction creates a JsonLogChannelOutputFunction.
// Call Close (defer it) to ensure flushing at the end of a run.
func NewJsonLogChannelOutputFunction(
	filePath string,
) *JsonLogChannelOutputFunction {
	logChannel := make(chan JsonLogEntry)
	file, err := os.Create(filePath)
	if err != nil {
		log.Fatal("Error creating log file:", err)
		panic(err)
	}
	go func() {
		for logEntry := range logChannel {
			jsonData, err := json.Marshal(logEntry)
			if err != nil {
				log.Printf("Error encoding JSON: %s\n", err)
				panic(err)
			}
			_, err = file.Write(append(jsonData, '\n'))
			if err != nil {
				panic(err)
			}
		}
	}()
	return &JsonLogChannelOutputFunction{logChannel: logChannel}
}

// WebsocketOutputFunction serialises and sends outputs via a websocket
// connection when the condition is met.
type WebsocketOutputFunction struct {
	connection *websocket.Conn
	mutex      *sync.Mutex
}

func (w *WebsocketOutputFunction) Configure(*Settings) {}

func (w *WebsocketOutputFunction) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
	data, err := proto.Marshal(
		&PartitionState{
			CumulativeTimesteps: cumulativeTimesteps,
			PartitionName:       partitionName,
			State:               state,
		},
	)
	if err != nil {
		fmt.Println("Error marshaling protobuf message:", err)
	}

	// lock the mutex to prevent concurrent writing to the websocket connection
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if w.connection != nil {
		err := w.connection.WriteMessage(websocket.BinaryMessage, data)
		if err != nil {
			if websocket.IsUnexpectedCloseError(
				err,
				websocket.CloseGoingAway,
				websocket.CloseAbnormalClosure,
			) {
				fmt.Println("WebSocket closed unexpectedly:", err)
			} else {
				fmt.Println("Error writing to WebSocket:", err)
			}
		}
	} else {
		fmt.Println("WebSocket connection is closed or not ready.")
	}
}

// NewWebsocketOutputFunction constructs a WebsocketOutputFunction with a
// connection and a mutex for safe concurrent writes.
func NewWebsocketOutputFunction(
	connection *websocket.Conn,
	mutex *sync.Mutex,
) *WebsocketOutputFunction {
	return &WebsocketOutputFunction{connection: connection, mutex: mutex}
}

// OutputCondition decides whether an output should be emitted this step.
type OutputCondition interface {
	IsOutputStep(partitionName string, state []float64, timestepsHistory *CumulativeTimestepsHistory) bool
}

// NilOutputCondition never outputs.
type NilOutputCondition struct{}

func (c *NilOutputCondition) IsOutputStep(
	partitionName string,
	state []float64,
	timestepsHistory *CumulativeTimestepsHistory,
) bool {
	return false
}

// EveryStepOutputCondition calls the OutputFunction at every step.
type EveryStepOutputCondition struct{}

func (c *EveryStepOutputCondition) IsOutputStep(
	partitionName string,
	state []float64,
	timestepsHistory *CumulativeTimestepsHistory,
) bool {
	return true
}

// EveryNStepsOutputCondition emits output once every N steps.
type EveryNStepsOutputCondition struct {
	N int
}

func (c *EveryNStepsOutputCondition) IsOutputStep(
	partitionName string,
	state []float64,
	timestepsHistory *CumulativeTimestepsHistory,
) bool {
	return timestepsHistory.CurrentStepNumber%c.N == 0
}

// OnlyGivenPartitionsOutputCondition emits output only for listed partitions.
type OnlyGivenPartitionsOutputCondition struct {
	Partitions map[string]bool
}

func (o *OnlyGivenPartitionsOutputCondition) IsOutputStep(
	partitionName string,
	state []float64,
	timestepsHistory *CumulativeTimestepsHistory,
) bool {
	return o.Partitions[partitionName]
}
