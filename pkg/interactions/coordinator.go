package interactions

import (
	"sync"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// PartitionCoordinatorWithAgents handles agent action generation and performing
// those actions on each state partition of the underlying stochastic phenonmenon.
type PartitionCoordinatorWithAgents struct {
	coordinator     *simulator.PartitionCoordinator
	agents          []*Interactor
	parallelIndices []int
	serialIndices   []int
	newWorkChannels [](chan *InteractorInputMessage)
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
	// update the overall step count and get the next time increment
	p.coordinator.TimestepsHistory.CurrentStepNumber += 1
	p.coordinator.TimestepsHistory = p.coordinator.TimestepFunction.SetNextIncrement(
		p.coordinator.TimestepsHistory,
	)

	// begin by requesting iterations for the next step and waiting
	p.coordinator.RequestMoreIterations(wg)
	wg.Wait()

	// then request interactions for the next step and waiting
	p.RequestMoreInteractions(wg)
	wg.Wait()

	// then implement the pending state and time updates to the histories
	p.coordinator.UpdateHistory(wg)
	wg.Wait()
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
			interactors = append(
				interactors,
				NewInteractor(
					index,
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
		parallelIndices: parallelIndices,
		serialIndices:   serialIndices,
		newWorkChannels: newWorkChannels,
	}
}
