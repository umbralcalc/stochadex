package general

import (
	"math"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// cumulativeMeanImplementations runs a varying data partition and its
// cumulative mean for 50 steps, outputting through function.
func cumulativeMeanImplementations(function simulator.OutputFunction) *simulator.Implementations {
	return &simulator.Implementations{
		Iterations: []simulator.Iteration{
			&ValuesFunctionIteration{Function: func(
				_ *simulator.Params,
				_ int,
				_ []*simulator.StateHistory,
				timestepsHistory *simulator.CumulativeTimestepsHistory,
			) []float64 {
				t := timestepsHistory.Values.AtVec(0) + timestepsHistory.NextIncrement
				return []float64{math.Sin(t), t * t}
			}},
			&ValuesFunctionCumulativeMeanIteration{Function: DataValuesFunction},
		},
		OutputCondition: &simulator.EveryStepOutputCondition{},
		OutputFunction:  function,
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: 50,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
	}
}

func TestValuesFunctionCumulativeMeanIteration(t *testing.T) {
	t.Run(
		"each step's state is the mean of every data row so far, the initial one included",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./values_function_cumulative_mean_settings.yaml")
			store := simulator.NewStateTimeStorage()
			implementations := cumulativeMeanImplementations(
				&simulator.StateTimeStorageOutputFunction{Store: store})
			for index, iteration := range implementations.Iterations {
				iteration.Configure(index, settings)
			}
			simulator.NewPartitionCoordinator(settings, implementations).Run()
			data, means := store.GetValues("data"), store.GetValues("cumulative_mean")
			sum := make([]float64, 2)
			for row := range data {
				floats.Add(sum, data[row])
				want := floats.ScaleTo(make([]float64, 2), 1/float64(row+1), sum)
				if !floats.EqualApprox(means[row], want, 1e-12) {
					t.Fatalf("row %d: mean %v, want %v", row, means[row], want)
				}
			}
		},
	)
	t.Run(
		"test that the values function cumulative mean iteration runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./values_function_cumulative_mean_settings.yaml")
			implementations := cumulativeMeanImplementations(&simulator.NilOutputFunction{})
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("test harness failed: %v", err)
			}
		},
	)
}
