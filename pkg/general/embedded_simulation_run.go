package general

import (
	"fmt"
	"regexp"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// StateMemoryIteration defines the interface that must be implemented
// in order to be update-able with a state history.
type StateMemoryIteration interface {
	UpdateMemory(stateHistory *simulator.StateHistory)
}

// EmbeddedSimulationRunIteration facilitates running an embedded
// sub-simulation to termination inside of an iteration of another
// simulation for each step of the latter simulation.
type EmbeddedSimulationRunIteration struct {
	settings                     *simulator.Settings
	implementations              *simulator.Implementations
	partitionNameToIndex         map[string]int
	stateMemoryPartitionMappings map[int]int
	burnInSteps                  int
}

func (e *EmbeddedSimulationRunIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	for index, iteration := range e.implementations.Iterations {
		iteration.Configure(index, e.settings)
	}
	e.partitionNameToIndex = make(map[string]int)
	for index, iteration := range e.settings.Iterations {
		e.partitionNameToIndex[iteration.Name] = index
	}
	e.stateMemoryPartitionMappings = make(map[int]int)
	pattern := regexp.MustCompile(`(\w+)/(\w+)`)
	for outParamsName, paramsValues := range settings.
		Iterations[partitionIndex].Params.Map {
		matches := pattern.FindStringSubmatch(outParamsName)
		if len(matches) == 3 {
			if matches[2] != "state_memory_partition" {
				continue
			}
			inPartition, ok := e.partitionNameToIndex[matches[1]]
			if !ok {
				panic("input partition was not found in embedded sim")
			}
			e.stateMemoryPartitionMappings[int(paramsValues[0])] = inPartition
		}
	}
	e.burnInSteps = int(
		settings.Iterations[partitionIndex].Params.GetIndex("burn_in_steps", 0))
}

func (e *EmbeddedSimulationRunIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	if timestepsHistory.CurrentStepNumber < e.burnInSteps {
		return stateHistories[partitionIndex].Values.RawRowView(0)
	}

	// set the initial conditions from params and the other params
	// that may have been configured
	pattern := regexp.MustCompile(`(\w+)/(\w+)`)
	for outParamsName, paramsValues := range params.Map {
		matches := pattern.FindStringSubmatch(outParamsName)
		if len(matches) == 3 {
			inPartition, ok := e.partitionNameToIndex[matches[1]]
			if !ok {
				panic("input partition was not found in embedded sim")
			}
			inParamsName := matches[2]
			switch inParamsName {
			case "init_state_values":
				e.settings.Iterations[inPartition].InitStateValues = paramsValues
			default:
				e.settings.Iterations[inPartition].Params.Set(
					inParamsName, paramsValues)
			}
		}
	}

	// set the data for the past timesteps and state memory partition
	// iterations, if configured - the application/non-application of
	// this logic basically determines whether or not the simulation
	// is being run over the past window of timesteps or up to some
	// future horizon
	if len(e.stateMemoryPartitionMappings) > 0 {
		e.implementations.TimestepFunction =
			&FromHistoryTimestepFunction{Data: timestepsHistory}
		params.Set("init_time_value", []float64{
			timestepsHistory.Values.AtVec(
				timestepsHistory.StateHistoryDepth - 1,
			),
		})
	}
	for outPartition, inPartition := range e.stateMemoryPartitionMappings {
		iteration, ok :=
			e.implementations.Iterations[inPartition].(StateMemoryIteration)
		if ok {
			outPartitionStateHistory := stateHistories[outPartition]
			iteration.UpdateMemory(outPartitionStateHistory)
			e.settings.Iterations[inPartition].InitStateValues =
				outPartitionStateHistory.Values.RawRowView(
					outPartitionStateHistory.StateHistoryDepth - 1,
				)
		} else {
			panic(fmt.Errorf(
				"internal state partition %d is not a StateMemoryIteration",
				inPartition,
			))
		}
	}
	e.settings.InitTimeValue = params.GetIndex("init_time_value", 0)

	// instantiate and run the embedded simulation to termination
	coordinator := simulator.NewPartitionCoordinator(
		e.settings,
		e.implementations,
	)
	coordinator.Run()

	// prepare the returned state slice as the concatenated
	// final states of all partitions
	concatFinalStates := make([]float64, 0)
	for _, stateHistory := range coordinator.Shared.StateHistories {
		concatFinalStates = append(
			concatFinalStates,
			stateHistory.Values.RawRowView(0)...,
		)
	}
	return concatFinalStates
}

// NewEmbeddedSimulationRunIterationFromConfigs creates a new
// EmbeddedSimulationRunIteration from settings and implementations
// configs.
func NewEmbeddedSimulationRunIteration(
	settings *simulator.Settings,
	implementations *simulator.Implementations,
) *EmbeddedSimulationRunIteration {
	return &EmbeddedSimulationRunIteration{
		settings:        settings,
		implementations: implementations,
	}
}
