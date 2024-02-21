package interactions

import (
	"sync"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/phenomena"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// randomActionIteration defines an action iteration which is only
// for testing - the .Iterate method will call for a randomly-drawn
// action from a uniform distribution.
type randomActionIteration struct {
	uniformDist *distuv.Uniform
}

func (r *randomActionIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	r.uniformDist = &distuv.Uniform{
		Min: -5.0,
		Max: 5.0,
		Src: rand.NewSource(settings.Seeds[partitionIndex]),
	}
}

func (r *randomActionIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	action := make([]float64, 0)
	for i := 0; i < stateHistories[partitionIndex].StateWidth; i++ {
		action = append(action, r.uniformDist.Rand())
	}
	return action
}

// jumpStateActor defines a state actor which is only for testing -
// the .Act method will add action to state values that already
// exist, 'jumping' the state.
type jumpStateActor struct{}

func (j *jumpStateActor) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (j *jumpStateActor) Act(
	state []float64,
	action []float64,
) []float64 {
	for i := range state {
		state[i] += action[i]
	}
	return state
}

func step(p *PartitionCoordinatorWithAgents, wg *sync.WaitGroup) {
	// update the overall step count and get the next time increment
	p.coordinator.TimestepsHistory.CurrentStepNumber += 1
	p.coordinator.TimestepsHistory = p.coordinator.TimestepFunction.SetNextIncrement(
		p.coordinator.TimestepsHistory,
	)

	// begin by requesting iterations for the next step and waiting
	p.coordinator.RequestMoreIterations(wg)
	wg.Wait()

	// interact with the system
	for i, agent := range p.agents {
		agent.Interact(
			p.coordinator.StateHistories,
			p.coordinator.TimestepsHistory,
			p.coordinator.Iterators[p.parallelIndices[i]][p.serialIndices[i]],
		)
	}

	// then implement the pending state and time updates to the histories
	p.coordinator.UpdateHistory(wg)
	wg.Wait()
}

func run(p *PartitionCoordinatorWithAgents) {
	// use this WaitGroup just to pass to the underlying simulation
	var wg sync.WaitGroup

	for !p.coordinator.ReadyToTerminate() {
		step(p, &wg)
	}
}

// initCoordinatorForTesting just creates a new coordinator with agents to try
// with and without the goroutines methods in testing.
func initCoordinatorForTesting(
	outputFunction simulator.OutputFunction,
) *PartitionCoordinatorWithAgents {
	otherParams := &simulator.OtherParams{
		FloatParams: map[string][]float64{
			"variances": {1.0, 1.5, 0.5, 1.0, 2.0},
		},
	}
	obsOtherParamsFirst := &simulator.OtherParams{
		FloatParams: map[string][]float64{
			"observation_noise_variances": {1.0, 2.0, 3.0, 4.0, 5.0},
		},
		IntParams: map[string][]int64{
			"partition_to_observe": {0},
		},
	}
	obsOtherParamsSecond := &simulator.OtherParams{
		FloatParams: map[string][]float64{
			"observation_noise_variances": {1.0, 2.0, 3.0, 4.0, 5.0},
		},
		IntParams: map[string][]int64{
			"partition_to_observe": {3},
		},
	}
	settings := &simulator.Settings{
		OtherParams: []*simulator.OtherParams{
			otherParams,
			obsOtherParamsFirst,
			otherParams,
			otherParams,
			obsOtherParamsSecond,
			otherParams,
		},
		InitStateValues: [][]float64{
			{0.0, 2.1, 3.5, -1.0, -2.3},
			{0.0, 0.0, 0.0, 0.0, 0.0},
			{1.0, 1.0, 0.0, 0.0, 1.0},
			{-1.8, 2.0, 3.2, 1.1, 2.3},
			{0.0, 0.0, 0.0, 0.0, 0.0},
			{1.0, 1.0, 0.0, 0.0, 1.0},
		},
		InitTimeValue:         0.0,
		Seeds:                 []uint64{236, 111, 232, 167, 1024, 2939},
		StateWidths:           []int{5, 5, 5, 5, 5, 5},
		StateHistoryDepths:    []int{2, 2, 2, 2, 2, 2},
		TimestepsHistoryDepth: 2,
	}
	iterations := make([][]simulator.Iteration, 0)
	iterations = append(
		iterations,
		[]simulator.Iteration{
			&phenomena.WienerProcessIteration{},
			&GaussianNoiseStateObservationIteration{},
			&randomActionIteration{},
		},
	)
	iterations = append(
		iterations,
		[]simulator.Iteration{
			&phenomena.WienerProcessIteration{},
			&GaussianNoiseStateObservationIteration{},
			&randomActionIteration{},
		},
	)
	index := 0
	for _, serialIterations := range iterations {
		for _, iteration := range serialIterations {
			iteration.Configure(index, settings)
			index += 1
		}
	}
	agents := make(map[int]*AgentConfig)
	agents[0] = &AgentConfig{
		Actor:              &jumpStateActor{},
		GeneratorPartition: 2,
	}
	agents[1] = &AgentConfig{
		Actor:              &jumpStateActor{},
		GeneratorPartition: 5,
	}
	implementations := &simulator.Implementations{
		Iterations:      iterations,
		OutputCondition: &simulator.EveryStepOutputCondition{},
		OutputFunction:  outputFunction,
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: 10,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
	}
	return NewPartitionCoordinatorWithAgents(
		settings,
		implementations,
		agents,
	)
}

func TestPartitionCoordinatorWithAgents(t *testing.T) {
	t.Run(
		"test for the correct usage of goroutines in the coordinator with agents",
		func(t *testing.T) {
			storeWithGoroutines := make([][][]float64, 6)
			outputWithGoroutines := &simulator.VariableStoreOutputFunction{
				Store: storeWithGoroutines,
			}
			envWithGoroutines := initCoordinatorForTesting(outputWithGoroutines)
			storeWithoutGoroutines := make([][][]float64, 6)
			outputWithoutGoroutines := &simulator.VariableStoreOutputFunction{
				Store: storeWithoutGoroutines,
			}
			envWithoutGoroutines := initCoordinatorForTesting(outputWithoutGoroutines)
			envWithGoroutines.Run()
			run(envWithoutGoroutines)
			for tIndex, store := range storeWithoutGoroutines {
				for pIndex, partitionStore := range store {
					for eIndex, element := range partitionStore {
						if element != storeWithGoroutines[tIndex][pIndex][eIndex] {
							t.Error("outputs with and without goroutines don't match")
						}
					}
				}
			}
		},
	)
}
