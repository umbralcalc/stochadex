package interactions

import (
	"sync"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// PartitionCoordinatorWithAgents handles agent action generation and performing
// those actions on each state partition of the underlying stochastic phenonmenon.
type PartitionCoordinatorWithAgents struct {
	coordinator     *simulator.PartitionCoordinator
	agents          []*Interactor
	newWorkChannels [](chan *InteractorInputMessage)
	parallelIndices []int
	serialIndices   []int
}

// RequestMoreInteractions spawns a goroutine for each interactor to
// carry out a ReceiveAndInteract job.
func (p *PartitionCoordinatorWithAgents) RequestMoreInteractions(wg *sync.WaitGroup) {
	// setup interactors to receive interaction requests
	for index, channel := range p.newWorkChannels {
		wg.Add(1)
		i := index
		c := channel
		go func() {
			defer wg.Done()
			p.agents[i].ReceiveAndInteract(c)
		}()
	}
	// send messages on the new work channels to ask for the next interaction
	// in the case of each partition
	for i, channel := range p.newWorkChannels {
		channel <- &InteractorInputMessage{
			StateHistories:   p.coordinator.StateHistories,
			TimestepsHistory: p.coordinator.TimestepsHistory,
			IteratorToUpdate: p.coordinator.Iterators[p.parallelIndices[i]][p.serialIndices[i]],
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
	settings *simulator.Settings,
	implementations *simulator.Implementations,
	agentByPartition map[int]*AgentConfig,
) *PartitionCoordinatorWithAgents {
	interactors := make([]*Interactor, 0)
	newWorkChannels := make([](chan *InteractorInputMessage), 0)
	parallelIndices := make([]int, 0)
	serialIndices := make([]int, 0)
	index := 0
	for parallelIndex, serialPartitions := range implementations.Iterations {
		for serialIndex, iteration := range serialPartitions {
			agent, ok := agentByPartition[index]
			if !ok {
				continue
			}
			parallelIndices = append(parallelIndices, parallelIndex)
			serialIndices = append(serialIndices, serialIndex)
			newWorkChannels = append(
				newWorkChannels,
				make(chan *InteractorInputMessage),
			)
			actionValues := settings.
				OtherParams[index].FloatParams["init_action_values"]
			action := &Action{}
			if actionValues != nil {
				action = &Action{
					Values: mat.NewVecDense(len(actionValues), actionValues),
					Width:  len(actionValues),
				}
			}
			iteration := &ActingAgentIteration{
				Action:    action,
				Iteration: iteration,
				Actor:     agent.Actor,
			}
			interactors = append(
				interactors,
				NewInteractor(
					index,
					iteration,
					agent,
					settings,
				),
			)
			// overwrite the base stochastic process iteration with one that
			// has actions by the agent in it
			implementations.Iterations[parallelIndex][serialIndex] = iteration
			index += 1
		}
	}
	return &PartitionCoordinatorWithAgents{
		coordinator: simulator.NewPartitionCoordinator(
			settings,
			implementations,
		),
		agents:          interactors,
		newWorkChannels: newWorkChannels,
		parallelIndices: parallelIndices,
		serialIndices:   serialIndices,
	}
}
