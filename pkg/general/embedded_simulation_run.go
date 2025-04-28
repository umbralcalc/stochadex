package general

import (
	"fmt"
	"regexp"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
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
	UpdateMemory(params *simulator.Params, update StateMemoryUpdate)
}

// NamedIndexedState pairs a partition index and name with a state history.
type NamedIndexedState struct {
	NamedIndex simulator.NamedPartitionIndex
	History    *simulator.StateHistory
}

// EmbeddedSimulationRunIteration facilitates running an embedded
// sub-simulation to termination inside of an iteration of another
// simulation for each step of the latter simulation.
type EmbeddedSimulationRunIteration struct {
	settings              *simulator.Settings
	implementations       *simulator.Implementations
	partitionNameToIndex  map[string]int
	updateFromHistories   map[int][]simulator.NamedPartitionIndex
	initStatesFromHistory map[int]NamedIndexedState
	timestepFunction      *FromHistoryTimestepFunction
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
	if timestepFunction, ok :=
		e.implementations.TimestepFunction.(*FromHistoryTimestepFunction); ok {
		e.timestepFunction = timestepFunction
	}
	e.updateFromHistories = make(map[int][]simulator.NamedPartitionIndex)
	e.initStatesFromHistory = make(map[int]NamedIndexedState)
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
				inSettings := e.settings.Iterations[inPartition]
				e.initStatesFromHistory[inPartition] = NamedIndexedState{
					NamedIndex: simulator.NamedPartitionIndex{
						Name:  settings.Iterations[int(paramsValues[0])].Name,
						Index: int(paramsValues[0]),
					},
					History: &simulator.StateHistory{
						Values: mat.NewDense(
							inSettings.StateHistoryDepth,
							inSettings.StateWidth,
							nil,
						),
						StateWidth:        inSettings.StateWidth,
						StateHistoryDepth: inSettings.StateHistoryDepth,
					},
				}
			case "update_from_partition_history":
				inPartition, ok := e.partitionNameToIndex[matches[1]]
				if !ok {
					panic("input partition was not found in embedded sim")
				}
				partitionNames := make([]simulator.NamedPartitionIndex, 0)
				for _, paramsValue := range paramsValues {
					partitionNames = append(
						partitionNames,
						simulator.NamedPartitionIndex{
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
	e.burnInSteps = int(
		settings.Iterations[partitionIndex].Params.GetIndex("burn_in_steps", 0))
}

func (e *EmbeddedSimulationRunIteration) updateStateMemoryAndTime(
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) {
	stateMemoryUpdate := StateMemoryUpdate{}
	if e.timestepFunction != nil {
		e.timestepFunction.Data = timestepsHistory
	}
	stateMemoryUpdate.TimestepsHistory = timestepsHistory
	for inIndex, outs := range e.updateFromHistories {
		iteration, ok :=
			e.implementations.Iterations[inIndex].(StateMemoryIteration)
		if ok {
			for _, out := range outs {
				stateMemoryUpdate.Name = out.Name
				stateMemoryUpdate.StateHistory = stateHistories[out.Index]
				iteration.UpdateMemory(
					&e.settings.Iterations[inIndex].Params,
					stateMemoryUpdate,
				)
			}
		} else {
			panic(fmt.Errorf(
				"internal state partition %d is not a StateMemoryIteration",
				int(inIndex),
			))
		}
	}
}

func (e *EmbeddedSimulationRunIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	// update any configured state memories and time history at the start
	if timestepsHistory.CurrentStepNumber == 1 {
		e.updateStateMemoryAndTime(stateHistories, timestepsHistory)
	}

	// skip any steps for configured burn-in
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

	// set the data for the past timesteps
	if e.timestepFunction != nil {
		e.settings.InitTimeValue = timestepsHistory.Values.AtVec(
			timestepsHistory.StateHistoryDepth -
				(e.timestepFunction.InitStepsTaken + 1),
		)
	}
	if t, ok := params.GetOk("init_time_value"); ok {
		e.settings.InitTimeValue = t[0]
	}

	// instantiate the embedded simulation
	coordinator := simulator.NewPartitionCoordinator(
		e.settings,
		e.implementations,
	)
	// update any configured state histories to run the simulation from
	for inIndex, out := range e.initStatesFromHistory {
		for i := out.History.StateHistoryDepth - 1; i > 0; i-- {
			out.History.Values.SetRow(i, out.History.Values.RawRowView(i-1))
		}
		out.History.Values.SetRow(0,
			stateHistories[out.NamedIndex.Index].Values.RawRowView(
				stateHistories[out.NamedIndex.Index].StateHistoryDepth-
					out.History.StateHistoryDepth,
			),
		)
		coordinator.Shared.StateHistories[inIndex] = out.History
	}
	// run the embedded simulation to termination
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
