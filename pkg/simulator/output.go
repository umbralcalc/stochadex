package simulator

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// ResourceError reports that an external resource a run depends on — an output
// file, a database, a websocket server — could not be reached when the run
// started. Sinks raise it (by panicking, as Configure has no error return) so a
// caller can tell an unavailable destination, which may be worth retrying, from
// a bug.
type ResourceError struct {
	// Resource describes what was being reached, e.g. "json_log: opening run.log".
	Resource string
	Err      error
}

func (e *ResourceError) Error() string { return e.Resource + ": " + e.Err.Error() }
func (e *ResourceError) Unwrap() error { return e.Err }

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

// FinalizingOutputFunction is the optional counterpart to OutputFunction for sinks
// that hold a resource which must be flushed, sealed or released once the run is
// over — a columnar buffer that only becomes a readable batch after the last row, a
// database handle that ingests in one shot, an open file.
//
// PartitionCoordinator.Run calls Finalize exactly once, after the final step and
// before returning, on an OutputFunction that implements this. It is an OPTIONAL
// interface deliberately: OutputFunction stays two methods, every existing sink is
// unaffected, and a sink that needs no teardown simply does not implement it.
type FinalizingOutputFunction interface {
	OutputFunction
	Finalize()
}

// StagedOutputFunction is an output function that can hold back what it writes
// until the run that wrote it is known to have ended cleanly. Once staged, it
// writes somewhere provisional; Commit publishes that to its destination, and
// Abort discards it. A run that fails, or is killed, then leaves nothing at the
// destination, and any earlier output there intact.
//
// It is OPTIONAL, like FinalizingOutputFunction, and opt-in: a sink that is
// never staged writes straight to its destination, as it always has. Stage is
// called before the sink's first run; Commit or Abort once, after its last
// Finalize. A nested run's sink is finalized once per outer step but committed
// once, when the outer run ends.
type StagedOutputFunction interface {
	OutputFunction
	Stage()
	Commit() error
	Abort()
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

// StdoutOutputFunction outputs the state to the terminal. It streams each row
// as the run goes, so it cannot be staged: a run that fails has printed the
// rows it reached.
type StdoutOutputFunction struct {
	// Prefix, when set, leads each row, naming which of several concurrent runs
	// wrote it (e.g. "member=0 seed=11"). Each row is one write, so rows from
	// concurrent runs never interleave within a line.
	Prefix string
}

func (s *StdoutOutputFunction) Configure(*Settings) {}

func (s *StdoutOutputFunction) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
	if s.Prefix == "" {
		fmt.Println(cumulativeTimesteps, partitionName, state)
		return
	}
	fmt.Println(s.Prefix, cumulativeTimesteps, partitionName, state)
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
//
// The file is touched only when a run starts, never when the sink is built:
// building happens when a config is loaded, and loading a config (to inspect it,
// check it, or run it into storage instead) must not truncate its outputs. The
// first Configure creates or truncates the file; Finalize closes it; a later
// Configure on the same sink reopens it for appending. That keeps every run of
// one sink in one log, which is what a nested run's sink needs: it is configured
// and finalized once per outer step, and its records accumulate across them.
//
// A run's entries are buffered and written in blocks, not one write per entry
// (about 30x cheaper per entry); Finalize flushes them. Run, and a Stepper's
// Close, finalize the run's output, so a log is complete once its run has
// finished. A sink driven by hand, with Output but no Configure, writes each
// entry straight to the file.
//
// Staged (see StagedOutputFunction), the log is written to path + ".partial"
// and Commit renames it to path, which replaces the file atomically: the log at
// path is always a whole run's. Abort removes the partial file.
type JsonLogOutputFunction struct {
	path    string
	file    *os.File
	writer  *bufio.Writer // nil when driven by hand: entries go straight to file
	created bool
	partial string // set by Stage: where the run is written until Commit
	mutex   *sync.Mutex
}

// partialSuffix is appended to a staged file's path while its run writes it.
const partialSuffix = ".partial"

// jsonLogBufferSize is how many bytes of entries a run's log holds before
// writing them out.
const jsonLogBufferSize = 64 * 1024

// Configure opens the log for a run: created (truncated) on the sink's first
// run, appended to on later ones, with entries buffered until Finalize. It
// panics if the file cannot be opened, as building it did.
func (j *JsonLogOutputFunction) Configure(*Settings) {
	j.mutex.Lock()
	defer j.mutex.Unlock()
	j.openLocked()
	if j.writer == nil {
		j.writer = bufio.NewWriterSize(j.file, jsonLogBufferSize)
	}
}

// openLocked opens the file if it is not already open. The caller holds mutex.
func (j *JsonLogOutputFunction) openLocked() {
	if j.file != nil {
		return
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if j.created {
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	file, err := os.OpenFile(j.writePath(), flags, 0o644)
	if err != nil {
		panic(&ResourceError{Resource: "json_log: opening " + j.writePath(), Err: err})
	}
	j.file = file
	j.created = true
}

// writePath is where the log is written: its path, or the partial file while
// staged.
func (j *JsonLogOutputFunction) writePath() string {
	if j.partial != "" {
		return j.partial
	}
	return j.path
}

// Stage makes the log write to a partial file until Commit. Call it before the
// sink's first run.
func (j *JsonLogOutputFunction) Stage() {
	j.mutex.Lock()
	defer j.mutex.Unlock()
	j.partial = j.path + partialSuffix
}

// Commit publishes the staged log at its path. A sink that never ran writes
// nothing, leaving any file at the path as it was.
func (j *JsonLogOutputFunction) Commit() error {
	j.mutex.Lock()
	defer j.mutex.Unlock()
	if !j.created {
		return nil
	}
	if err := j.closeLocked(); err != nil {
		return err
	}
	j.created = false
	if err := os.Rename(j.writePath(), j.path); err != nil {
		return &ResourceError{Resource: "json_log: publishing " + j.path, Err: err}
	}
	return nil
}

// Abort discards the staged log, along with any partial file a killed run left.
func (j *JsonLogOutputFunction) Abort() {
	j.mutex.Lock()
	defer j.mutex.Unlock()
	j.closeLocked()
	j.created = false
	os.Remove(j.writePath())
}

// closeLocked flushes and closes the file if it is open. The caller holds mutex.
func (j *JsonLogOutputFunction) closeLocked() error {
	if j.file == nil {
		return nil
	}
	var err error
	if j.writer != nil {
		err = j.writer.Flush()
		j.writer = nil
	}
	if closeErr := j.file.Close(); err == nil {
		err = closeErr
	}
	j.file = nil
	if err != nil {
		return &ResourceError{Resource: "json_log: writing " + j.writePath(), Err: err}
	}
	return nil
}

// Finalize writes out the run's buffered entries and closes the log, once the
// run can produce no more output.
func (j *JsonLogOutputFunction) Finalize() {
	j.mutex.Lock()
	defer j.mutex.Unlock()
	if err := j.closeLocked(); err != nil {
		panic(err)
	}
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

	j.mutex.Lock()
	defer j.mutex.Unlock()
	// A caller driving the sink by hand, without a coordinator, may never call
	// Configure; open on first output so that still works.
	j.openLocked()
	if j.writer != nil {
		// The newline goes in separately: appending it to jsonData would
		// reallocate it, once per entry.
		if _, err = j.writer.Write(jsonData); err == nil {
			err = j.writer.WriteByte('\n')
		}
	} else {
		_, err = j.file.Write(append(jsonData, '\n'))
	}
	if err != nil {
		panic(&ResourceError{Resource: "json_log: writing " + j.writePath(), Err: err})
	}
}

// NewJsonLogOutputFunction creates a JsonLogOutputFunction writing to filePath.
// Nothing is opened until the run starts (see JsonLogOutputFunction).
func NewJsonLogOutputFunction(
	filePath string,
) *JsonLogOutputFunction {
	return &JsonLogOutputFunction{path: filePath, mutex: &sync.Mutex{}}
}

// JsonLogChannelOutputFunction writes JSON log entries via a background
// goroutine using a channel for improved throughput.
type JsonLogChannelOutputFunction struct {
	logChannel chan JsonLogEntry
	done       chan struct{}
}

func (j *JsonLogChannelOutputFunction) Configure(*Settings) {}

func (j *JsonLogChannelOutputFunction) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
	// Copy on retain: the background writer marshals this entry asynchronously,
	// so it must not alias a reusable buffer the iteration may overwrite next
	// step (see StateHistory.NextValues).
	j.logChannel <- JsonLogEntry{
		PartitionName:       partitionName,
		State:               append([]float64(nil), state...),
		CumulativeTimesteps: cumulativeTimesteps,
	}
}

// Close flushes and stops the background writer. Defer it after construction.
// It blocks until the writer goroutine has drained the channel and flushed
// every buffered entry to the file, so callers may read the file once Close
// returns.
func (j *JsonLogChannelOutputFunction) Close() {
	close(j.logChannel)
	<-j.done
}

// NewJsonLogChannelOutputFunction creates a JsonLogChannelOutputFunction.
// Call Close (defer it) to ensure flushing at the end of a run.
func NewJsonLogChannelOutputFunction(
	filePath string,
) *JsonLogChannelOutputFunction {
	logChannel := make(chan JsonLogEntry)
	done := make(chan struct{})
	file, err := os.Create(filePath)
	if err != nil {
		log.Fatal("Error creating log file:", err)
		panic(err)
	}
	go func() {
		defer close(done)
		defer file.Close()
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
	return &JsonLogChannelOutputFunction{logChannel: logChannel, done: done}
}

// WebsocketOutputFunction serialises and sends outputs via a websocket
// connection when the condition is met.
//
// Like every streaming sink, it is not staged (see StagedOutputFunction): the
// client has received each row as it was sent.
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

// WebsocketPushOutputFunction streams outputs to a websocket server, acting as
// the client: the config names a URL (output_function: {type: websocket, url:
// ...}) and the run pushes to it. Messages are the same protobuf PartitionState
// frames the serving mode (WebsocketOutputFunction) sends.
//
// It connects when a run starts, never when it is built — building happens when a
// config is loaded, and loading must not reach out over the network. Finalize
// sends a normal close frame and disconnects; a later run of the same sink
// reconnects, so each run is one connection.
//
// It streams as the run goes, so it is not staged (see StagedOutputFunction):
// a run that fails has sent the rows it reached.
type WebsocketPushOutputFunction struct {
	url        string
	connection *websocket.Conn
	inner      *WebsocketOutputFunction
	writeMutex sync.Mutex
	mutex      sync.Mutex
}

// NewWebsocketPushOutputFunction creates a sink that pushes to url once a run
// starts.
func NewWebsocketPushOutputFunction(url string) *WebsocketPushOutputFunction {
	return &WebsocketPushOutputFunction{url: url}
}

// Configure connects to the server, panicking if it cannot, as other sinks do
// when their destination is unavailable.
func (w *WebsocketPushOutputFunction) Configure(*Settings) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.connectLocked()
}

// connectLocked dials the server if not already connected. The caller holds mutex.
func (w *WebsocketPushOutputFunction) connectLocked() {
	if w.connection != nil {
		return
	}
	connection, _, err := websocket.DefaultDialer.Dial(w.url, nil)
	if err != nil {
		panic(&ResourceError{Resource: "websocket: connecting to " + w.url, Err: err})
	}
	w.connection = connection
	w.inner = NewWebsocketOutputFunction(connection, &w.writeMutex)
}

func (w *WebsocketPushOutputFunction) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
	w.mutex.Lock()
	// A caller driving the sink by hand may never call Configure.
	w.connectLocked()
	inner := w.inner
	w.mutex.Unlock()
	inner.Output(partitionName, state, cumulativeTimesteps)
}

// Finalize closes the connection cleanly once the run can produce no more output.
func (w *WebsocketPushOutputFunction) Finalize() {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if w.connection == nil {
		return
	}
	w.writeMutex.Lock()
	_ = w.connection.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second),
	)
	w.writeMutex.Unlock()
	_ = w.connection.Close()
	w.connection = nil
	w.inner = nil
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
