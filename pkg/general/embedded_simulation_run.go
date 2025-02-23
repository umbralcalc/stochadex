package general

import (
	"fmt"
	"regexp"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// StateMemoryUpdate packages a memory update with a name which is the
// partition name in the other simulation that it came from.
type StateMemoryUpdate struct {
	Name             string
	StateHistory     *simulator.StateHistory
	TimestepsHistory *simulator.CumulativeTimestepsHistory
}

// StateMemoryIteration defines the interface that must be implemented
// in order to configure an updateable memory of params, states and times
// which come from another simulation.
type StateMemoryIteration interface {
	UpdateMemory(params *simulator.Params, update *StateMemoryUpdate)
}

// EmbeddedSimulationRunIteration facilitates running an embedded
// sub-simulation to termination inside of an iteration of another
// simulation for each step of the latter simulation.
type EmbeddedSimulationRunIteration struct {
	settings              *simulator.Settings
	implementations       *simulator.Implementations
	stateMemoryUpdate     *StateMemoryUpdate
	partitionNameToIndex  map[string]int
	updateFromHistories   map[int][]string
	initStatesFromHistory map[int]int
	burnInSteps           int
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
	e.updateFromHistories = make(map[int][]string)
	e.initStatesFromHistory = make(map[int]int)
	pattern := regexp.MustCompile(`(\w+)/(\w+)`)
	for outParamsName, paramsValues := range settings.
		Iterations[partitionIndex].Params.Map {
		matches := pattern.FindStringSubmatch(outParamsName)
		if len(matches) == 3 {
			switch matches[2] {
			case "initial_state_from_partition_history":
				inPartition, ok := e.partitionNameToIndex[matches[1]]
				if !ok {
					panic("input partition was not found in embedded sim")
				}
				e.initStatesFromHistory[inPartition] = int(paramsValues[0])
			case "update_from_partition_history":
				inPartition, ok := e.partitionNameToIndex[matches[1]]
				if !ok {
					panic("input partition was not found in embedded sim")
				}
				partitionNames := make([]string, 0)
				for _, paramsValue := range paramsValues {
					partitionNames = append(
						partitionNames,
						e.settings.Iterations[int(paramsValue)].Name,
					)
				}
				e.updateFromHistories[inPartition] = partitionNames
			default:
				continue
			}
		}
	}
	e.stateMemoryUpdate = &StateMemoryUpdate{}
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
	if len(e.updateFromHistories) > 0 {
		e.implementations.TimestepFunction =
			&FromHistoryTimestepFunction{Data: timestepsHistory}
		params.Set("init_time_value", []float64{
			timestepsHistory.Values.AtVec(
				timestepsHistory.StateHistoryDepth - 1,
			),
		})
	}
	e.stateMemoryUpdate.TimestepsHistory = timestepsHistory
	for inIndex, outNames := range e.updateFromHistories {
		iteration, ok :=
			e.implementations.Iterations[inIndex].(StateMemoryIteration)
		if ok {
			for _, outName := range outNames {
				e.stateMemoryUpdate.Name = outName
				e.stateMemoryUpdate.StateHistory =
					stateHistories[e.partitionNameToIndex[outName]]
				iteration.UpdateMemory(
					&e.settings.Iterations[inIndex].Params,
					e.stateMemoryUpdate,
				)
			}
		} else {
			panic(fmt.Errorf(
				"internal state partition %d is not a StateMemoryIteration",
				int(inIndex),
			))
		}
	}
	for inIndex, outIndex := range e.initStatesFromHistory {
		e.settings.Iterations[inIndex].InitStateValues =
			stateHistories[outIndex].Values.RawRowView(
				stateHistories[outIndex].StateHistoryDepth - 1,
			)
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
