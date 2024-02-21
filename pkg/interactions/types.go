package interactions

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// AgentConfig specifies the settings and method implementations for an agent.
type AgentConfig struct {
	Actor              Actor
	GeneratorPartition int
}

// AgentConfigStrings are the string inputs which enable templating for the
// AgentConfig struct.
type AgentConfigStrings struct {
	Actor              string `yaml:"actor"`
	GeneratorPartition int    `yaml:"generator_partition"`
}

// InteractorInputMessage is a struct which gets passed from the
// PartitionCoordinatorWithAgents to an Interactor.
type InteractorInputMessage struct {
	StateHistories   []*simulator.StateHistory
	TimestepsHistory *simulator.CumulativeTimestepsHistory
	IteratorToUpdate *simulator.StateIterator
}
