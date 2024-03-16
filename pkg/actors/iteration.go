package actors

import (
	"fmt"
	"strings"
	"time"

	"github.com/eiannone/keyboard"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ActorIteration implements an actor as an iteration in the stochadex
// based on actions set by parameter.
type ActorIteration struct {
	Iteration simulator.Iteration
	Actor     Actor
}

func (a *ActorIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {

	a.Iteration.Configure(partitionIndex, settings)
	a.Actor.Configure(partitionIndex, settings)
}

func (a *ActorIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return a.Actor.Act(
		a.Iteration.Iterate(
			params,
			partitionIndex,
			stateHistories,
			timestepsHistory,
		),
		params.FloatParams["action"],
	)
}

// UserInputActorIteration implements an actor iteration that uses
// actions collected by user keyboard input.
type UserInputActorIteration struct {
	Iteration        simulator.Iteration
	Actor            Actor
	keystrokeMap     map[string]int64
	keyEvents        <-chan keyboard.KeyEvent
	waitMilliseconds uint64
	skipScanning     bool
}

func (u *UserInputActorIteration) Configure(
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

func (u *UserInputActorIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	if u.skipScanning {
		return u.Actor.Act(
			u.Iteration.Iterate(
				params,
				partitionIndex,
				stateHistories,
				timestepsHistory,
			),
			params.FloatParams["action"],
		)
	}
	select {
	case event := <-u.keyEvents:
		// main action-setting code
		act := u.keystrokeMap[string(event.Rune)]
		fmt.Println("User input action: " + fmt.Sprintf("%d", act) +
			" at timestep " + fmt.Sprintf("%f",
			timestepsHistory.Values.AtVec(0)+timestepsHistory.NextIncrement))
		params.FloatParams["action"][0] = float64(act)

		// allows for graceful exit
		if event.Key == keyboard.KeyEsc {
			_ = keyboard.Close()
			u.skipScanning = true
		}
	case <-time.After(time.Duration(u.waitMilliseconds) * time.Millisecond):
		break
	}
	return u.Actor.Act(
		u.Iteration.Iterate(
			params,
			partitionIndex,
			stateHistories,
			timestepsHistory,
		),
		params.FloatParams["action"],
	)
}
