package interactions

import "github.com/umbralcalc/stochadex/pkg/simulator"

// ActionGenerator is the interface that must be implemented in order
// to enact the policy of the agent in the simulation.
type ActionGenerator interface {
	Configure(partitionIndex int, settings *simulator.LoadSettingsConfig)
	Generate(
		action *Action,
		params *simulator.OtherParams,
		observedState []float64,
	) *Action
}

// DoNothingActionGenerator implements an action generator that just returns
// the last Action.
type DoNothingActionGenerator struct{}

func (d *DoNothingActionGenerator) Configure(
	partitionIndex int,
	settings *simulator.LoadSettingsConfig,
) {
}

func (d *DoNothingActionGenerator) Generate(
	action *Action,
	params *simulator.OtherParams,
	observedState []float64,
) *Action {
	return action
}

// Actor is the interface that must be implemented in order for the Agent
// to perform actions directly on the state of the stochastic process.
type Actor interface {
	Configure(partitionIndex int, settings *simulator.LoadSettingsConfig)
	Act(
		state []float64,
		action *Action,
	) []float64
}

// DoNothingActor implements an actor which does not ever act.
type DoNothingActor struct{}

func (d *DoNothingActor) Configure(
	partitionIndex int,
	settings *simulator.LoadSettingsConfig,
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
	settings *simulator.LoadSettingsConfig,
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
