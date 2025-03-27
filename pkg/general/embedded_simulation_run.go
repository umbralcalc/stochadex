package general

import (
	"fmt"
	"regexp"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// TODO: 'embedded_partition_name/set_state_from_partition_history' feature
// (uses relative state history depths ot figure out what data to fill in)
// and 'set_timestep_history' feature as well doing the same thing.
// This stateful loading of histories will support two use cases at once:
// 1. The posterior kernel history loading
// 2. Future reward calculations from current state

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

// NamedPartitionIndex pairs the name of a partition with the partition
// index assigned to it by the PartitionCoordinator.
type NamedPartitionIndex struct {
	Name  string
	Index int
}

// EmbeddedSimulationRunIteration facilitates running an embedded
// sub-simulation to termination inside of an iteration of another
// simulation for each step of the latter simulation.
type EmbeddedSimulationRunIteration struct {
	settings              *simulator.Settings
	implementations       *simulator.Implementations
	stateMemoryUpdate     *StateMemoryUpdate
	partitionNameToIndex  map[string]int
	updateFromHistories   map[int][]NamedPartitionIndex
	initStatesFromHistory map[int]int
	useTimestepHistory    bool
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
	e.useTimestepHistory = true
	if ignore, ok := settings.Iterations[partitionIndex].Params.GetOk(
		"ignore_timestep_history"); ok {
		e.useTimestepHistory = int(ignore[0]) != 1
	}
	e.updateFromHistories = make(map[int][]NamedPartitionIndex)
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
				partitionNames := make([]NamedPartitionIndex, 0)
				for _, paramsValue := range paramsValues {
					partitionNames = append(
						partitionNames,
						NamedPartitionIndex{
							Name:  settings.Iterations[int(paramsValue)].Name,
							Index: int(paramsValue),
						},
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
	// iterations, if configured
	initTimeValue := 0.0
	if e.useTimestepHistory {
		e.implementations.TimestepFunction =
			&FromHistoryTimestepFunction{Data: timestepsHistory}
		initTimeValue = timestepsHistory.Values.AtVec(
			timestepsHistory.StateHistoryDepth - 1,
		)
	}
	e.stateMemoryUpdate.TimestepsHistory = timestepsHistory
	for inIndex, outPartitions := range e.updateFromHistories {
		iteration, ok :=
			e.implementations.Iterations[inIndex].(StateMemoryIteration)
		if ok {
			for _, outPartition := range outPartitions {
				e.stateMemoryUpdate.Name = outPartition.Name
				e.stateMemoryUpdate.StateHistory =
					stateHistories[outPartition.Index]
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
	if t, ok := params.GetOk("init_time_value"); ok {
		initTimeValue = t[0]
	}
	e.settings.InitTimeValue = initTimeValue

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
