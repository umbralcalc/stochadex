package inference

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distmv"
)

func TestGaussianProcessGradient(t *testing.T) {
	t.Run(
		"test that the Gaussian Process gradient runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"gaussian_process_gradient_settings.yaml",
			)
			dist, _ := distmv.NewNormal(
				[]float64{5.0, -3.0},
				mat.NewSymDense(2, []float64{1.0, 0.0, 0.0, 7.0}),
				rand.NewSource(14265),
			)
			batchData := make([]float64, 0)
			batchFuncData := make([]float64, 0)
			batchTimesData := make([]float64, 0)
			for i := 0; i < 100; i++ {
				values := dist.Rand(nil)
				batchData = append(batchData, values...)
				batchFuncData = append(batchFuncData, values[0])
				batchTimesData = append(batchTimesData, float64(i))
			}
			iterations := []simulator.Iteration{
				&GaussianProcessGradientIteration{
					Kernel: &kernels.ConstantIntegrationKernel{},
					Batch: &simulator.StateHistory{
						Values:            mat.NewDense(100, 2, batchData),
						StateWidth:        2,
						StateHistoryDepth: 100,
					},
					BatchFunction: &simulator.StateHistory{
						Values:            mat.NewDense(100, 1, batchFuncData),
						StateWidth:        1,
						StateHistoryDepth: 100,
					},
					BatchTimes: &simulator.CumulativeTimestepsHistory{
						Values:            mat.NewVecDense(100, batchTimesData),
						StateHistoryDepth: 100,
					},
				},
				&continuous.GradientDescentIteration{},
			}
			for index, iteration := range iterations {
				iteration.Configure(index, settings)
			}
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
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
		"test that the Gaussian Process gradient runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"gaussian_process_gradient_settings.yaml",
			)
			dist, _ := distmv.NewNormal(
				[]float64{5.0, -3.0},
				mat.NewSymDense(2, []float64{1.0, 0.0, 0.0, 7.0}),
				rand.NewSource(14265),
			)
			batchData := make([]float64, 0)
			batchFuncData := make([]float64, 0)
			batchTimesData := make([]float64, 0)
			for i := 0; i < 100; i++ {
				values := dist.Rand(nil)
				batchData = append(batchData, values...)
				batchFuncData = append(batchFuncData, values[0])
				batchTimesData = append(batchTimesData, float64(i))
			}
			iterations := []simulator.Iteration{
				&GaussianProcessGradientIteration{
					Kernel: &kernels.ConstantIntegrationKernel{},
					Batch: &simulator.StateHistory{
						Values:            mat.NewDense(100, 2, batchData),
						StateWidth:        2,
						StateHistoryDepth: 100,
					},
					BatchFunction: &simulator.StateHistory{
						Values:            mat.NewDense(100, 1, batchFuncData),
						StateWidth:        1,
						StateHistoryDepth: 100,
					},
					BatchTimes: &simulator.CumulativeTimestepsHistory{
						Values:            mat.NewVecDense(100, batchTimesData),
						StateHistoryDepth: 100,
					},
				},
				&continuous.GradientDescentIteration{},
			}
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
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
}
