package environment

import (
	"sync"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/agent"
	"github.com/umbralcalc/stochadex/pkg/phenomena"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// randomActionGenerator defines an action generator which is only
// for testing - the .Generate method will call for a randomly-drawn
// action from a uniform distribution.
type randomActionGenerator struct {
	numDims     int
	uniformDist *distuv.Uniform
}

func (r *randomActionGenerator) Configure(
	partitionIndex int,
	settings *simulator.LoadSettingsConfig,
) {
	r.numDims = settings.StateWidths[partitionIndex]
	r.uniformDist = &distuv.Uniform{
		Min: -5.0,
		Max: 5.0,
		Src: rand.NewSource(settings.Seeds[partitionIndex]),
	}
}

func (r *randomActionGenerator) Generate(
	actions *agent.Actions,
	params *simulator.OtherParams,
	observedState []float64,
) *agent.Actions {
	for i := 0; i < r.numDims; i++ {
		actions.State.Values.SetVec(i, r.uniformDist.Rand())
	}
	return actions
}

// jumpStateActor defines a state actor which is only for testing -
// the .Act method will add the .State action to state values that
// already exist, 'jumping' the state.
type jumpStateActor struct{}

func (j *jumpStateActor) Configure(
	partitionIndex int,
	settings *simulator.LoadSettingsConfig,
) {
}

func (j *jumpStateActor) Act(
	state []float64,
	actions *agent.Actions,
) []float64 {
	for i := range state {
		state[i] += actions.State.Values.AtVec(i)
	}
	return state
}

func step(e *Environment, wg *sync.WaitGroup) {
	// interact with the system
	for index := 0; index < e.numberOfPartitions; index++ {
		e.agents[index].Interact(
			e.coordinator.StateHistories,
			e.coordinator.TimestepsHistory,
			e.coordinator.Iterators[index],
		)
	}
	// now step the underlying simulation
	e.coordinator.Step(wg)
}

func run(e *Environment) {
	// use this WaitGroup just to pass to the underlying simulation
	var wg sync.WaitGroup

	for !e.coordinator.ReadyToTerminate() {
		step(e, &wg)
	}
}

// initEnvForTesting just creates a new environment to try with and without
// the goroutines methods in testing.
func initEnvForTesting(outputFunction simulator.OutputFunction) *Environment {
	otherParams := &simulator.OtherParams{
		FloatParams: map[string][]float64{
			"variances":                   {1.0, 1.5, 0.5, 1.0, 2.0},
			"environment_noise_variances": {1.0, 2.0, 3.0, 4.0, 5.0},
			"init_state_action_values":    {1.0, 1.0, 0.0, 0.0, 1.0},
		},
	}
	settings := &simulator.LoadSettingsConfig{
		OtherParams: []*simulator.OtherParams{otherParams, otherParams},
		InitStateValues: [][]float64{
			{0.0, 2.1, 3.5, -1.0, -2.3},
			{-1.8, 2.0, 3.2, 1.1, 2.3},
		},
		Seeds:                 []uint64{236, 167},
		StateWidths:           []int{5, 5},
		StateHistoryDepths:    []int{2, 2},
		TimestepsHistoryDepth: 2,
	}
	iterations := make([]simulator.Iteration, 0)
	firstIteration := &phenomena.WienerProcessIteration{}
	iterations = append(iterations, firstIteration)
	secondIteration := &phenomena.WienerProcessIteration{}
	iterations = append(iterations, secondIteration)
	actors := &agent.Actors{
		State:      &jumpStateActor{},
		Parametric: &agent.DoNothingParametricActor{},
	}
	agents := make([]*agent.AgentConfig, 0)
	agents = append(
		agents,
		&agent.AgentConfig{
			Actors:      actors,
			Generator:   &randomActionGenerator{},
			Observation: &agent.GaussianNoiseStateObservation{},
		},
	)
	agents = append(
		agents,
		&agent.AgentConfig{
			Actors:      actors,
			Generator:   &randomActionGenerator{},
			Observation: &agent.GaussianNoiseStateObservation{},
		},
	)
	implementations := &simulator.LoadImplementationsConfig{
		Iterations:      iterations,
		OutputCondition: &simulator.EveryStepOutputCondition{},
		OutputFunction:  outputFunction,
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: 10,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
	}
	return NewEnvironment(
		&LoadConfigWithAgents{
			Settings:        settings,
			Implementations: implementations,
			Agents:          agents,
		},
	)
}

func TestEnvironment(t *testing.T) {
	t.Run(
		"test for the correct usage of goroutines in the environment",
		func(t *testing.T) {
			storeWithGoroutines := make([][][]float64, 2)
			outputWithGoroutines := &simulator.VariableStoreOutputFunction{
				Store: storeWithGoroutines,
			}
			envWithGoroutines := initEnvForTesting(outputWithGoroutines)
			storeWithoutGoroutines := make([][][]float64, 2)
			outputWithoutGoroutines := &simulator.VariableStoreOutputFunction{
				Store: storeWithoutGoroutines,
			}
			envWithoutGoroutines := initEnvForTesting(outputWithoutGoroutines)
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
