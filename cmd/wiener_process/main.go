package main

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distuv"
)


type WienerProcessParams struct {
	Variances []float64
}

type WienerProcessIteration struct {
	dist *distuv.Normal
}

func (w *WienerProcessIteration) Iterate(
	params *WienerProcessParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.TimestepsHistory,
) *simulator.State {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
    for i := range values {
        values[i] = stateHistory.Values.At(0, i) +
		    math.Sqrt(params.Variances[i]) * w.dist.Rand()
	}
	return &simulator.State{
		Values: mat.NewVecDense(
			stateHistory.StateWidth, 
			values,
		),
		StateWidth: stateHistory.StateWidth,
	}
}

func main() {
	settings := &simulator.LoadSettingsConfig{
		OtherParams: []OtherParams,
		InitStateValues: [][]float64,
		Seeds: []int,
		StateWidths: []int{1},
		StateHistoryDepths: []int{1},
		TimestepsHistoryDepth: 1,
	}
	iterations := make([]simulator.WienerProcessIteration, 0)
	for _ := range settings.StateWidths {
        iterations = append(iterations, simulator.WienerProcessIteration)
	}
	implementations := &simulator.LoadImplementationsConfig{
		Iterations: iterations,
	    OutputCondition      OutputCondition
	    OutputFunction       OutputFunction
	    TerminationCondition TerminationCondition
	    TimestepFunction     TimestepFunction
	}
	config := simulator.NewStochadexConfig(
		settings,
		implementations,
	)
	manager := simulator.NewPartitionManager(config)
	manager.Run()
}
