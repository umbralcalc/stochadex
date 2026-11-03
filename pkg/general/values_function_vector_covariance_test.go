package general

import (
	"math"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// subsetCovarianceIterations streams a deterministic, correlated
// three-wide data series into a full-width rolling mean and covariance and a
// covariance over the reordered subset [2, 0] of the data values (see
// values_function_vector_covariance_subset_settings.yaml).
func subsetCovarianceIterations(steps int) []simulator.Iteration {
	data := make([][]float64, steps+1)
	for i := range data {
		t := float64(i)
		data[i] = []float64{
			math.Sin(0.7 * t),
			2.0 * math.Cos(1.3*t),
			math.Sin(0.7*t) + 0.5*math.Cos(0.4*t),
		}
	}
	return []simulator.Iteration{
		&FromStorageIteration{Data: data},
		&ValuesFunctionVectorMeanIteration{
			Function: DataValuesFunction,
			Kernel:   &kernels.ExponentialIntegrationKernel{},
		},
		&ValuesFunctionVectorCovarianceIteration{
			Function: DataValuesFunction,
			Kernel:   &kernels.ExponentialIntegrationKernel{},
		},
		&ValuesFunctionVectorCovarianceIteration{
			Function: DataValuesFunction,
			Kernel:   &kernels.ExponentialIntegrationKernel{},
		},
	}
}

func TestValuesFunctionVectorCovarianceIteration(t *testing.T) {
	t.Run(
		"test that the values function vector covariance iteration runs",
		func(t *testing.T) {
			settings :=
				simulator.LoadSettingsFromYaml("./values_function_vector_covariance_settings.yaml")
			iterations := []simulator.Iteration{
				&ConstantValuesIteration{},
				&ValuesFunctionVectorMeanIteration{
					Function: DataValuesFunction,
					Kernel:   &kernels.ExponentialIntegrationKernel{},
				},
				&ValuesFunctionVectorCovarianceIteration{
					Function: DataValuesFunction,
					Kernel:   &kernels.ExponentialIntegrationKernel{},
				},
			}
			for index, iteration := range iterations {
				iteration.Configure(index, settings)
			}
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.NilOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			coordinator := simulator.NewPartitionCoordinator(
				settings,
				implementations,
			)
			coordinator.Run()
		},
	)
	t.Run(
		"test that the values function vector covariance iteration runs with harnesses",
		func(t *testing.T) {
			settings :=
				simulator.LoadSettingsFromYaml("./values_function_vector_covariance_settings.yaml")
			iterations := []simulator.Iteration{
				&ConstantValuesIteration{},
				&ValuesFunctionVectorMeanIteration{
					Function: DataValuesFunction,
					Kernel:   &kernels.ExponentialIntegrationKernel{},
				},
				&ValuesFunctionVectorCovarianceIteration{
					Function: DataValuesFunction,
					Kernel:   &kernels.ExponentialIntegrationKernel{},
				},
			}
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.NilOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("test harness failed: %v", err)
			}
		},
	)
	t.Run(
		"test that a covariance over a subset of the data values is the full covariance's sub-block",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"./values_function_vector_covariance_subset_settings.yaml")
			const steps = 50
			iterations := subsetCovarianceIterations(steps)
			for index, iteration := range iterations {
				iteration.Configure(index, settings)
			}
			store := simulator.NewStateTimeStorage()
			simulator.NewPartitionCoordinator(settings, &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: steps,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}).Run()
			full := store.GetValues("full_covariance")
			subset := store.GetValues("subset_covariance")
			if len(full) < steps || len(subset) != len(full) {
				t.Fatalf("got %d full and %d subset rows over %d steps",
					len(full), len(subset), steps)
			}
			for step := range full {
				// the output is a row-major n×n matrix: subset pair (0, 0) is
				// data (2, 2), (0, 1) and (1, 0) are data (0, 2) and (1, 1) is
				// data (0, 0)
				want := []float64{full[step][2*3+2], full[step][0*3+2],
					full[step][2*3+0], full[step][0]}
				if !floats.EqualApprox(subset[step], want, 1e-12) {
					t.Fatalf("step %d: subset covariance %v, want full sub-block %v",
						step, subset[step], want)
				}
			}
			if final := subset[len(subset)-1]; final[0] <= 0 || final[1] <= 0 || final[3] <= 0 {
				t.Errorf("final subset covariance %v should be positive for this series", final)
			}
		},
	)
	t.Run(
		"test that the subset covariance runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"./values_function_vector_covariance_subset_settings.yaml")
			implementations := &simulator.Implementations{
				Iterations:      subsetCovarianceIterations(50),
				OutputCondition: &simulator.NilOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 50,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("test harness failed: %v", err)
			}
		},
	)
	t.Run(
		"test that the covariance is the full kernel-weighted covariance matrix",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"./values_function_vector_covariance_subset_settings.yaml")
			const steps = 50
			iterations := subsetCovarianceIterations(steps)
			for index, iteration := range iterations {
				iteration.Configure(index, settings)
			}
			store := simulator.NewStateTimeStorage()
			simulator.NewPartitionCoordinator(settings, &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: steps,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}).Run()
			data := store.GetValues("data")
			means := store.GetValues("full_mean")
			got := store.GetValues("full_covariance")
			times := store.GetTimes()
			depth := settings.Iterations[0].StateHistoryDepth
			timescale := settings.Iterations[2].Params.GetIndex(
				"exponential_weighting_timescale", 0)
			// Recompute each filled step's matrix directly: the latest data row
			// with weight 1, and each of the depth rows before it weighted by
			// exp(-age / timescale), each entry summing the weighted products
			// of deviations from the mean, over the weights' sum less 1.
			compared := 0
			for step := depth; step < len(got); step++ {
				want := make([]float64, 9)
				weightSum := 0.0
				for back := range depth + 1 {
					row := step - back
					weight := math.Exp((times[row] - times[step]) / timescale)
					weightSum += weight
					for i := range 3 {
						for j := range 3 {
							want[i*3+j] += weight *
								(data[row][i] - means[step][i]) *
								(data[row][j] - means[step][j])
						}
					}
				}
				floats.Scale(1.0/(weightSum-1.0), want)
				if !floats.EqualApprox(got[step], want, 1e-9) {
					t.Fatalf("step %d: covariance %v, want %v", step, got[step], want)
				}
				compared++
			}
			if compared < steps-depth {
				t.Errorf("compared %d steps, want at least %d", compared, steps-depth)
			}
		},
	)
}
