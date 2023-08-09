package interactions

import "github.com/umbralcalc/stochadex/pkg/simulator"

// ActionGenerator is the interface that must be implemented in order
// to enact the policy of the agent in the simulation.
type ActionGenerator interface {
	Configure(partitionIndex int, settings *simulator.LoadSettingsConfig)
	Generate(
		actions *Actions,
		params *simulator.OtherParams,
		observedState []float64,
	) *Actions
}

// DoNothingActionGenerator implements an action generator that just returns
// the last Actions.
type DoNothingActionGenerator struct{}

func (d *DoNothingActionGenerator) Configure(
	partitionIndex int,
	settings *simulator.LoadSettingsConfig,
) {
}

func (d *DoNothingActionGenerator) Generate(
	actions *Actions,
	params *simulator.OtherParams,
	observedState []float64,
) *Actions {
	return actions
}

// StateActor is the interface that must be implemented in order for
// the Agent to perform actions directly on the state of the stochastic process.
type StateActor interface {
	Configure(partitionIndex int, settings *simulator.LoadSettingsConfig)
	Act(
		state []float64,
		actions *Actions,
	) []float64
}

// DoNothingStateActor implements a state actor which does not ever act.
type DoNothingStateActor struct{}

func (d *DoNothingStateActor) Configure(
	partitionIndex int,
	settings *simulator.LoadSettingsConfig,
) {
}

func (d *DoNothingStateActor) Act(
	state []float64,
	actions *Actions,
) []float64 {
	return state
}

// ParametricActor is the interface that must be implemented in order for
// the Agent to perform actions on the parameters of the stochastic process.
type ParametricActor interface {
	Configure(partitionIndex int, settings *simulator.LoadSettingsConfig)
	Act(
		params *simulator.OtherParams,
		actions *Actions,
	) *simulator.OtherParams
}

// DoNothingParametricActor implements a parametric actor which does not ever act.
type DoNothingParametricActor struct{}

func (d *DoNothingParametricActor) Configure(
	partitionIndex int,
	settings *simulator.LoadSettingsConfig,
) {
}

func (d *DoNothingParametricActor) Act(
	params *simulator.OtherParams,
	actions *Actions,
) *simulator.OtherParams {
	return params
}

// ActingAgentIteration implements the same iterface of an Iteration of the
// stochadex simulator but separates out the functions for taking actions from
// the simulation iteration.
type ActingAgentIteration struct {
	Actions         *Actions
	Iteration       simulator.Iteration
	StateActor      StateActor
	ParametricActor ParametricActor
}

// Configure simply passes on the configuration settings to the stochadex
// iteration as well as the actors.
func (a *ActingAgentIteration) Configure(
	partitionIndex int,
	settings *simulator.LoadSettingsConfig,
) {
	a.Iteration.Configure(partitionIndex, settings)
	a.StateActor.Configure(partitionIndex, settings)
	a.ParametricActor.Configure(partitionIndex, settings)
}

// Iterate takes the state and timesteps history and outputs an updated
// State struct using an implemented Iteration interface that was passed
// to the ActingAgentIteration at instantiation and also performs the
// .Actions attributes that have been set using the StateActor and
// ParametricActor that was also passed to ActingAgentIteration.
func (a *ActingAgentIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	// act on the params
	params = a.ParametricActor.Act(params, a.Actions)
	// iterate and then act on the state
	return a.StateActor.Act(
		a.Iteration.Iterate(
			params,
			partitionIndex,
			stateHistories,
			timestepsHistory,
		),
		a.Actions,
	)
}
