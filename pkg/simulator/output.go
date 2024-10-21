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
// which can be used to outputs data from the stochastic process when the provided
// OutputCondition is met.
type OutputFunction interface {
	Output(partitionName string, state []float64, cumulativeTimesteps float64)
}

// NilOutputFunction outputs nothing from the stochastic process.
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

// VariableStoreOutputFunction stores the data from the stochastic process in a provided
// Store variable on the steps when the OutputCondition is met
type VariableStoreOutputFunction struct {
	Store map[string][][]float64
}

func (f *VariableStoreOutputFunction) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
	if store, ok := f.Store[partitionName]; ok {
		f.Store[partitionName] = append(store, state)
	} else {
		f.Store[partitionName] = [][]float64{state}
	}
}

// JsonLogEntry is the format in which the logs are serialised when using the
// JsonLogOutputFunction.
type JsonLogEntry struct {
	PartitionName       string    `json:"partition_name"`
	State               []float64 `json:"state"`
	CumulativeTimesteps float64   `json:"time"`
}

// JsonLogOutputFunction outputs data to a log of json packets from
// the simulation.
type JsonLogOutputFunction struct {
	file *os.File
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
	jsonData = append(jsonData, []byte("\n")...)
	_, err = j.file.Write(jsonData)
	if err != nil {
		panic(err)
	}
}

// NewJsonLogOutputFunction creates a new JsonLogOutputFunction.
func NewJsonLogOutputFunction(
	filePath string,
) *JsonLogOutputFunction {
	file, err := os.Create(filePath)
	if err != nil {
		log.Fatal("Error creating log file:", err)
		panic(err)
	}
	return &JsonLogOutputFunction{file: file}
}

// WebsocketOutputFunction serialises the state of each partition of the simulation
// and sends this data via a websocket connection.
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
		&DashboardPartitionState{
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
	w.mutex.Unlock()
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
// stochastic process calls the OutputFunction.
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

// EveryStepOutputCondition calls the OutputFunction once for every N
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
