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

// VariableStore dynamically adapts its structure to support incoming time series
// data from the simulation output. This is done in a way to minimise write blocking
// for better concurrent performance.
type VariableStore struct {
	indexByName map[string]int
	store       [][][]float64
	mutex       *sync.Mutex
}

// GetNames retrieves all the names in the store to key each time series.
func (v *VariableStore) GetNames() []string {
	names := make([]string, 0)
	for name := range v.indexByName {
		names = append(names, name)
	}
	return names
}

// GetValues retrieves all the time series values keyed by the name.
func (v *VariableStore) GetValues(name string) [][]float64 {
	return v.store[v.indexByName[name]]
}

// Append adds another set of values to the time series data keyed
// by the provided name. This method also handles dynamic extension
// of the size of the store in response to the inputs.
func (v *VariableStore) Append(name string, values []float64) {
	if index, ok := v.indexByName[name]; ok {
		v.store[index] = append(v.store[index], values)
	} else {
		v.mutex.Lock()
		v.indexByName[name] = len(v.indexByName)
		v.mutex.Unlock()
		v.store = append(v.store, [][]float64{values})
	}
}

// NewVariableStore creates a new VariableStore.
func NewVariableStore() *VariableStore {
	var mutex sync.Mutex
	return &VariableStore{
		indexByName: make(map[string]int),
		store:       make([][][]float64, 0),
		mutex:       &mutex,
	}
}

// VariableStoreOutputFunction stores the data from the stochastic
// process in the provided VariableStore variable on the steps when
// the OutputCondition is met.
type VariableStoreOutputFunction struct {
	Store *VariableStore
}

func (f *VariableStoreOutputFunction) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
	f.Store.Append(partitionName, state)
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
