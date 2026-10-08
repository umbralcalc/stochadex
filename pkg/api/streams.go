package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// params_from_input and stream inputs (PLAN.md 1.5). A partition's params key
// can be set from an input between steps:
//
//	params_from_input: {price: {input: feed, partition: price}}
//
// From a stored input (source: or simulation:), step k gets the input
// partition's row k, as from_input does (row 0 belongs to the initial state).
// From a stream input, each step gets the newest value a message has brought,
// and holds it until the next (on_empty: hold_last); until the first message
// the key keeps its configured value.
//
// Values only ever enter between steps, through
// simulator.PartitionCoordinator.InjectParams, never inside Iterate. A run with
// no params_from_input is run exactly as before, with no per-step cost.
//
// A stream input with record: writes the value it gave each step (row 0 the
// initial value, row k the value step k used, at step k's time) as a json_log.
// Replaying the config with the stream swapped for {source: {json_log: ...}}
// on the record injects the same values at the same steps, so the run repeats.

// paramFeeds sets a run's params from its inputs between steps.
type paramFeeds struct {
	stored  []storedFeed
	streams []*streamFeed
}

// storedFeed sets one params key from a stored input's rows.
type storedFeed struct {
	partition, key, input, source string
	rows                          [][]float64
	injector                      *simulator.ParamsInjector // resolved by open
}

// streamBinding is one params key set from a stream input partition.
type streamBinding struct {
	partition, key, source string
	injector               *simulator.ParamsInjector // resolved by open
}

// streamFeed is one stream input: its transport, the newest value of each
// partition it is bound to, and its recording.
type streamFeed struct {
	name     string
	url      string
	record   string
	bindings []streamBinding
	// sources are the stream partitions bound, in name order; current holds the
	// value each is giving the run, starting with the partition's configured one.
	sources []string
	current map[string][]float64

	connection *websocket.Conn
	done       chan struct{}
	recorder   *simulator.JsonLogOutputFunction

	mutex  sync.Mutex
	latest map[string][]float64 // newest value from a message, by partition
	fresh  map[string]bool      // partitions with a value not yet injected
	err    error                // a message that could not be decoded
}

// newParamFeeds builds the feeds a config's params_from_input describes, or nil
// when it has none. Stored inputs are read through loader; streams are not
// connected until open.
func newParamFeeds(config *ApiRunConfig, loader *inputLoader) (*paramFeeds, error) {
	bound := false
	for _, partition := range config.Main.Partitions {
		bound = bound || len(partition.ParamsFromInput) > 0
	}
	if !bound {
		return nil, nil // the common case: nothing to set, and nothing allocated
	}
	feeds := &paramFeeds{}
	byName := map[string]*streamFeed{}
	for _, partition := range config.Main.Partitions {
		keys := make([]string, 0, len(partition.ParamsFromInput))
		for key := range partition.ParamsFromInput {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			binding := partition.ParamsFromInput[key]
			source := sourcePartition(binding, key)
			input := config.Inputs[binding.Input]
			if input.Stream == nil {
				rows, err := storedParamRows(config, loader, partition, key, binding, source)
				if err != nil {
					return nil, err
				}
				feeds.stored = append(feeds.stored, storedFeed{partition: partition.Name,
					key: key, input: binding.Input, source: source, rows: rows})
				continue
			}
			stream, ok := byName[binding.Input]
			if !ok {
				stream = &streamFeed{name: binding.Input, url: input.Stream.Websocket.URL,
					record: input.Record, current: map[string][]float64{}}
				byName[binding.Input] = stream
				feeds.streams = append(feeds.streams, stream)
			}
			stream.bindings = append(stream.bindings,
				streamBinding{partition: partition.Name, key: key, source: source})
			if _, seen := stream.current[source]; !seen {
				stream.sources = append(stream.sources, source)
				stream.current[source] = append([]float64(nil), partition.Params.Map[key]...)
			}
		}
	}
	for _, stream := range feeds.streams {
		sort.Strings(stream.sources)
	}
	return feeds, nil
}

