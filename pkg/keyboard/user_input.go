package keyboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/eiannone/keyboard"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// KeystrokeChannel is an interface which must be implemented in order
// to setup the channel of key stroke inputs from the user into the
// UserInputIteration.
type KeystrokeChannel interface {
	Get(
		partitionIndex int,
		settings *simulator.Settings,
	) (<-chan keyboard.KeyEvent, error)
}

// StandardKeystrokeChannel is the standard method for retrieving key strokes
// for the user input.
type StandardKeystrokeChannel struct{}

func (s *StandardKeystrokeChannel) Get(
	partitionIndex int,
	settings *simulator.Settings,
) (<-chan keyboard.KeyEvent, error) {
	return keyboard.GetKeys(1)
}

// UserInputIteration implements an iteration that uses actions collected
// from user keyboard input.
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
