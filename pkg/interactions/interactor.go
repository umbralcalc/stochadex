package interactions

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Interactor handles interactions with a given state partition on a
// separate goroutine by modifying its underlying Iteration function.
type Interactor struct {
	partitionIndex int
	iteration      *ActingAgentIteration
	generator      ActionGenerator
	observation    StateObservation
	settings       *simulator.Settings
}

// Configure configures all of the internal structs with the settings
// provided at initialisation.
func (a *Interactor) Configure() {
	a.iteration.Configure(a.partitionIndex, a.settings)
	a.generator.Configure(a.partitionIndex, a.settings)
	a.observation.Configure(a.partitionIndex, a.settings)
}

// Interact will perform a state observation given the stochadex
// StateHistory in time and then generates new actions to be performed
// on the state by modifying the underlying Iteration function.
func (a *Interactor) Interact(
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
	iteratorToUpdate *simulator.StateIterator,
) {
	// observe the state
	observedState := a.observation.Observe(
		a.settings.OtherParams[a.partitionIndex],
		a.partitionIndex,
		stateHistories,
		timestepsHistory,
	)
	// generate actions based on the Policy and observedState
	// and update the coordinator with the corresponding new iteration
	a.iteration.Action = a.generator.Generate(
		a.iteration.Action,
		a.settings.OtherParams[a.partitionIndex],
		observedState,
		timestepsHistory.Values.AtVec(0),
	)
	iteratorToUpdate.Iteration = a.iteration
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

// NewInteractor creates a new Interactor given an AgentConfig and partitionIndex.
func NewInteractor(
	partitionIndex int,
	iteration *ActingAgentIteration,
	config *AgentConfig,
	settings *simulator.Settings,
) *Interactor {
	interactor := &Interactor{
		partitionIndex: partitionIndex,
		iteration:      iteration,
		generator:      config.Generator,
		observation:    config.Observation,
		settings:       settings,
	}
	interactor.Configure()
	return interactor
}
