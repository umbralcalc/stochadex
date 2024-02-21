package interactions

import (
	"fmt"
	"strings"
	"time"

	"github.com/eiannone/keyboard"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// DoNothingActionIteration implements an action iteration that just returns
// the last Action.
type DoNothingActionIteration struct {
}

func (d *DoNothingActionIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (d *DoNothingActionIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return stateHistories[partitionIndex].Values.RawRowView(0)
}

// UserInputActionIteration implements an action iteration that returns
// configured actions based on user keyboard input.
type UserInputActionIteration struct {
	keystrokeMap     map[string]int64
	keyEvents        <-chan keyboard.KeyEvent
	waitMilliseconds uint64
	skipScanning     bool
}

func (u *UserInputActionIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	u.keystrokeMap = make(map[string]int64)
	for key, vals := range settings.OtherParams[partitionIndex].IntParams {
		if strings.Contains(key, "user_input_keystroke_action_") {
			_, keystroke, ok := strings.Cut(key, "user_input_keystroke_action_")
			if !ok {
				panic("configured keystroke not identified")
			}
			u.keystrokeMap[keystroke] = vals[0]
		}
		if key == "wait_milliseconds" {
			u.waitMilliseconds = uint64(vals[0])
		}
	}
	var err error
	u.keyEvents, err = keyboard.GetKeys(1)
	if err != nil {
		panic(err)
	}
	fmt.Println("Now listening to keyboard for user-input actions " +
		"of partitionIndex = " + fmt.Sprintf("%d", partitionIndex) +
		". Press ESC to stop.")
	u.skipScanning = false // useful for graceful exits
}

func (u *UserInputActionIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	action := stateHistories[partitionIndex].Values.RawRowView(0)
	if u.skipScanning {
		return action
	}
	select {
	case event := <-u.keyEvents:
		// main action-setting code
		act := u.keystrokeMap[string(event.Rune)]
		fmt.Println("User input action: " + fmt.Sprintf("%d", act) +
			" at timestep " + fmt.Sprintf("%f",
			timestepsHistory.Values.AtVec(0)+timestepsHistory.NextIncrement))
		action[0] = float64(act)

		// allows for graceful exit
		if event.Key == keyboard.KeyEsc {
			_ = keyboard.Close()
			u.skipScanning = true
		}
	case <-time.After(time.Duration(u.waitMilliseconds) * time.Millisecond):
		break
	}
	return action
}
