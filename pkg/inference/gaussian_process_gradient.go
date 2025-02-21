package inference

import (
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// GaussianProcessGradientIteration computes the gradient for a
// Gaussian process model defined over some function values over
// a batch of data.
type GaussianProcessGradientIteration struct {
	Kernel              kernels.IntegrationKernel
	Batch               *simulator.StateHistory
	BatchFunction       *simulator.StateHistory
	BatchTimes          *simulator.CumulativeTimestepsHistory
	functionValuesIndex int
}

func (g *GaussianProcessGradientIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	g.Kernel.Configure(partitionIndex, settings)
	if index, ok := settings.Iterations[partitionIndex].Params.GetOk(
		"function_state_values_index"); ok {
		g.functionValuesIndex = int(index[0])
	}
}

func (g *GaussianProcessGradientIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	gradient := 0.0
	g.Kernel.SetParams(params)
	currentFunction := params.Get("latest_function_value")[0]
	var kernelValue float64
	for i, ti := range g.BatchTimes.Values.RawVector().Data {
		for j, tj := range g.BatchTimes.Values.RawVector().Data[i:] {
			kernelValue = g.Kernel.Evaluate(
				g.Batch.Values.RawRowView(i),
				g.Batch.Values.RawRowView(j),
				ti,
				tj,
			)
			gradient -= currentFunction / kernelValue
			if g.BatchFunction != nil {
				gradient += 0.5 * (g.BatchFunction.Values.At(
					i, g.functionValuesIndex) + g.BatchFunction.Values.At(
					j, g.functionValuesIndex)) / kernelValue
			}
		}
	}
	return []float64{gradient}
}

func (g *GaussianProcessGradientIteration) UpdateMemory(
	params *simulator.Params,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) {
	if functionPartition, ok := params.GetOk(
		"function_values_data_partition"); ok {
		g.BatchFunction = stateHistories[int(functionPartition[0])]
	}
	g.Batch = stateHistories[int(params.GetIndex("data_partition", 0))]
	g.BatchTimes = timestepsHistory
}
