package general

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// EmbeddedSimulationRunIteration facilitates running an embedded
// sub-simulation to termination inside of an iteration of another
// simulation for each step of the latter simulation.
type EmbeddedSimulationRunIteration struct {
	settings                     *simulator.Settings
	implementations              *simulator.Implementations
	stateMemoryPartitionMappings map[int]int
	burnInSteps                  int
}

func (e *EmbeddedSimulationRunIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	for index, partition := range e.implementations.Partitions {
		partition.Iteration.Configure(index, e.settings)
	}
	e.stateMemoryPartitionMappings = make(map[int]int)
	pattern := regexp.MustCompile(`(\d+)/(\w+)`)
	for outParamsName, paramsValues := range settings.Params[partitionIndex] {
		matches := pattern.FindStringSubmatch(outParamsName)
		if len(matches) == 3 {
			if matches[2] != "state_memory_partition" {
				continue
			}
			inPartition, err := strconv.Atoi(matches[1])
			if err != nil {
				panic(err)
			}
			e.stateMemoryPartitionMappings[int(paramsValues[0])] = inPartition
		}
	}
	e.burnInSteps = int(settings.Params[partitionIndex]["burn_in_steps"][0])
}

func (e *EmbeddedSimulationRunIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	if timestepsHistory.CurrentStepNumber < e.burnInSteps {
		return stateHistories[partitionIndex].Values.RawRowView(0)
	}

	// set the initial conditions from params and the other params
	// that may have been configured
	pattern := regexp.MustCompile(`(\d+)/(\w+)`)
	for outParamsName, paramsValues := range params {
		matches := pattern.FindStringSubmatch(outParamsName)
		if len(matches) == 3 {
			inPartition, err := strconv.Atoi(matches[1])
			if err != nil {
				panic(err)
			}
			inParamsName := matches[2]
			switch inParamsName {
			case "init_state_values":
				e.settings.InitStateValues[inPartition] = paramsValues
			default:
				e.settings.Params[inPartition][inParamsName] = paramsValues
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
			&MemoryTimestepFunction{Data: timestepsHistory}
		params["init_time_value"] = []float64{
			timestepsHistory.Values.AtVec(
				timestepsHistory.StateHistoryDepth - 1,
			),
		}
	}
	for outPartition, inPartition := range e.stateMemoryPartitionMappings {
		iteration, ok := e.implementations.Partitions[inPartition].
			Iteration.(*MemoryIteration)
		if ok {
			iteration.Data = stateHistories[outPartition]
			e.settings.InitStateValues[inPartition] =
				iteration.Data.Values.RawRowView(
					iteration.Data.StateHistoryDepth - 1,
				)
		} else {
			panic(fmt.Errorf(
				"internal state partition %d is not a MemoryIteration",
				inPartition,
			))
		}
	}
	e.settings.InitTimeValue = params["init_time_value"][0]

	// instantiate and run the embedded simulation to termination
	coordinator := simulator.NewPartitionCoordinator(
		e.settings,
		e.implementations,
	)
	coordinator.Run()

	// prepare the returned state slice as the concatenated
	// final states of all partitions
	concatFinalStates := make([]float64, 0)
	for _, stateHistory := range coordinator.StateHistories {
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
