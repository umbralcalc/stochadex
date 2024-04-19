package embedded

import (
	"regexp"
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"github.com/umbralcalc/stochadex/pkg/streamers"
)

// EmbeddedSimulationRunIteration facilitates running an embedded
// sub-simulation to termination inside of an iteration of another
// simulation for each step of the latter simulation.
type EmbeddedSimulationRunIteration struct {
	settings                     *simulator.Settings
	implementations              *simulator.Implementations
	stateMemoryPartitionMappings map[int]int
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
	for outParamsName, paramsValues := range settings.
		OtherParams[partitionIndex].IntParams {
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
}

func (e *EmbeddedSimulationRunIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	// set the initial conditions from params and the other params
	// that may have been configured
	pattern := regexp.MustCompile(`(\d+)/(\w+)`)
	for outParamsName, paramsValues := range params.FloatParams {
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
				e.settings.OtherParams[inPartition].FloatParams[inParamsName] =
					paramsValues
			}
		}
	}
	e.settings.InitTimeValue = params.FloatParams["init_time_value"][0]

	// instantiate the embedded simulation
	coordinator := simulator.NewPartitionCoordinator(
		e.settings,
		e.implementations,
	)

	// set the data for the past timesteps and state memory partition
	// iterations, if configured - the application/non-application of
	// this logic basically determines whether or not the simulation
	// is being run over the past window of timesteps or up to some
	// future horizon
	if len(e.stateMemoryPartitionMappings) > 0 {
		e.implementations.TimestepFunction =
			&streamers.MemoryTimestepFunction{Data: timestepsHistory}
	}
	for outPartition, inPartition := range e.stateMemoryPartitionMappings {
		iteration, ok := coordinator.Iterators[inPartition].
			Iteration.(*streamers.MemoryIteration)
		if ok {
			iteration.Data = stateHistories[outPartition]
		}
	}

	// run the embedded simulation to termination
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
