package interactions

import (
	"fmt"
	"strings"
	"time"

	"github.com/eiannone/keyboard"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ActionGenerator is the interface that must be implemented in order
// to enact the policy of the agent in the simulation.
type ActionGenerator interface {
	Configure(partitionIndex int, settings *simulator.Settings)
	Generate(
		action *Action,
		params *simulator.OtherParams,
		observedState []float64,
		timestep float64,
	) *Action
}

// DoNothingActionGenerator implements an action generator that just returns
// the last Action.
type DoNothingActionGenerator struct{}

func (d *DoNothingActionGenerator) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (d *DoNothingActionGenerator) Generate(
	action *Action,
	params *simulator.OtherParams,
	observedState []float64,
	timestep float64,
) *Action {
	return action
}

// UserInputActionGenerator implements an action generator that returns
// configured actions based on user keyboard input.
type UserInputActionGenerator struct {
	keystrokeMap     map[string]int64
	keyEvents        <-chan keyboard.KeyEvent
	waitMilliseconds uint64
	partitionIndex   int
	skipScanning     bool
}

func (u *UserInputActionGenerator) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	u.partitionIndex = partitionIndex
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
		"of partitionIndex = " + fmt.Sprintf("%d", u.partitionIndex) +
		". Press ESC to stop.")
	u.skipScanning = false // useful for graceful exits
}

func (u *UserInputActionGenerator) Generate(
	action *Action,
	params *simulator.OtherParams,
	observedState []float64,
	timestep float64,
) *Action {
	if u.skipScanning {
		return action
	}
	select {
	case event := <-u.keyEvents:
		// main action-setting code
		act := u.keystrokeMap[string(event.Rune)]
		fmt.Println("User input action: " + fmt.Sprintf("%d", act) +
			" at timestep " + fmt.Sprintf("%f", timestep))
		action.Values.SetVec(u.partitionIndex, float64(act))

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

// Actor is the interface that must be implemented in order for the Agent
// to perform actions directly on the state of the stochastic process.
type Actor interface {
	Configure(partitionIndex int, settings *simulator.Settings)
	Act(state []float64, action *Action) []float64
}

// DoNothingActor implements an actor which does not ever act.
type DoNothingActor struct{}

func (d *DoNothingActor) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (d *DoNothingActor) Act(
	state []float64,
	action *Action,
) []float64 {
	return state
}

// ActingAgentIteration implements the same iterface of an Iteration of the
// stochadex simulator but separates out the functions for taking actions from
// the simulation iteration.
type ActingAgentIteration struct {
	Action    *Action
	Iteration simulator.Iteration
	Actor     Actor
}

// Configure simply passes on the configuration settings to the stochadex
// iteration as well as the actor.
func (a *ActingAgentIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	a.Iteration.Configure(partitionIndex, settings)
	a.Actor.Configure(partitionIndex, settings)
}

// Iterate takes the state and timesteps history and outputs an updated
// State struct using an implemented Iteration interface that was passed
// to the ActingAgentIteration at instantiation and also performs the
// .Action attribute that has been set using the Actor that was also
// passed to ActingAgentIteration.
func (a *ActingAgentIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	// iterate and then act on the state
	return a.Actor.Act(
		a.Iteration.Iterate(
			params,
			partitionIndex,
			stateHistories,
			timestepsHistory,
		),
		a.Action,
	)
}
