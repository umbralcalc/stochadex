package main

import (
	"testing"

	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// Create a new partition iteration struct
type MyCustomIteration struct {
	binomialDist *distuv.Binomial
}

// Define how the parameters and settings are used configure this iteration
func (m *MyCustomIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	m.binomialDist = &distuv.Binomial{
		N: 0,
		P: 1.0,
		Src: rand.NewPCG(
			settings.Iterations[partitionIndex].Seed,
			settings.Iterations[partitionIndex].Seed,
		),
	}
}

// Define how this iteration actually changes the state of the partition over time
func (m *MyCustomIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	outputValues := make([]float64, 0)
	ps := params.Get("p_values")
	for i, n := range params.Get("n_values") {
		m.binomialDist.N = n
		m.binomialDist.P = ps[i]
		outputValues = append(outputValues, m.binomialDist.Rand())
	}
	return outputValues
}

func TestCustomIterations(t *testing.T) {
	t.Run(
		"integration test: custom iterations",
		func(t *testing.T) {
			// Create a simulator.StateTimeStorage from a simulation run
			storage := analysis.NewStateTimeStorageFromPartitions(
				// Instantiate the desired simulation state partitions
				[]*simulator.PartitionConfig{{
					Name:      "custom_partition",
					Iteration: &MyCustomIteration{},
					Params: simulator.NewParams(map[string][]float64{
						"n_values": {10, 14, 27},
						"p_values": {0.3, 0.8, 0.1},
					}),
					InitStateValues:   []float64{0.0, 0.0, 0.0},
					StateHistoryDepth: 1,
					Seed:              3421,
				}},
				// Decide when should we stop the simulation
				&simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				// Decide how time should evolve
				&simulator.ConstantTimestepFunction{
					Stepsize: 1.0,
				},
				// Input the initial time
				0.0,
			)

			// Reference the plotting data for the x-axis
			xRef := analysis.DataRef{Plotting: &analysis.DataPlotting{IsTime: true}}

			// Reference the plotting data for the y-axis
			yRefs := []analysis.DataRef{{PartitionName: "custom_partition"}}

			// Create a scatter plot from partitions in a simulator.StateTimeStorage
			_ = analysis.NewScatterPlotFromPartition(storage, xRef, yRefs)
		},
	)
}
