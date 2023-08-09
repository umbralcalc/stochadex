package agent

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// Action is a struct which holds the parameters required to specify an action.
type Action struct {
	Values *mat.VecDense
	Width  int
}

// Actions is a struct which holds the parameters required to specify all
// actions that are to be performed on a given timestep of the stochastic process.
type Actions struct {
	State      *Action
	Parametric *Action
}

// Actors is a struct which holds the types of actors chosen for the agent.
type Actors struct {
	State      StateActor
	Parametric ParametricActor
}

// ActorStrings are the string inputs which enable templating for the Actors struct.
type ActorStrings struct {
	Parametric string `yaml:"parametric"`
	State      string `yaml:"state"`
}

// AgentConfig specifies the settings and method implementations for an agent.
type AgentConfig struct {
	Actors      *Actors
	Generator   ActionGenerator
	Observation StateObservation
}

// AgentConfigStrings are the string inputs which enable templating for the
// AgentConfig struct.
type AgentConfigStrings struct {
	Actors      ActorStrings `yaml:"actors"`
	Generator   string       `yaml:"generator"`
	Observation string       `yaml:"observation"`
}

// InteractorInputMessage is a struct which gets passed from the Environment
// to an Interactor in order to trigger it to call its .Interact method.
type InteractorInputMessage struct {
	StateHistories   []*simulator.StateHistory
	TimestepsHistory *simulator.CumulativeTimestepsHistory
	IteratorToUpdate *simulator.StateIterator
}
