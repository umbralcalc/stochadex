package main

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distuv"
)

type WienerProcessIteration struct {
	unitNormalDist *distuv.Normal
}

func (w *WienerProcessIteration) Iterate(
	otherParams *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.TimestepsHistory,
) *simulator.State {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		values[i] = stateHistory.Values.At(0, i) +
			math.Sqrt(otherParams.FloatParams["variances"][i])*
				w.unitNormalDist.Rand()
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
	settings := simulator.NewLoadSettingsConfigFromYaml("config.yaml")
	iterations := make([]simulator.Iteration, 0)
	for partitionIndex := range settings.StateWidths {
		iterations = append(
			iterations,
			&WienerProcessIteration{
				unitNormalDist: &distuv.Normal{
					Mu:    0.0,
					Sigma: 1.0,
					Src:   rand.NewSource(settings.Seeds[partitionIndex]),
				},
			},
		)
	}
	implementations := &simulator.LoadImplementationsConfig{
		Iterations:      iterations,
		OutputCondition: &simulator.EveryStepOutputCondition{},
		OutputFunction:  &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: 1000,
		},
		TimestepFunction: &simulator.ConstantNoMemoryTimestepFunction{
			Stepsize: 1.0,
		},
	}
	config := simulator.NewStochadexConfig(
		settings,
		implementations,
	)
	manager := simulator.NewPartitionManager(config)
	manager.Run()
}
