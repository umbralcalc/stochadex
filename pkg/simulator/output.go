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

// OutputFunction is the interface that must be implemented for any function
// which can be used to outputs data from the simulation when the provided
// OutputCondition is met.
type OutputFunction interface {
	Output(partitionName string, state []float64, cumulativeTimesteps float64)
}

// NilOutputFunction outputs nothing from the simulation.
type NilOutputFunction struct{}

func (f *NilOutputFunction) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
}

// StdoutOutputFunction outputs the state to the terminal.
type StdoutOutputFunction struct{}

func (s *StdoutOutputFunction) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
	fmt.Println(cumulativeTimesteps, partitionName, state)
}

// StateTimeStorageOutputFunction stores the data from the simulation
// in the provided StateTimeStorage on the steps when the OutputCondition
// is met.
type StateTimeStorageOutputFunction struct {
	Store *StateTimeStorage
}

func (f *StateTimeStorageOutputFunction) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
	f.Store.ConcurrentAppend(partitionName, cumulativeTimesteps, state)
}

// JsonLogEntry is the format in which the logs are serialised when using the
// JsonLogOutputFunction.
type JsonLogEntry struct {
	PartitionName       string    `json:"partition_name"`
	State               []float64 `json:"state"`
	CumulativeTimesteps float64   `json:"time"`
}

// JsonLogOutputFunction outputs data to a log of json packets from
// the simulation on the steps where the OutputCondition is met.
type JsonLogOutputFunction struct {
	file  *os.File
	mutex *sync.Mutex
}

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

// JsonLogChannelOutputFunction outputs data to a log of json packets from
// the simulation on the steps where the OutputCondition is met. This is
// functionally the same as the JsonLogOutputFunction but runs in its own
// thread and receives logs via channel for improved performance.
type JsonLogChannelOutputFunction struct {
	logChannel chan JsonLogEntry
}

func (j *JsonLogChannelOutputFunction) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
	logEntry := JsonLogEntry{
		PartitionName:       partitionName,
		State:               state,
		CumulativeTimesteps: cumulativeTimesteps,
	}
	j.logChannel <- logEntry
}

// Close ensures that the log channel flushes at the end of a run.
func (j *JsonLogChannelOutputFunction) Close() {
	close(j.logChannel)
}

// NewJsonLogChannelOutputFunction creates a new JsonLogChannelOutputFunction.
// This creates a new channel which can be deferred to close so that flushing
// at the end of a run is ensured like this:
// f = NewJsonLogChannelOutputFunction("file.log"); defer f.Close()
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
		for {
			select {
			case logEntry := <-logChannel:
				jsonData, err := json.Marshal(logEntry)
				if err != nil {
					log.Printf("Error encoding JSON: %s\n", err)
					panic(err)
				}
				jsonData = append(jsonData, '\n')

				_, err = file.Write(jsonData)
				if err != nil {
					panic(err)
				}
			default:
				continue
			}
		}
	}()
	return &JsonLogChannelOutputFunction{logChannel: logChannel}
}

// WebsocketOutputFunction serialises the state of each partition of the
// simulation and sends this data via a websocket connection on the steps
// when the OutputCondition is met.
type WebsocketOutputFunction struct {
	connection *websocket.Conn
	mutex      *sync.Mutex
}

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

// NewWebsocketOutputFunction creates a new WebsocketOutputFunction given a
// connection struct and mutex to protect concurrent writes to the connection.
func NewWebsocketOutputFunction(
	connection *websocket.Conn,
	mutex *sync.Mutex,
) *WebsocketOutputFunction {
	return &WebsocketOutputFunction{connection: connection, mutex: mutex}
}

// OutputCondition is the interface that must be implemented to define when the
// simulation calls the OutputFunction.
type OutputCondition interface {
	IsOutputStep(partitionName string, state []float64, cumulativeTimesteps float64) bool
}

// NilOutputCondition never outputs.
type NilOutputCondition struct{}

func (c *NilOutputCondition) IsOutputStep(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) bool {
	return false
}

// EveryStepOutputCondition calls the OutputFunction at every step.
type EveryStepOutputCondition struct{}

func (c *EveryStepOutputCondition) IsOutputStep(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) bool {
	return true
}

// EveryNStepsOutputCondition calls the OutputFunction once for every N
// steps that occur.
type EveryNStepsOutputCondition struct {
	N      int
	ticker int
}

func (c *EveryNStepsOutputCondition) IsOutputStep(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) bool {
	c.ticker += 1
	if c.ticker == c.N {
		c.ticker = 0
		return true
	}
	return false
}

// OnlyGivenPartitionsOutputCondition calls the OutputFunction for only
// the given partition names.
type OnlyGivenPartitionsOutputCondition struct {
	Partitions map[string]bool
}

func (o *OnlyGivenPartitionsOutputCondition) IsOutputStep(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) bool {
	if v, ok := o.Partitions[partitionName]; ok && v {
		return true
	}
	return false
}