// storedParamRows returns a stored input's rows for one params_from_input key,
// read on the config's first run and kept, as bindInputs binds from_input
// partitions once: later runs of the same config reuse them.
func storedParamRows(
	config *ApiRunConfig,
	loader *inputLoader,
	partition simulator.PartitionConfig,
	key string,
	binding simulator.InputParamConfig,
	source string,
) ([][]float64, error) {
	bound := partition.Name + "/" + key
	if rows, ok := config.boundParamRows[bound]; ok {
		return rows, nil
	}
	storage, err := loader.storage(binding.Input)
	if err != nil {
		return nil, err
	}
	rows, err := inputRows(storage, binding.Input, source)
	if err != nil {
		return nil, err
	}
	if width := len(partition.Params.Map[key]); len(rows[0]) != width {
		return nil, withKind(ErrData, fmt.Errorf(
			"api: partition %q params %q has width %d but input %q's %q rows have %d",
			partition.Name, key, width, binding.Input, source, len(rows[0])))
	}
	if config.boundParamRows == nil {
		config.boundParamRows = map[string][][]float64{}
	}
	config.boundParamRows[bound] = rows
	return rows, nil
}

// runCoordinator runs a coordinator to termination, setting its params from
// feeds between steps. Without feeds it is coordinator.Run.
func runCoordinator(coordinator *simulator.PartitionCoordinator, feeds *paramFeeds) error {
	if feeds == nil {
		coordinator.Run()
		return nil
	}
	if err := feeds.open(coordinator); err != nil {
		return err
	}
	defer feeds.close()
	stepper := coordinator.NewStepper()
	defer stepper.Close()
	for step := 1; !coordinator.ReadyToTerminate(); step++ {
		if err := feeds.beforeStep(coordinator, step); err != nil {
			return err
		}
		stepper.Step()
		feeds.afterStep(coordinator)
	}
	return nil
}

// open resolves every params key the feeds set (once, so each step is a copy),
// connects every stream, starts reading its messages, and starts its recording
// with the initial values at the run's start time.
func (f *paramFeeds) open(coordinator *simulator.PartitionCoordinator) error {
	resolve := func(partition, key string) (*simulator.ParamsInjector, error) {
		injector, err := coordinator.NewParamsInjector(partition, key)
		return injector, withKind(ErrData, err)
	}
	for index := range f.stored {
		injector, err := resolve(f.stored[index].partition, f.stored[index].key)
		if err != nil {
			return err
		}
		f.stored[index].injector = injector
	}
	for _, stream := range f.streams {
		for index := range stream.bindings {
			injector, err := resolve(stream.bindings[index].partition, stream.bindings[index].key)
			if err != nil {
				return err
			}
			stream.bindings[index].injector = injector
		}
	}
	for index, stream := range f.streams {
		if err := stream.open(); err != nil {
			for _, opened := range f.streams[:index] {
				opened.close()
			}
			return err
		}
		stream.recordRow(coordinator)
	}
	return nil
}

func (f *paramFeeds) close() {
	for _, stream := range f.streams {
		stream.close()
	}
}

// beforeStep injects step's values: each stored input's row for the step, and
// each stream partition's newest value if a message has brought one since the
// last step.
func (f *paramFeeds) beforeStep(coordinator *simulator.PartitionCoordinator, step int) error {
	for _, feed := range f.stored {
		if step >= len(feed.rows) {
			return withKind(ErrData, fmt.Errorf(
				"api: input %q's %q has %d rows (the initial row and %d steps), but the "+
					"run took more steps; stop it with termination_condition: "+
					"{type: input_exhausted, input: %s}", feed.input, feed.source,
				len(feed.rows), len(feed.rows)-1, feed.input))
		}
		if err := feed.injector.Inject(feed.rows[step]); err != nil {
			return withKind(ErrData, err)
		}
	}
	for _, stream := range f.streams {
		if err := stream.inject(); err != nil {
			return err
		}
	}
	return nil
}

// afterStep records the values each stream gave the step just taken.
func (f *paramFeeds) afterStep(coordinator *simulator.PartitionCoordinator) {
	for _, stream := range f.streams {
		stream.recordRow(coordinator)
	}
}

