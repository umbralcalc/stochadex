package general

import (
	"fmt"
	"regexp"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// StateMemoryUpdate carries a named partition's state and timestep history
// from an outer simulation into an inner (embedded) simulation.
//
// Usage hints:
//   - Used by iterations implementing StateMemoryIteration to receive context.
//   - Set Name to the outer partition name for disambiguation.
type StateMemoryUpdate struct {
	Name             string
	StateHistory     *simulator.StateHistory
	TimestepsHistory *simulator.CumulativeTimestepsHistory
}

// StateMemoryIteration marks iterations that can receive state/time from a
// parent simulation and store it for later use.
type StateMemoryIteration interface {
	UpdateMemory(params *simulator.Params, update StateMemoryUpdate)
}

// NamedIndexedState pairs a partition's name/index with its state history.
// Useful for initialising inner histories from a chosen outer partition.
type NamedIndexedState struct {
	NamedIndex simulator.NamedPartitionIndex
	History    *simulator.StateHistory
}

// EmbeddedSimulationRunIteration runs a nested simulation to termination at
// each outer step.
//
// Usage hints:
//   - Configure params on the outer iteration with keys of the form
//     "<innerPartitionName>/<param_name>" to forward into the inner simulation.
//   - Use "<innerPartitionName>/initial_state_from_partition_history" to seed
//     inner initial states from an outer partition's history.
//   - Use "<innerPartitionName>/update_from_partition_history" to stream
//     outer histories into inner iterations that implement StateMemoryIteration.
//   - Use "<innerPartitionName>/init_state_values_from_outer" with value
//     [offset, width] to seed the named inner partition's InitStateValues from
//     a slice of the outer partition's previous state at each step. Enables
//     warm-starting inner optimisers across outer steps.
//   - Optional "burn_in_steps" skips initial outer steps before running inner sim.
//
// This is a stream by default: Configure seeds the inner iterations once and each
// outer step advances them from wherever the last run left their RNGs, so two
// runs with identical inputs give different answers. That is what advancing a
// nested simulation alongside an outer one wants.
//
// SetReseedBase makes a run a pure function of its inputs instead, for when the
// nested simulation is being evaluated rather than advanced — a model re-run over
// a window, a proposal scored more than once. simulator.ReentrantSimulation is
// the same capability as a standalone value.
type EmbeddedSimulationRunIteration struct {
	settings              *simulator.Settings
	implementations       *simulator.Implementations
	partitionNameToIndex  map[string]int
	updateFromHistories   map[int][]simulator.NamedPartitionIndex
	initStatesFromHistory map[int]NamedIndexedState
	warmStartConfigs      map[int][2]int
	timestepFunction      *FromHistoryTimestepFunction
	burnInSteps           int
	reseedBase            *uint64
	simulation            *simulator.ReentrantSimulation
	concatBuffer          []float64
}

// SetReseedBase makes every run reseed its inner iterations from base mixed with
// the outer step number, so a run becomes a pure function of its inputs.
//
// Off by default: enabling it changes the numbers a configuration produces,
// because the inner noise stream is restarted rather than continued.
func (e *EmbeddedSimulationRunIteration) SetReseedBase(base uint64) {
	e.reseedBase = &base
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
	e.warmStartConfigs = make(map[int][2]int)
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
						NextValues:        make([]float64, inSettings.StateWidth),
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
			case "init_state_values_from_outer":
				inPartition, ok := e.partitionNameToIndex[matches[1]]
				if !ok {
					panic("input partition was not found in embedded sim")
				}
				e.warmStartConfigs[inPartition] = [2]int{
					int(paramsValues[0]), int(paramsValues[1]),
				}
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
		return stateHistories[partitionIndex].GetNextStateRowToUpdate()
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

	// seed inner partition init states from the outer partition's previous state
	if len(e.warmStartConfigs) > 0 {
		prevState := stateHistories[partitionIndex].Values.RawRowView(0)
		for inIdx, ow := range e.warmStartConfigs {
			init := make([]float64, ow[1])
			copy(init, prevState[ow[0]:ow[0]+ow[1]])
			e.settings.Iterations[inIdx].InitStateValues = init
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

	// roll each configured window forward by one and drop in the outer
	// partition's latest row, so the inner simulation starts from that history
	histories := make(map[int]*simulator.StateHistory, len(e.initStatesFromHistory))
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
		histories[inIndex] = out.History
	}

	// Reseeding, when enabled, restarts the inner noise stream so the run
	// depends only on its inputs and the outer step it happens at. It goes
	// through Configure, so any injected state memory is re-applied afterwards
	// — an iteration whose Configure resets fields would otherwise lose it.
	run := simulator.ReentrantRun{
		Histories: histories,
		// Steps 0: an embedded run's length is its own termination condition's
		// to decide, not the caller's.
		Steps: 0,
	}
	if e.reseedBase != nil {
		seed := simulator.DeriveSeed(*e.reseedBase, timestepsHistory.CurrentStepNumber)
		run.Seed = &seed
		run.AfterConfigure = func() {
			e.updateStateMemoryAndTime(stateHistories, timestepsHistory)
		}
	}

	// the returned state is the concatenated final states of all partitions,
	// built into a buffer this iteration keeps so a run costs no per-partition
	// allocation
	e.concatBuffer = e.simulation.RunInto(run, e.concatBuffer)
	return e.concatBuffer
}

// NewEmbeddedSimulationRunIteration constructs an embedded run iteration
// from prepared settings and implementations.
func NewEmbeddedSimulationRunIteration(
	settings *simulator.Settings,
	implementations *simulator.Implementations,
) *EmbeddedSimulationRunIteration {
	return &EmbeddedSimulationRunIteration{
		settings:        settings,
		implementations: implementations,
		// The re-entrant tier does the running; this iteration's own job is
		// plumbing outer context into the inner simulation's starting
		// conditions. They share one settings pointer, so the assignments this
		// iteration makes are what the next run sees.
		simulation: simulator.NewReentrantSimulation(settings, implementations),
	}
}
