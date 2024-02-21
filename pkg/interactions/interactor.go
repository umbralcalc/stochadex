package interactions

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Interactor handles interactions with a given state partition on a
// separate goroutine by modifying its underlying Iteration PendingStateUpdate.
type Interactor struct {
	actor              Actor
	statePartition     int
	generatorPartition int
	settings           *simulator.Settings
}

// Configure configures all of the internal structs with the settings
// provided at initialisation.
func (a *Interactor) Configure() {
	a.actor.Configure(a.statePartition, a.settings)
}

// Interact will perform a state observation given the stochadex
// StateHistory in time and then generates new actions to be performed
// on the state by modifying the underlying Iteration PendingStateUpdate.
func (a *Interactor) Interact(
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
	iteratorToUpdate *simulator.StateIterator,
) {
	// get actions based on the Policy iteration and update
	// the coordinator with the corresponding new iteration
	iteratorToUpdate.PendingStateUpdate = a.actor.Act(
		iteratorToUpdate.PendingStateUpdate,
		stateHistories[a.generatorPartition].Values.RawRowView(0),
	)
}

// ReceiveAndInteract listens for input messages sent to the input channel
// and runs Interact when an InteractorInputMessage has been received on the
// provided inputChannel.
func (a *Interactor) ReceiveAndInteract(
	inputChannel <-chan *InteractorInputMessage,
) {
	inputMessage := <-inputChannel
	a.Interact(
		inputMessage.StateHistories,
		inputMessage.TimestepsHistory,
		inputMessage.IteratorToUpdate,
	)
}

// NewInteractor creates a new Interactor.
func NewInteractor(
	partitionIndex int,
	config *AgentConfig,
	settings *simulator.Settings,
) *Interactor {
	interactor := &Interactor{
		actor:              config.Actor,
		statePartition:     partitionIndex,
		generatorPartition: config.GeneratorPartition,
		settings:           settings,
	}
	interactor.Configure()
	return interactor
}