// open connects to the stream's server, starts the reader, and opens the
// recording. A server that cannot be reached is ErrUnavailable.
func (s *streamFeed) open() error {
	connection, _, err := websocket.DefaultDialer.Dial(s.url, nil)
	if err != nil {
		return &Error{Kind: ErrUnavailable, Err: fmt.Errorf(
			"api: stream input %q: connecting to %s: %w", s.name, s.url, err)}
	}
	s.connection = connection
	s.latest, s.fresh, s.err = map[string][]float64{}, map[string]bool{}, nil
	s.done = make(chan struct{})
	go s.read()
	if s.record != "" {
		s.recorder = simulator.NewJsonLogOutputFunction(s.record)
		s.recorder.Configure(nil)
	}
	return nil
}

// read keeps the newest value of each bound partition from the stream's
// messages, until the connection closes. A message that cannot be decoded
// stops reading and fails the run at the next step.
func (s *streamFeed) read() {
	defer close(s.done)
	wanted := map[string]bool{}
	for _, source := range s.sources {
		wanted[source] = true
	}
	for {
		_, message, err := s.connection.ReadMessage()
		if err != nil {
			return // the stream has ended; the run holds the last values
		}
		entries, err := decodeEntries(message)
		s.mutex.Lock()
		if err != nil {
			s.err = withKind(ErrData, fmt.Errorf("api: stream input %q: %w", s.name, err))
			s.mutex.Unlock()
			return
		}
		for _, entry := range entries {
			if wanted[entry.PartitionName] {
				s.latest[entry.PartitionName] = entry.State
				s.fresh[entry.PartitionName] = true
			}
		}
		s.mutex.Unlock()
	}
}

// inject applies every partition value that arrived since the last step.
func (s *streamFeed) inject() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.err != nil {
		return s.err
	}
	for source := range s.fresh {
		value := s.latest[source]
		for _, binding := range s.bindings {
			if binding.source != source {
				continue
			}
			if err := binding.injector.Inject(value); err != nil {
				return withKind(ErrData, fmt.Errorf("api: stream input %q partition %q: %w",
					s.name, source, err))
			}
		}
		s.current[source] = value
		delete(s.fresh, source)
	}
	return nil
}

// recordRow writes the values the stream is giving the run, at the run's
// current time.
func (s *streamFeed) recordRow(coordinator *simulator.PartitionCoordinator) {
	if s.recorder == nil {
		return
	}
	time := coordinator.Shared.TimestepsHistory.Values.AtVec(0)
	for _, source := range s.sources {
		s.recorder.Output(source, s.current[source], time)
	}
}

// close disconnects the stream, waits for its reader, and completes the
// recording.
func (s *streamFeed) close() {
	s.connection.Close()
	<-s.done
	s.connection = nil
	if s.recorder != nil {
		s.recorder.Finalize()
		s.recorder = nil
	}
}

// decodeEntries reads a json stream message: one json_log entry
// ({"partition_name": ..., "state": [...]}, "time" optional and ignored), or a
// JSON array of them.
func decodeEntries(message []byte) ([]simulator.JsonLogEntry, error) {
	trimmed := bytes.TrimSpace(message)
	var entries []simulator.JsonLogEntry
	if len(trimmed) > 0 && trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &entries); err != nil {
			return nil, fmt.Errorf("decoding message %q: %w", truncate(trimmed), err)
		}
	} else {
		var entry simulator.JsonLogEntry
		if err := json.Unmarshal(trimmed, &entry); err != nil {
			return nil, fmt.Errorf("decoding message %q: %w", truncate(trimmed), err)
		}
		entries = []simulator.JsonLogEntry{entry}
	}
	for _, entry := range entries {
		if entry.PartitionName == "" {
			return nil, fmt.Errorf("message %q has an entry with no partition_name", truncate(trimmed))
		}
	}
	return entries, nil
}

func truncate(message []byte) string {
	if len(message) > 80 {
		return string(message[:80]) + "..."
	}
	return string(message)
}
