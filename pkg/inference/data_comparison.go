package inference

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// DataComparisonIteration allows for any log-likelihood to be used as a
// comparison distribution between data values, a mean vector and covariance
// matrix.
type DataComparisonIteration struct {
	Likelihood  LikelihoodDistribution
	burnInSteps int
	cumulative  bool
}

func (d *DataComparisonIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	d.cumulative = false
	c, ok := settings.Iterations[partitionIndex].Params.GetOk("cumulative")
	if ok {
		d.cumulative = c[0] == 1
	}
	d.burnInSteps = int(
		settings.Iterations[partitionIndex].Params.GetIndex("burn_in_steps", 0))
	d.Likelihood.Configure(partitionIndex, settings)
}

func (d *DataComparisonIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	if timestepsHistory.CurrentStepNumber < d.burnInSteps {
		return []float64{stateHistories[partitionIndex].Values.At(0, 0)}
	}
	d.Likelihood.SetParams(params)
	like := d.Likelihood.EvaluateLogLike(params.Get("latest_data_values"))
	if d.cumulative {
		like += stateHistories[partitionIndex].Values.At(0, 0)
	}
	return []float64{like}
}
