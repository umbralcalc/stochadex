package phenomena

import (
	"math"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// exponentialExcitingKernel weights the historic Hawkes process increments
// with an exponential function - this is just for testing.
type exponentialExcitingKernel struct{}

func (e *exponentialExcitingKernel) Evaluate(
	params *simulator.OtherParams,
	currentTime float64,
	somePreviousTime float64,
	stateElement int,
) float64 {
	return math.Exp(
		-params.FloatParams["exponential_decays"][stateElement] *
			(currentTime - somePreviousTime))
}

func TestHawkesProcess(t *testing.T) {
	t.Run(
		"test that the Hawkes process runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"hawkes_process_config.yaml",
			)
			iterations := make([][]simulator.Iteration, 0)
			serialIterations := make([]simulator.Iteration, 0)
			intensityIteration := &HawkesProcessIntensityIteration{
				excitingKernel: &exponentialExcitingKernel{},
			}
			intensityIteration.Configure(0, settings)
			serialIterations = append(serialIterations, intensityIteration)
			hawkesIteration := &HawkesProcessIteration{}
			hawkesIteration.Configure(1, settings)
			serialIterations = append(serialIterations, hawkesIteration)
			iterations = append(iterations, serialIterations)
			store := make([][][]float64, len(settings.StateWidths))
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.VariableStoreOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 250,
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
}
