package interactions

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// Action is a struct which holds the parameters required to specify an action.
type Action struct {
	Values *mat.VecDense
	Width  int
}

// AgentConfig specifies the settings and method implementations for an agent.
type AgentConfig struct {
	Actor       Actor
	Generator   ActionGenerator
	Observation StateObservation
}

// AgentConfigStrings are the string inputs which enable templating for the
// AgentConfig struct.
type AgentConfigStrings struct {
	Actor       string `yaml:"actor"`
	Generator   string `yaml:"generator"`
	Observation string `yaml:"observation"`
}

// InteractorInputMessage is a struct which gets passed from the
// PartitionCoordinatorWithAgents to an Interactor.
type InteractorInputMessage struct {
	StateHistories   []*simulator.StateHistory
	TimestepsHistory *simulator.CumulativeTimestepsHistory
	IteratorToUpdate *simulator.StateIterator
}
