package environment

import (
	"sync"

	"github.com/umbralcalc/stochadex/pkg/agent"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// Environment handles agent action generation and performing those actions
// on each state partition of the underlying stochastic phenonmenon. It also
// is responsible for receiving reward responses.
type Environment struct {
	coordinator        *simulator.PartitionCoordinator
	agents             []*agent.Interactor
	newWorkChannels    [](chan *agent.InteractorInputMessage)
	numberOfPartitions int
}

// RequestMoreInteractions spawns a goroutine for each interactor to
// carry out a ReceiveAndInteract job.
func (e *Environment) RequestMoreInteractions(wg *sync.WaitGroup) {
	// setup interactors to receive interaction requests
	for index := 0; index < e.numberOfPartitions; index++ {
		wg.Add(1)
		i := index
		go func() {
			defer wg.Done()
			e.agents[i].ReceiveAndInteract(e.newWorkChannels[i])
		}()
	}
	// send messages on the new work channels to ask for the next interaction
	// in the case of each partition
	for index := 0; index < e.numberOfPartitions; index++ {
		e.newWorkChannels[index] <- &agent.InteractorInputMessage{
			StateHistories:   e.coordinator.StateHistories,
			TimestepsHistory: e.coordinator.TimestepsHistory,
			IteratorToUpdate: e.coordinator.Iterators[index],
		}
	}
}

// Step a simulation with a collection of agents acting in an environment.
func (e *Environment) Step(wg *sync.WaitGroup) {
	// begin by requesting interactions for the next step and waiting
	e.RequestMoreInteractions(wg)
	wg.Wait()

	// now step the underlying simulation
	e.coordinator.Step(wg)
}

// ReadyToTerminate just wraps the came call to the coordinator.
func (e *Environment) ReadyToTerminate() bool {
	return e.coordinator.ReadyToTerminate()
}

// Run calls Step repeatedly until the simulation has a true boolean flag
// returned by its ReadyToTerminate call.
func (e *Environment) Run() {
	var wg sync.WaitGroup

	// terminate the for loop if the condition has been met
	for !e.ReadyToTerminate() {
		e.Step(&wg)
	}
}

// NewEnvironment creates a new Environment given a LoadConfigWithAgents.
func NewEnvironment(config *LoadConfigWithAgents) *Environment {
	agents := make([]*agent.Interactor, 0)
	newWorkChannels := make([](chan *agent.InteractorInputMessage), 0)
	for index := range config.Implementations.Iterations {
		newWorkChannels = append(
			newWorkChannels,
			make(chan *agent.InteractorInputMessage),
		)
		stateActions := config.Settings.
			OtherParams[index].FloatParams["init_state_action_values"]
		parametricActions := config.Settings.
			OtherParams[index].FloatParams["init_parametric_action_values"]
		actions := &agent.Actions{}
		if stateActions != nil {
			actions.State = &agent.Action{
				Values: mat.NewVecDense(len(stateActions), stateActions),
				Width:  len(stateActions),
			}
		}
		if parametricActions != nil {
			actions.Parametric = &agent.Action{
				Values: mat.NewVecDense(len(parametricActions), parametricActions),
				Width:  len(parametricActions),
			}
		}
		iteration := &agent.ActingAgentIteration{
			Actions:         actions,
			Iteration:       config.Implementations.Iterations[index],
			StateActor:      config.Agents[index].Actors.State,
			ParametricActor: config.Agents[index].Actors.Parametric,
		}
		agents = append(
			agents,
			agent.NewInteractor(
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
	return &Environment{
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
