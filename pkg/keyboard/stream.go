package keyboard

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/eiannone/keyboard"
	"github.com/umbralcalc/stochadex/pkg/api"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// The keyboard as a stream input (PLAN.md 1.7). Each mapped key press is a
// message setting one stream partition's value, so the keyboard steers a run
// through params_from_input like any other stream:
//
//	inputs:
//	  keys:
//	    stream:
//	      keyboard: {partition: steer, keys: {a: [-1.0], d: [1.0], s: [0.0]}}
//	main:
//	  partitions:
//	  - name: steering
//	    iteration: {type: param_values}
//	    params: {param_values: [0.0]}
//	    params_from_input: {param_values: {input: keys, partition: steer}}
//
// A value holds until the next mapped key (hold_last), unmapped keys are
// ignored, and Esc stops listening; the run carries on with the last value.
// record: writes what the keyboard gave each step, and the recording replays
// the run exactly as a json_log source input. A run is only worth steering in
// real time, so give it run: {pace_ms: ...}.
//
// Importing this package registers the transport; the stochadex CLI does.

func init() {
	api.RegisterStreamTransport("keyboard", api.StreamTransport{
		Check: func(fields map[string]interface{}) error {
			_, err := parseKeyMap(fields)
			return err
		},
		Open: openKeyboard,
	})
}

// openKeys opens the terminal's key events, and the function that releases
// them; tests replace it.
var openKeys = func() (<-chan keyboard.KeyEvent, func() error, error) {
	events, err := keyboard.GetKeys(1)
	return events, keyboard.Close, err
}

// keyMap is what each key sets a stream partition to.
type keyMap struct {
	partition string
	keys      map[rune][]float64
}

// parseKeyMap reads a keyboard stream's fields: the stream partition the keys
// set, and each key's value, all of one width.
func parseKeyMap(fields map[string]interface{}) (keyMap, error) {
	parsed := keyMap{keys: map[rune][]float64{}}
	width := -1
	for field, value := range fields {
		switch field {
		case "partition":
			name, ok := value.(string)
			if !ok {
				return keyMap{}, fmt.Errorf("partition must name the stream partition the keys set")
			}
			parsed.partition = name
		case "keys":
			keys, ok := value.(map[interface{}]interface{})
			if !ok || len(keys) == 0 {
				return keyMap{}, fmt.Errorf("keys must map each key to its value, e.g. keys: {a: [1.0]}")
			}
			for key, raw := range keys {
				label := []rune(fmt.Sprint(key))
				if len(label) != 1 {
					return keyMap{}, fmt.Errorf("key %q must be a single character", fmt.Sprint(key))
				}
				values, err := floats(raw)
				if err != nil {
					return keyMap{}, fmt.Errorf("key %q: %w", string(label), err)
				}
				if width >= 0 && len(values) != width {
					return keyMap{}, fmt.Errorf("key %q sets %d values, but other keys set %d; "+
						"every key sets the whole partition", string(label), len(values), width)
				}
				width = len(values)
				parsed.keys[label[0]] = values
			}
		default:
			return keyMap{}, fmt.Errorf("unknown field %q (expected partition and keys)", field)
		}
	}
	if parsed.partition == "" || len(parsed.keys) == 0 {
		return keyMap{}, fmt.Errorf("needs partition: and keys:, e.g. " +
			"keyboard: {partition: steer, keys: {a: [-1.0], d: [1.0]}}")
	}
	return parsed, nil
}

func floats(raw interface{}) ([]float64, error) {
	list, ok := raw.([]interface{})
	if !ok || len(list) == 0 {
		return nil, fmt.Errorf("its value must be a list of numbers, e.g. [1.0]")
	}
	values := make([]float64, len(list))
	for i, element := range list {
		switch number := element.(type) {
		case int:
			values[i] = float64(number)
		case float64:
			values[i] = number
		default:
			return nil, fmt.Errorf("its value must be a list of numbers, got %v", element)
		}
	}
	return values, nil
}

// keyboardSource reads key presses until Esc or Close.
type keyboardSource struct {
	keyMap
	events  <-chan keyboard.KeyEvent
	release func() error
	stop    chan struct{}
	once    sync.Once
}

func openKeyboard(fields map[string]interface{}) (api.StreamSource, error) {
	parsed, err := parseKeyMap(fields)
	if err != nil {
		return nil, err
	}
	events, release, err := openKeys()
	if err != nil {
		return nil, err
	}
	labels := make([]string, 0, len(parsed.keys))
	for key := range parsed.keys {
		labels = append(labels, string(key))
	}
	sort.Strings(labels)
	fmt.Fprintf(os.Stderr, "stochadex: keys %s set %s; Esc stops listening\n",
		strings.Join(labels, ", "), parsed.partition)
	return &keyboardSource{keyMap: parsed, events: events, release: release,
		stop: make(chan struct{})}, nil
}

// Read delivers a message for each mapped key until Esc, Close, or a message
// the run cannot take.
func (k *keyboardSource) Read(deliver func(message []byte) error) {
	for {
		select {
		case <-k.stop:
			return
		case event, ok := <-k.events:
			if !ok || event.Key == keyboard.KeyEsc {
				return
			}
			values, mapped := k.keys[event.Rune]
			if !mapped {
				continue
			}
			message, _ := json.Marshal([]simulator.JsonLogEntry{
				{PartitionName: k.partition, State: values}})
			if deliver(message) != nil {
				return
			}
		}
	}
}

// Close stops reading and gives the terminal back.
func (k *keyboardSource) Close() error {
	k.once.Do(func() { close(k.stop) })
	return k.release()
}
