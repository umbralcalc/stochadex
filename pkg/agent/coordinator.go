package agent

import (
	"sync"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// PartitionCoordinatorWithAgents handles agent action generation and performing
// those actions on each state partition of the underlying stochastic phenonmenon.
type PartitionCoordinatorWithAgents struct {
	coordinator        *simulator.PartitionCoordinator
	agents             []*Interactor
	newWorkChannels    [](chan *InteractorInputMessage)
	numberOfPartitions int
}

// RequestMoreInteractions spawns a goroutine for each interactor to
// carry out a ReceiveAndInteract job.
func (p *PartitionCoordinatorWithAgents) RequestMoreInteractions(wg *sync.WaitGroup) {
	// setup interactors to receive interaction requests
	for index := 0; index < p.numberOfPartitions; index++ {
		wg.Add(1)
		i := index
		go func() {
			defer wg.Done()
			p.agents[i].ReceiveAndInteract(p.newWorkChannels[i])
		}()
	}
	// send messages on the new work channels to ask for the next interaction
	// in the case of each partition
	for index := 0; index < p.numberOfPartitions; index++ {
		p.newWorkChannels[index] <- &InteractorInputMessage{
			StateHistories:   p.coordinator.StateHistories,
			TimestepsHistory: p.coordinator.TimestepsHistory,
			IteratorToUpdate: p.coordinator.Iterators[index],
		}
	}
}

// Step a simulation with a collection of agents acting.
func (p *PartitionCoordinatorWithAgents) Step(wg *sync.WaitGroup) {
	// begin by requesting interactions for the next step and waiting
	p.RequestMoreInteractions(wg)
	wg.Wait()

	// now step the underlying simulation
	p.coordinator.Step(wg)
}

// ReadyToTerminate just wraps the same call to the coordinator.
func (p *PartitionCoordinatorWithAgents) ReadyToTerminate() bool {
	return p.coordinator.ReadyToTerminate()
}

// Run calls Step repeatedly until the simulation has a true boolean flag
// returned by its ReadyToTerminate call.
func (p *PartitionCoordinatorWithAgents) Run() {
	var wg sync.WaitGroup

	// terminate the for loop if the condition has been met
	for !p.ReadyToTerminate() {
		p.Step(&wg)
	}
}

// NewPartitionCoordinatorWithAgents creates a new PartitionCoordinatorWithAgents
// given a LoadConfigWithAgents.
func NewPartitionCoordinatorWithAgents(
	config *LoadConfigWithAgents,
) *PartitionCoordinatorWithAgents {
	agents := make([]*Interactor, 0)
	newWorkChannels := make([](chan *InteractorInputMessage), 0)
	for index := range config.Implementations.Iterations {
		newWorkChannels = append(
			newWorkChannels,
			make(chan *InteractorInputMessage),
		)
		stateActions := config.Settings.
			OtherParams[index].FloatParams["init_state_action_values"]
		parametricActions := config.Settings.
			OtherParams[index].FloatParams["init_parametric_action_values"]
		actions := &Actions{}
		if stateActions != nil {
			actions.State = &Action{
				Values: mat.NewVecDense(len(stateActions), stateActions),
				Width:  len(stateActions),
			}
		}
		if parametricActions != nil {
			actions.Parametric = &Action{
				Values: mat.NewVecDense(len(parametricActions), parametricActions),
				Width:  len(parametricActions),
			}
		}
		iteration := &ActingAgentIteration{
			Actions:         actions,
			Iteration:       config.Implementations.Iterations[index],
			StateActor:      config.Agents[index].Actors.State,
			ParametricActor: config.Agents[index].Actors.Parametric,
		}
		agents = append(
			agents,
			NewInteractor(
				index,
				iteration,
				config.Agents[index],
				config.Settings,
			),
		)
		// overwrite the base stochastic process iteration with one that
		// has actions by the agent in it
		config.Implementations.Iterations[index] = iteration
	}
	return &PartitionCoordinatorWithAgents{
		coordinator: simulator.NewPartitionCoordinator(
			simulator.NewStochadexConfig(
				config.Settings,
				config.Implementations,
			),
		),
		agents:             agents,
		newWorkChannels:    newWorkChannels,
		numberOfPartitions: len(config.Implementations.Iterations),
	}
}
