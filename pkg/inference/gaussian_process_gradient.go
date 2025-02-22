package inference

import (
	"github.com/umbralcalc/stochadex/pkg/general"
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
		"function_values_data_index"); ok {
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
	currentFunction := stateHistories[int(
		params.Get("function_values_partition")[0])].Values.At(0, 0)
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
	update *general.StateMemoryUpdate,
) {
	if _, ok := params.GetOk(update.Name + "->data"); ok {
		g.Batch = update.StateHistory
		g.BatchTimes = update.TimestepsHistory
	} else if _, ok := params.GetOk(update.Name + "->function_values_data"); ok {
		g.BatchFunction = update.StateHistory
	} else {
		panic("gaussian process gradient: memory update from partition: " +
			update.Name + " has no configured use")
	}
}
