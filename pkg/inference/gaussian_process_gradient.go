package inference

import (
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// TODO: This needs to be removed and become the PosteriorKernelUpdateIteration
// that always has a specific historical time range associated to it.

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
	discount := 1.0
	g.Kernel.SetParams(params)
	if d, ok := params.GetOk("past_discounting_factor"); ok {
		discount = d[0]
	}
	baseVariance := params.Get("base_variance")[0]
	currentFunction := stateHistories[int(
		params.Get("function_values_partition")[0])].Values.At(0, 0)
	batchTimes := g.BatchTimes.Values.RawVector().Data
	var kernelValue float64
	var discountProd float64
	for i, ti := range batchTimes {
		discountProd = 1.0
		for j, tj := range batchTimes[i:] {
			kernelValue = discountProd * g.Kernel.Evaluate(
				g.Batch.Values.RawRowView(i),
				g.Batch.Values.RawRowView(j),
				ti,
				tj,
			) / baseVariance
			gradient -= currentFunction * kernelValue
			if g.BatchFunction != nil {
				gradient += 0.5 * (g.BatchFunction.Values.At(
					i, g.functionValuesIndex) + g.BatchFunction.Values.At(
					i+j, g.functionValuesIndex)) * kernelValue
			}
			discountProd *= discount
		}
	}
	norm := float64(len(batchTimes) * (len(batchTimes) / 2))
	return []float64{gradient / norm}
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
