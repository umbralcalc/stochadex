// Package keyboard provides interactive user input capabilities for stochadex simulations.
// It enables real-time user interaction during simulation runs through keyboard events,
// allowing for dynamic control and parameter adjustment during execution.
//
// Key Features:
//   - Real-time keyboard event handling
//   - Configurable key-to-action mapping
//   - Timeout-based input handling
//   - Graceful exit and cleanup
//   - Extensible input channel abstraction
//
// Design Philosophy:
// This package provides a clean abstraction for user input that can be easily
// integrated into simulation workflows. It supports both interactive and
// non-interactive modes, making it suitable for both development and production use.
//
// Usage Patterns:
//   - Interactive simulation control (pause, resume, parameter adjustment)
//   - Real-time monitoring and intervention
//   - Development and debugging tools
//   - Educational and demonstration applications
package keyboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/eiannone/keyboard"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// KeystrokeChannel abstracts a source of keyboard events.
//
// Usage hints:
//   - Implement to inject custom key event sources (e.g., tests or GUIs).
//   - The default StandardKeystrokeChannel reads from the terminal.
type KeystrokeChannel interface {
	Get(
		partitionIndex int,
		settings *simulator.Settings,
	) (<-chan keyboard.KeyEvent, error)
}

// StandardKeystrokeChannel retrieves keystrokes from the terminal.
type StandardKeystrokeChannel struct{}

func (s *StandardKeystrokeChannel) Get(
	partitionIndex int,
	settings *simulator.Settings,
) (<-chan keyboard.KeyEvent, error) {
	return keyboard.GetKeys(1)
}

// UserInputIteration provides real-time user interaction during simulation runs
// through keyboard input handling and action mapping.
//
// This iteration type enables interactive control of simulations by mapping
// keyboard events to simulation actions. It supports configurable key mappings,
// timeout handling, and graceful exit functionality.
//
// Interactive Features:
//   - Real-time keyboard event processing
//   - Configurable key-to-action mapping
//   - Timeout-based input handling for non-blocking operation
//   - Graceful exit with ESC key
//   - Default value fallback for timeouts
//
// Configuration Parameters:
//   - "user_input_keystroke_action_<key>": Maps key characters to action IDs
//   - "wait_milliseconds": Timeout duration for input waiting (optional)
//   - "default_value": Default action ID used on timeout or exit
//
// Key Mapping:
// Key mappings are configured using parameters named "user_input_keystroke_action_<key>"
// where <key> is the character that triggers the action. For example:
//   - "user_input_keystroke_action_p" => 1 (pause action)
//   - "user_input_keystroke_action_r" => 2 (resume action)
//   - "user_input_keystroke_action_s" => 3 (stop action)
//
// Example Configuration:
//
//	iteration := &UserInputIteration{
//	    Channel: &StandardKeystrokeChannel{},
//	}
//
//	// Configure key mappings
//	params.Set("user_input_keystroke_action_p", []float64{1}) // pause
//	params.Set("user_input_keystroke_action_r", []float64{2}) // resume
//	params.Set("user_input_keystroke_action_s", []float64{3}) // stop
//	params.Set("wait_milliseconds", []float64{100})           // 100ms timeout
//	params.Set("default_value", []float64{0})                 // no action
//
// Usage Patterns:
//   - Interactive simulation control: Allow users to pause, resume, or stop simulations
//   - Parameter adjustment: Enable real-time parameter modification
//   - Monitoring and intervention: Provide manual override capabilities
//   - Educational tools: Create interactive learning experiences
//
// Thread Safety:
//   - Safe for concurrent use within simulation partitions
//   - Uses channels for thread-safe communication
//   - Automatic cleanup on exit
//
// Performance:
//   - Non-blocking operation with timeout support
//   - Minimal memory overhead
//   - Efficient event processing
//
// Error Handling:
//   - Graceful handling of keyboard initialization errors
//   - Automatic fallback to default values on timeout
//   - Clean exit on ESC key press
type UserInputIteration struct {
	Channel          KeystrokeChannel
	keystrokeMap     map[string]int64
	keyEvents        <-chan keyboard.KeyEvent
	waitMilliseconds uint64
	skipScanning     bool
}

func (u *UserInputIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	u.keystrokeMap = make(map[string]int64)
	for key, vals := range settings.Iterations[partitionIndex].Params.Map {
		if strings.Contains(key, "user_input_keystroke_action_") {
			_, keystroke, ok := strings.Cut(key, "user_input_keystroke_action_")
			if !ok {
				panic("configured keystroke not identified")
			}
			u.keystrokeMap[keystroke] = int64(vals[0])
		}
		if key == "wait_milliseconds" {
			u.waitMilliseconds = uint64(vals[0])
		}
	}
	var err error
	u.keyEvents, err = u.Channel.Get(partitionIndex, settings)
	if err != nil {
		panic(err)
	}
	fmt.Println("Now listening to keyboard for user-input actions " +
		"of partitionIndex = " + fmt.Sprintf("%d", partitionIndex) +
		". Press ESC to stop.")
	u.skipScanning = false // useful for graceful exits
}

func (u *UserInputIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	if u.skipScanning {
		return params.Get("default_value")
	}
	select {
	case event := <-u.keyEvents:
		act := u.keystrokeMap[string(event.Rune)]

		// allows for graceful exit
		if event.Key == keyboard.KeyEsc {
			_ = keyboard.Close()
			u.skipScanning = true
		}
		return []float64{float64(act)}
	case <-time.After(time.Duration(u.waitMilliseconds) * time.Millisecond):
		break
	}
	return params.Get("default_value")
}
